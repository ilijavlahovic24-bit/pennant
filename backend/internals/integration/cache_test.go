//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"pennant/backend/internals/evaluation"
)

func TestCache_SetGet(t *testing.T) {
	resetRedis(t)

	cache := evaluation.NewCache(rdb)
	ctx := context.Background()

	snap := &evaluation.FlagSnapshot{
		FlagKey:        "f",
		FlagType:       "boolean",
		Enabled:        true,
		RolloutPercent: 50,
	}
	if err := cache.Set(ctx, "org1", "production", "f", snap); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := cache.Get(ctx, "org1", "production", "f")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	if !got.Enabled || got.RolloutPercent != 50 {
		t.Fatalf("snapshot mismatch: %+v", got)
	}
}

func TestCache_GetMissing(t *testing.T) {
	resetRedis(t)

	cache := evaluation.NewCache(rdb)
	got, err := cache.Get(context.Background(), "nope", "production", "x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing key, got %+v", got)
	}
}

func TestCache_InvalidateOrg(t *testing.T) {
	resetRedis(t)

	cache := evaluation.NewCache(rdb)
	ctx := context.Background()

	// Dva ključa za org1, jedan za org2.
	_ = cache.Set(ctx, "org1", "production", "a", &evaluation.FlagSnapshot{FlagKey: "a"})
	_ = cache.Set(ctx, "org1", "staging", "b", &evaluation.FlagSnapshot{FlagKey: "b"})
	_ = cache.Set(ctx, "org2", "production", "c", &evaluation.FlagSnapshot{FlagKey: "c"})

	if err := cache.InvalidateOrg(ctx, "org1"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	if v, _ := cache.Get(ctx, "org1", "production", "a"); v != nil {
		t.Fatalf("expected org1/a invalidated, got %+v", v)
	}
	if v, _ := cache.Get(ctx, "org1", "staging", "b"); v != nil {
		t.Fatalf("expected org1/b invalidated, got %+v", v)
	}
	if v, _ := cache.Get(ctx, "org2", "production", "c"); v == nil {
		t.Fatalf("expected org2/c to survive")
	}
}

func TestPublisher_Subscriber_Invalidates(t *testing.T) {
	resetRedis(t)

	cache := evaluation.NewCache(rdb)
	pub := evaluation.NewPublisher(rdb)
	sub := evaluation.NewSubscriber(rdb, cache)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Run(ctx)
	// Sačekaj da subscriber uspostavi PSubscribe.
	time.Sleep(150 * time.Millisecond)

	// Postavi cache.
	if err := cache.Set(ctx, "org-x", "production", "f",
		&evaluation.FlagSnapshot{FlagKey: "f", Enabled: true}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Publish — invalidiraj.
	if err := pub.PublishFlagUpdate(ctx, "org-x"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Sačekaj da subscriber obradi.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		v, err := cache.Get(ctx, "org-x", "production", "f")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if v == nil {
			return // uspešno invalidirano
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("cache was not invalidated within timeout")
}

func TestPublisher_IsolatedPerOrg(t *testing.T) {
	resetRedis(t)

	cache := evaluation.NewCache(rdb)
	pub := evaluation.NewPublisher(rdb)
	sub := evaluation.NewSubscriber(rdb, cache)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Run(ctx)
	time.Sleep(150 * time.Millisecond)

	// Cache za org-a i org-b.
	_ = cache.Set(ctx, "org-a", "production", "f", &evaluation.FlagSnapshot{FlagKey: "f"})
	_ = cache.Set(ctx, "org-b", "production", "f", &evaluation.FlagSnapshot{FlagKey: "f"})

	// Publish samo za org-a.
	if err := pub.PublishFlagUpdate(ctx, "org-a"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Sačekaj da org-a bude invalidiran.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := cache.Get(ctx, "org-a", "production", "f")
		if a == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	a, _ := cache.Get(ctx, "org-a", "production", "f")
	if a != nil {
		t.Fatalf("expected org-a invalidated")
	}

	// org-b mora ostati netaknut.
	b, _ := cache.Get(ctx, "org-b", "production", "f")
	if b == nil {
		t.Fatalf("expected org-b to survive cross-org publish")
	}
}
