package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"pennant/backend/internals/audit"
	"pennant/backend/internals/auth"
	"pennant/backend/internals/config"
	"pennant/backend/internals/flags"
	"pennant/backend/internals/targeting"
)

type Server struct {
	cfg              *config.Config
	http             *http.Server
	db               *pgxpool.Pool
	redis            *goredis.Client
	authHandler      *auth.Handler
	flagHandler      *flags.Handler
	targetingHandler *targeting.Handler
	auditHandler     *audit.Handler
}

func New(cfg *config.Config, db *pgxpool.Pool, rdb *goredis.Client) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger())

	// Audit se kreira prvi jer ga flags i targeting koriste.
	auditSvc := audit.NewService(db)
	auditHandler := audit.NewHandler(auditSvc)

	authSvc := auth.NewService(cfg, db)
	authHandler := auth.NewHandler(cfg, authSvc)

	flagSvc := flags.NewService(db, auditSvc)
	flagHandler := flags.NewHandler(flagSvc)

	targetingSvc := targeting.NewService(db, auditSvc)
	targetingHandler := targeting.NewHandler(targetingSvc)

	s := &Server{
		cfg:              cfg,
		db:               db,
		redis:            rdb,
		authHandler:      authHandler,
		flagHandler:      flagHandler,
		targetingHandler: targetingHandler,
		auditHandler:     auditHandler,
	}
	s.registerRoutes(router)

	s.http = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
