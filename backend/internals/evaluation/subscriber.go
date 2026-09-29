package evaluation

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

// UpdateHandler is called on every message with org_id.
type UpdateHandler func(ctx context.Context, orgID string)

type Subscriber struct {
	rdb      *goredis.Client
	handlers []UpdateHandler
}

func NewSubscriber(rdb *goredis.Client) *Subscriber {
	return &Subscriber{rdb: rdb}
}

// OnUpdate registers a handler. Call before Run().
func (s *Subscriber) OnUpdate(h UpdateHandler) {
	s.handlers = append(s.handlers, h)
}

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

	for _, h := range s.handlers {
		h(ctx, orgID)
	}
}
