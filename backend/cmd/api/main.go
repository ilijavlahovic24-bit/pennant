package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pennant/backend/internals/config"
	"pennant/backend/internals/db"
	redisclient "pennant/backend/internals/redis"
	"pennant/backend/internals/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), 30*time.Second)

	dbPool, err := db.Connect(startCtx, cfg.DatabaseURL)
	if err != nil {
		cancelStart()
		slog.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	rdb, err := redisclient.Connect(startCtx, cfg.RedisURL)
	if err != nil {
		cancelStart()
		slog.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	cancelStart()

	srv := server.New(cfg, dbPool, rdb)

	// Background worker-i dele isti kontekst života.
	bgCtx, cancelBg := context.WithCancel(context.Background())
	defer cancelBg()

	go srv.RunSubscriber(bgCtx)
	go srv.RunScheduler(bgCtx)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		slog.Error("server error", "err", err)
		cancelBg()
		os.Exit(1)
	case sig := <-stop:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	// Zaustavljamo background worker-e prvo.
	cancelBg()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped cleanly")
}
