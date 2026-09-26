package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/repository"
)

// Engine orkestrira evaluation flow: cache-aside + targeting + bucketing.
type Engine struct {
	pool     *pgxpool.Pool
	cache    *Cache
	flags    *repository.FlagRepo
	flagEnvs *repository.FlagEnvRepo
	rules    *repository.TargetingRuleRepo
}

func NewEngine(pool *pgxpool.Pool, cache *Cache) *Engine {
	return &Engine{
		pool:     pool,
		cache:    cache,
		flags:    repository.NewFlagRepo(),
		flagEnvs: repository.NewFlagEnvRepo(),
		rules:    repository.NewTargetingRuleRepo(),
	}
}

// Evaluate radi evaluaciju jednog flaga.
func (e *Engine) Evaluate(ctx context.Context, orgID, envSlug, flagKey string, user UserContext) (*Result, error) {
	snap, err := e.snapshot(ctx, orgID, envSlug, flagKey)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return &Result{FlagKey: flagKey, Enabled: false, Reason: "not_found"}, nil
	}
	return evaluateSnapshot(snap, flagKey, user), nil
}

// snapshot učitava iz cache-a ili (na miss) iz Postgres-a.
func (e *Engine) snapshot(ctx context.Context, orgID, envSlug, flagKey string) (*FlagSnapshot, error) {
	// 1. cache
	if snap, err := e.cache.Get(ctx, orgID, envSlug, flagKey); err == nil && snap != nil {
		return snap, nil
	}
	// Na grešku u cache-u, nastavljamo ka Postgres-u (fail-open).

	// 2. Postgres
	flag, err := e.flags.FindByKey(ctx, e.pool, orgID, flagKey)
	if err != nil {
		return nil, fmt.Errorf("find flag: %w", err)
	}
	if flag == nil {
		return nil, nil
	}

	flagEnv, err := e.flagEnvs.FindByFlagAndEnvSlug(ctx, e.pool, flag.ID, envSlug)
	if err != nil {
		return nil, fmt.Errorf("find flag env: %w", err)
	}
	if flagEnv == nil {
		return nil, nil
	}

	rawRules, err := e.rules.ListByFlagEnv(ctx, e.pool, flagEnv.ID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}

	rules := make([]RuleSnapshot, len(rawRules))
	for i, r := range rawRules {
		rules[i] = RuleSnapshot{
			Priority:    r.Priority,
			Attribute:   r.Attribute,
			Operator:    r.Operator,
			Value:       r.Value,
			Action:      r.Action,
			ActionValue: r.ActionValue,
		}
	}

	snap := &FlagSnapshot{
		FlagKey:        flag.Key,
		FlagType:       flag.FlagType,
		Archived:       flag.Archived,
		Enabled:        flagEnv.Enabled,
		RolloutPercent: flagEnv.RolloutPercent,
		Value:          flagEnv.Value,
		Rules:          rules,
	}

	// 3. upiši u cache (best-effort — ne obara evaluaciju ako Redis padne)
	_ = e.cache.Set(ctx, orgID, envSlug, flagKey, snap)

	return snap, nil
}

// evaluateSnapshot primenjuje evaluation flow po specifikaciji.
//
//  1. archived check
//  2. enabled check
//  3. targeting rules (po prioritetu)
//  4. deterministic bucket check (gradual rollout)
func evaluateSnapshot(snap *FlagSnapshot, flagKey string, user UserContext) *Result {
	if snap.Archived {
		return &Result{FlagKey: flagKey, Enabled: false, Reason: "archived"}
	}
	if !snap.Enabled {
		return &Result{FlagKey: flagKey, Enabled: false, Reason: "disabled"}
	}

	outcome := evaluateRules(snap.Rules, user)
	if outcome.matched {
		switch outcome.action {
		case "serve_enabled":
			return &Result{FlagKey: flagKey, Enabled: true, Value: snap.Value, Reason: "targeting_match"}
		case "serve_disabled":
			return &Result{FlagKey: flagKey, Enabled: false, Reason: "targeting_match"}
		case "serve_percent":
			enabled := ShouldServe(user.UserID, flagKey, outcome.actionPercent)
			return &Result{FlagKey: flagKey, Enabled: enabled, Value: valueIf(enabled, snap.Value), Reason: "targeting_match"}
		}
	}

	enabled := ShouldServe(user.UserID, flagKey, snap.RolloutPercent)
	return &Result{
		FlagKey: flagKey,
		Enabled: enabled,
		Value:   valueIf(enabled, snap.Value),
		Reason:  "rollout",
	}
}

func valueIf(enabled bool, v json.RawMessage) json.RawMessage {
	if !enabled {
		return nil
	}
	return v
}
