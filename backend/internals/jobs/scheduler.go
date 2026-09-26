package jobs

import (
	"context"
	"log/slog"
	"time"
)

type Scheduler struct {
	expiry   *ExpiryJob
	interval time.Duration
}

func NewScheduler(expiry *ExpiryJob, interval time.Duration) *Scheduler {
	return &Scheduler{expiry: expiry, interval: interval}
}

// Run blokira dok se ctx ne otkaže. Pokreće expiry job na svakom tick-u.
func (s *Scheduler) Run(ctx context.Context) {
	slog.Info("scheduler started", "interval", s.interval.String())

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Prvi run odmah, ne čekamo prvi interval.
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopping")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	if err := s.expiry.Run(ctx); err != nil {
		slog.Error("expiry job failed", "err", err)
	}
}
