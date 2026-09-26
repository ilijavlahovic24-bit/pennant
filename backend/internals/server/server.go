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
	"pennant/backend/internals/evaluation"
	"pennant/backend/internals/flags"
	"pennant/backend/internals/jobs"
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
	evalHandler      *evaluation.Handler
	evalSubscriber   *evaluation.Subscriber
	scheduler        *jobs.Scheduler
}

func New(cfg *config.Config, db *pgxpool.Pool, rdb *goredis.Client) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger())

	// --- Evaluation engine (cache + pub/sub) ---
	evalCache := evaluation.NewCache(rdb)
	evalEngine := evaluation.NewEngine(db, evalCache)
	evalHandler := evaluation.NewHandler(evalEngine)
	evalPublisher := evaluation.NewPublisher(rdb)
	evalSubscriber := evaluation.NewSubscriber(rdb, evalCache)

	// --- Domenski servisi ---
	auditSvc := audit.NewService(db)
	auditHandler := audit.NewHandler(auditSvc)

	authSvc := auth.NewService(cfg, db)
	authHandler := auth.NewHandler(cfg, authSvc)

	flagSvc := flags.NewService(db, auditSvc, evalPublisher)
	flagHandler := flags.NewHandler(flagSvc)

	targetingSvc := targeting.NewService(db, auditSvc, evalPublisher)
	targetingHandler := targeting.NewHandler(targetingSvc)

	expiryJob := jobs.NewExpiryJob(db, auditSvc, evalPublisher)
	scheduler := jobs.NewScheduler(expiryJob, cfg.ExpiryInterval)

	s := &Server{
		cfg:              cfg,
		db:               db,
		redis:            rdb,
		authHandler:      authHandler,
		flagHandler:      flagHandler,
		targetingHandler: targetingHandler,
		auditHandler:     auditHandler,
		evalHandler:      evalHandler,
		evalSubscriber:   evalSubscriber,
		scheduler:        scheduler,
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

// RunSubscriber pokreće pub/sub slušaoca u pozadini.
// Poziva se iz main-a kao goroutine.
func (s *Server) RunSubscriber(ctx context.Context) {
	s.evalSubscriber.Run(ctx)
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
func (s *Server) RunScheduler(ctx context.Context) {
	s.scheduler.Run(ctx)
}
