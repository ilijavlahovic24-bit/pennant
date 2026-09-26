package evaluation

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

// Subscriber sluša sve `flag-updates:*` kanale i invalidira cache.
type Subscriber struct {
	rdb   *goredis.Client
	cache *Cache
}

func NewSubscriber(rdb *goredis.Client, cache *Cache) *Subscriber {
	return &Subscriber{rdb: rdb, cache: cache}
}

// Run se pokreće u goroutine-i iz main-a. Blokira dok se ctx ne otkaže.
func (s *Subscriber) Run(ctx context.Context) {
	pubsub := s.rdb.PSubscribe(ctx, "flag-updates:*")
	defer func() { _ = pubsub.Close() }()

	ch := pubsub.Channel()
	slog.Info("evaluation subscriber started", "pattern", "flag-updates:*")

	for {
		select {
		case <-ctx.Done():
			slog.Info("evaluation subscriber stopping")
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			s.handle(ctx, msg.Channel, msg.Payload)
		}
	}
}

func (s *Subscriber) handle(ctx context.Context, channel, payload string) {
	var evt flagUpdateEvent
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		slog.Warn("invalid flag update payload", "err", err, "payload", payload)
		return
	}
	orgID := evt.OrgID
	if orgID == "" {
		if i := strings.LastIndex(channel, ":"); i >= 0 {
			orgID = channel[i+1:]
		}
	}
	if orgID == "" {
		return
	}
	if err := s.cache.InvalidateOrg(ctx, orgID); err != nil {
		slog.Error("cache invalidation failed", "err", err, "org_id", orgID)
		return
	}
	slog.Debug("evaluation cache invalidated", "org_id", orgID)
}
