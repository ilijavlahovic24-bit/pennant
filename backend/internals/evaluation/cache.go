package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const cacheTTL = 5 * time.Minute

type Cache struct {
	rdb *goredis.Client
}

func NewCache(rdb *goredis.Client) *Cache {
	return &Cache{rdb: rdb}
}

func cacheKey(orgID, envSlug, flagKey string) string {
	return fmt.Sprintf("flag:%s:%s:%s", orgID, envSlug, flagKey)
}

func (c *Cache) Get(ctx context.Context, orgID, envSlug, flagKey string) (*FlagSnapshot, error) {
	data, err := c.rdb.Get(ctx, cacheKey(orgID, envSlug, flagKey)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s FlagSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Cache) Set(ctx context.Context, orgID, envSlug, flagKey string, s *FlagSnapshot) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, cacheKey(orgID, envSlug, flagKey), data, cacheTTL).Err()
}

// InvalidateOrg briše sve flag cache ključeve za datu organizaciju.
func (c *Cache) InvalidateOrg(ctx context.Context, orgID string) error {
	pattern := fmt.Sprintf("flag:%s:*", orgID)
	iter := c.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.rdb.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}
