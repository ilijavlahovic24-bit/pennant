package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// Publisher objavljuje događaje o izmenama flagova na Redis pub/sub.
type Publisher struct {
	rdb *goredis.Client
}

func NewPublisher(rdb *goredis.Client) *Publisher {
	return &Publisher{rdb: rdb}
}

func channelFor(orgID string) string {
	return fmt.Sprintf("flag-updates:%s", orgID)
}

type flagUpdateEvent struct {
	OrgID string `json:"org_id"`
}

// PublishFlagUpdate se poziva POSLE uspešnog commit-a izmene.
// Subscriber na drugoj strani invalidira cache.
func (p *Publisher) PublishFlagUpdate(ctx context.Context, orgID string) error {
	payload, err := json.Marshal(flagUpdateEvent{OrgID: orgID})
	if err != nil {
		return err
	}
	return p.rdb.Publish(ctx, channelFor(orgID), payload).Err()
}
