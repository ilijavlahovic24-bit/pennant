//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"pennant/backend/internals/evaluation"
)

type seedOpts struct {
	OrgName  string
	OrgSlug  string
	FlagKey  string
	FlagType string
	Archived bool
	Enabled  bool
	Rollout  int
	Value    json.RawMessage
}

// seedFlag kreira org + env (production) + flag + flag_env u jednoj transakciji.
func seedFlag(t *testing.T, opts seedOpts) (orgID, envID, flagID string) {
	t.Helper()
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		opts.OrgName, opts.OrgSlug).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO environments (org_id, name, slug)
		 VALUES ($1, 'Production', 'production') RETURNING id`,
		orgID).Scan(&envID); err != nil {
		t.Fatalf("insert env: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO flags (org_id, key, name, description, flag_type, archived)
		 VALUES ($1, $2, $3, '', $4, $5) RETURNING id`,
		orgID, opts.FlagKey, opts.FlagKey, opts.FlagType, opts.Archived).Scan(&flagID); err != nil {
		t.Fatalf("insert flag: %v", err)
	}

	var value any
	if opts.Value != nil {
		value = opts.Value
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO flag_environments (flag_id, env_id, enabled, rollout_percent, value)
		 VALUES ($1, $2, $3, $4, $5)`,
		flagID, envID, opts.Enabled, opts.Rollout, value); err != nil {
		t.Fatalf("insert flag_env: %v", err)
	}

	return orgID, envID, flagID
}

// seedRule dodaje targeting pravilo u flag_env.
func seedRule(t *testing.T, flagID, envID string, priority int,
	attribute, operator, value, action string) {
	t.Helper()
	ctx := context.Background()

	var flagEnvID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM flag_environments WHERE flag_id = $1 AND env_id = $2`,
		flagID, envID).Scan(&flagEnvID); err != nil {
		t.Fatalf("find flag_env: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO targeting_rules (flag_env_id, priority, attribute, operator, value, action)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		flagEnvID, priority, attribute, operator, value, action); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
}

func newEngine() *evaluation.Engine {
	return evaluation.NewEngine(pool, evaluation.NewCache(rdb))
}

func TestEvaluate_FlagNotFound(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "existing",
		FlagType: "boolean", Enabled: true, Rollout: 100,
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "production", "missing", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled || res.Reason != "not_found" {
		t.Fatalf("expected not_found, got %+v", res)
	}
}

func TestEvaluate_EnvNotFound(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 100,
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "nonexistent-env", "f", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled || res.Reason != "not_found" {
		t.Fatalf("expected not_found for missing env, got %+v", res)
	}
}

func TestEvaluate_Archived(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Archived: true, Enabled: true, Rollout: 100,
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "production", "f", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled || res.Reason != "archived" {
		t.Fatalf("expected archived, got %+v", res)
	}
}

func TestEvaluate_Disabled(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: false, Rollout: 100,
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "production", "f", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled || res.Reason != "disabled" {
		t.Fatalf("expected disabled, got %+v", res)
	}
}

func TestEvaluate_Rollout100(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 100,
		Value: json.RawMessage(`true`),
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "production", "f", evaluation.UserContext{UserID: "u-42"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !res.Enabled || res.Reason != "rollout" {
		t.Fatalf("expected rollout enabled, got %+v", res)
	}
	if string(res.Value) != "true" {
		t.Fatalf("expected value true, got %s", res.Value)
	}
}

func TestEvaluate_Rollout0(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 0,
	})

	res, err := newEngine().Evaluate(context.Background(),
		orgID, "production", "f", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled || res.Reason != "rollout" {
		t.Fatalf("expected rollout disabled, got %+v", res)
	}
}

func TestEvaluate_Deterministic(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 50,
	})

	engine := newEngine()
	ctx := context.Background()

	first, err := engine.Evaluate(ctx, orgID, "production", "f", evaluation.UserContext{UserID: "u-42"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	for i := 0; i < 20; i++ {
		got, err := engine.Evaluate(ctx, orgID, "production", "f", evaluation.UserContext{UserID: "u-42"})
		if err != nil {
			t.Fatalf("evaluate %d: %v", i, err)
		}
		if got.Enabled != first.Enabled {
			t.Fatalf("not deterministic at iteration %d: %v vs %v", i, first.Enabled, got.Enabled)
		}
	}
}

func TestEvaluate_TargetingMatch(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, envID, flagID := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 0,
	})

	seedRule(t, flagID, envID, 0, "plan", "equals", `"enterprise"`, "serve_enabled")

	engine := newEngine()
	ctx := context.Background()

	// enterprise → targeting_match enabled
	res, err := engine.Evaluate(ctx, orgID, "production", "f",
		evaluation.UserContext{UserID: "u", Attributes: map[string]any{"plan": "enterprise"}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !res.Enabled || res.Reason != "targeting_match" {
		t.Fatalf("expected targeting_match enabled, got %+v", res)
	}

	// free → pada na rollout 0 → disabled
	res2, err := engine.Evaluate(ctx, orgID, "production", "f",
		evaluation.UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res2.Enabled {
		t.Fatalf("expected disabled for free plan, got %+v", res2)
	}
}

func TestEvaluate_TargetingPriority(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, envID, flagID := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 100,
	})

	// Priority 0: ako je plan=free → serve_disabled (pobeđuje rollout 100%)
	// Priority 1: ako je plan=pro → serve_enabled
	seedRule(t, flagID, envID, 0, "plan", "equals", `"free"`, "serve_disabled")
	seedRule(t, flagID, envID, 1, "plan", "equals", `"pro"`, "serve_enabled")

	engine := newEngine()
	ctx := context.Background()

	// free → serve_disabled (priority 0 match)
	res, err := engine.Evaluate(ctx, orgID, "production", "f",
		evaluation.UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Enabled {
		t.Fatalf("expected disabled for free, got %+v", res)
	}

	// pro → serve_enabled (priority 1 match)
	res2, err := engine.Evaluate(ctx, orgID, "production", "f",
		evaluation.UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !res2.Enabled {
		t.Fatalf("expected enabled for pro, got %+v", res2)
	}
}

func TestEvaluate_CacheHit(t *testing.T) {
	resetDB(t)
	resetRedis(t)

	orgID, _, _ := seedFlag(t, seedOpts{
		OrgName: "E", OrgSlug: "e", FlagKey: "f",
		FlagType: "boolean", Enabled: true, Rollout: 100,
	})

	engine := newEngine()
	ctx := context.Background()

	// Prvi poziv — cache miss, učitava iz DB i kešira.
	if _, err := engine.Evaluate(ctx, orgID, "production", "f", evaluation.UserContext{UserID: "u"}); err != nil {
		t.Fatalf("first evaluate: %v", err)
	}

	// Cache ključ mora postojati.
	keys, err := rdb.Keys(ctx, "flag:*").Result()
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 cache key, got %d: %v", len(keys), keys)
	}

	// Obriši flag iz DB-a — ako drugi poziv čita iz cache-a, neće biti not_found.
	if _, err := pool.Exec(ctx, `DELETE FROM flags`); err != nil {
		t.Fatalf("delete flags: %v", err)
	}

	res, err := engine.Evaluate(ctx, orgID, "production", "f", evaluation.UserContext{UserID: "u"})
	if err != nil {
		t.Fatalf("second evaluate: %v", err)
	}
	if !res.Enabled {
		t.Fatalf("expected cache hit (enabled), got %+v", res)
	}
}
