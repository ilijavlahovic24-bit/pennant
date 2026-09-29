package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"pennant/backend/internals/audit"
	"pennant/backend/internals/auth"
	"pennant/backend/internals/config"
	"pennant/backend/internals/evaluation"
	"pennant/backend/internals/flags"
	"pennant/backend/internals/jobs"
	"pennant/backend/internals/members"
	"pennant/backend/internals/targeting"
	"pennant/backend/internals/ws"
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
	membersHandler   *members.Handler
	wsHandler        *ws.Handler
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
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-User-Context"},
		AllowCredentials: true,
	}))

	// --- WebSocket hub ---
	wsHub := ws.NewHub()
	wsHandler := ws.NewHandler(wsHub, cfg)

	// --- Evaluation engine (cache + pub/sub) ---
	evalCache := evaluation.NewCache(rdb)
	evalEngine := evaluation.NewEngine(db, evalCache)
	evalHandler := evaluation.NewHandler(evalEngine)
	evalPublisher := evaluation.NewPublisher(rdb)

	evalSubscriber := evaluation.NewSubscriber(rdb)
	evalSubscriber.OnUpdate(func(ctx context.Context, orgID string) {
		if err := evalCache.InvalidateOrg(ctx, orgID); err != nil {
			slog.Error("cache invalidation failed", "err", err, "org_id", orgID)
		}
	})
	evalSubscriber.OnUpdate(func(_ context.Context, orgID string) {
		wsHub.BroadcastToOrg(orgID, ws.Event{
			Type:    "flag.updated",
			Payload: map[string]any{"org_id": orgID},
		})
	})

	// --- Domenski servisi ---
	auditSvc := audit.NewService(db)
	auditHandler := audit.NewHandler(auditSvc)

	authSvc := auth.NewService(cfg, db)
	authHandler := auth.NewHandler(cfg, authSvc)

	flagSvc := flags.NewService(db, auditSvc, evalPublisher)
	flagHandler := flags.NewHandler(flagSvc)

	targetingSvc := targeting.NewService(db, auditSvc, evalPublisher)
	targetingHandler := targeting.NewHandler(targetingSvc)

	membersSvc := members.NewService(db, cfg, auditSvc)
	membersHandler := members.NewHandler(membersSvc)

	// --- Background jobs ---
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
		membersHandler:   membersHandler,
		wsHandler:        wsHandler,
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

func (s *Server) RunSubscriber(ctx context.Context) {
	s.evalSubscriber.Run(ctx)
}

func (s *Server) RunScheduler(ctx context.Context) {
	s.scheduler.Run(ctx)
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
