package server

import (
	"context"
	"errors"
	"net/http"
	"pennant/backend/internals/auth"
	"pennant/backend/internals/config"
	"pennant/backend/internals/flags"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

type Server struct {
	cfg         *config.Config
	http        *http.Server
	db          *pgxpool.Pool
	redis       *goredis.Client
	authHandler *auth.Handler
	flagHandler *flags.Handler
}

func New(cfg *config.Config, db *pgxpool.Pool, rdb *goredis.Client) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger())
	authSvc := auth.NewService(cfg, db)
	authHandler := auth.NewHandler(cfg, authSvc)
	flagSvc := flags.NewService(db)
	flagHandler := flags.NewHandler(flagSvc)
	s := &Server{
		cfg:         cfg,
		db:          db,
		redis:       rdb,
		authHandler: authHandler,
		flagHandler: flagHandler,
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
