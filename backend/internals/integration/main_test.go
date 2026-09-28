//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"pennant/backend/internals/config"
)

var (
	pool *pgxpool.Pool
	rdb  *goredis.Client
	cfg  *config.Config
)

func TestMain(m *testing.M) {
	cfg = &config.Config{
		Env:            "test",
		HTTPAddr:       ":0",
		DatabaseURL:    getenv("DATABASE_URL", "postgres://pennant:pennant@localhost:5432/pennant_test?sslmode=disable"),
		RedisURL:       getenv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:      getenv("JWT_SECRET", "test-secret"),
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     7 * 24 * time.Hour,
		ExpiryInterval: time.Minute,
	}

	ctx := context.Background()

	var err error
	pool, err = pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		panic("connect postgres: " + err.Error())
	}
	if err := pool.Ping(ctx); err != nil {
		panic("ping postgres: " + err.Error())
	}

	opt, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("parse redis url: " + err.Error())
	}
	rdb = goredis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic("ping redis: " + err.Error())
	}

	code := m.Run()

	pool.Close()
	_ = rdb.Close()
	os.Exit(code)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// resetDB briše sve podatke. CASCADE pokriva sve FK zavisnosti:
// organizations → environments, flags, memberships, invitations, audit_logs
// flags → flag_environments → targeting_rules
// users → memberships, refresh_tokens, invitations, audit_logs
func resetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE organizations, users CASCADE`); err != nil {
		t.Fatalf("reset db: %v", err)
	}
}

// resetRedis briše sve ključeve iz trenutne DB.
func resetRedis(t *testing.T) {
	t.Helper()
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("reset redis: %v", err)
	}
}
