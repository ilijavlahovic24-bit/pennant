package targeting

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/audit"
	"pennant/backend/internals/repository"
)

const maxRulesPerEnv = 50

var validOperators = map[string]struct{}{
	"equals":     {},
	"not_equals": {},
	"in":         {},
	"not_in":     {},
	"contains":   {},
}

var validActions = map[string]struct{}{
	"serve_enabled":  {},
	"serve_disabled": {},
	"serve_percent":  {},
}

type Service struct {
	pool     *pgxpool.Pool
	flagEnvs *repository.FlagEnvRepo
	rules    *repository.TargetingRuleRepo
	audit    *audit.Service
}

func NewService(pool *pgxpool.Pool, auditSvc *audit.Service) *Service {
	return &Service{
		pool:     pool,
		flagEnvs: repository.NewFlagEnvRepo(),
		rules:    repository.NewTargetingRuleRepo(),
		audit:    auditSvc,
	}
}

type RuleInput struct {
	Attribute   string          `json:"attribute"`
	Operator    string          `json:"operator"`
	Value       json.RawMessage `json:"value"`
	Action      string          `json:"action"`
	ActionValue json.RawMessage `json:"action_value"`
}

func (s *Service) List(ctx context.Context, orgID, flagID, envID string) ([]*repository.TargetingRule, error) {
	flagEnv, err := s.resolveFlagEnv(ctx, orgID, flagID, envID)
	if err != nil {
		return nil, err
	}
	return s.rules.ListByFlagEnv(ctx, s.pool, flagEnv.ID)
}

type ReplaceInput struct {
	OrgID   string
	ActorID string
	FlagID  string
	EnvID   string
	Rules   []RuleInput
}

func (s *Service) ReplaceAll(ctx context.Context, in ReplaceInput) ([]*repository.TargetingRule, error) {
	if len(in.Rules) > maxRulesPerEnv {
		return nil, ErrTooManyRules
	}

	flagEnv, err := s.resolveFlagEnv(ctx, in.OrgID, in.FlagID, in.EnvID)
	if err != nil {
		return nil, err
	}

	params := make([]repository.CreateRuleParams, len(in.Rules))
	for i, r := range in.Rules {
		if err := validateRule(r); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		params[i] = repository.CreateRuleParams{
			FlagEnvID:   flagEnv.ID,
			Priority:    i,
			Attribute:   r.Attribute,
			Operator:    r.Operator,
			Value:       r.Value,
			Action:      r.Action,
			ActionValue: r.ActionValue,
		}
	}

	// Snimi before za audit.
	before, err := s.rules.ListByFlagEnv(ctx, s.pool, flagEnv.ID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.rules.ReplaceAll(ctx, tx, flagEnv.ID, params); err != nil {
		return nil, fmt.Errorf("replace rules: %w", err)
	}

	after, err := s.rules.ListByFlagEnv(ctx, tx, flagEnv.ID)
	if err != nil {
		return nil, err
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionTargetingReplace,
		ResourceType: audit.ResourceTypeTargetingRule,
		ResourceID:   flagEnv.ID,
		Before:       rulesToAudit(before),
		After:        rulesToAudit(after),
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.rules.ListByFlagEnv(ctx, s.pool, flagEnv.ID)
}

type AddInput struct {
	OrgID   string
	ActorID string
	FlagID  string
	EnvID   string
	Rule    RuleInput
}

func (s *Service) Add(ctx context.Context, in AddInput) (*repository.TargetingRule, error) {
	if err := validateRule(in.Rule); err != nil {
		return nil, err
	}

	flagEnv, err := s.resolveFlagEnv(ctx, in.OrgID, in.FlagID, in.EnvID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	next, err := s.rules.NextPriority(ctx, tx, flagEnv.ID)
	if err != nil {
		return nil, fmt.Errorf("next priority: %w", err)
	}
	if next >= maxRulesPerEnv {
		return nil, ErrTooManyRules
	}

	rule, err := s.rules.Create(ctx, tx, repository.CreateRuleParams{
		FlagEnvID:   flagEnv.ID,
		Priority:    next,
		Attribute:   in.Rule.Attribute,
		Operator:    in.Rule.Operator,
		Value:       in.Rule.Value,
		Action:      in.Rule.Action,
		ActionValue: in.Rule.ActionValue,
	})
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionTargetingAdd,
		ResourceType: audit.ResourceTypeTargetingRule,
		ResourceID:   rule.ID,
		Before:       nil,
		After:        ruleToAudit(rule),
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return rule, nil
}

type DeleteInput struct {
	OrgID   string
	ActorID string
	FlagID  string
	EnvID   string
	RuleID  string
}

func (s *Service) Delete(ctx context.Context, in DeleteInput) error {
	flagEnv, err := s.resolveFlagEnv(ctx, in.OrgID, in.FlagID, in.EnvID)
	if err != nil {
		return err
	}

	existing, err := s.rules.ListByFlagEnv(ctx, s.pool, flagEnv.ID)
	if err != nil {
		return err
	}
	var before *repository.TargetingRule
	for _, r := range existing {
		if r.ID == in.RuleID {
			before = r
			break
		}
	}
	if before == nil {
		return ErrNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ok, err := s.rules.Delete(ctx, tx, flagEnv.ID, in.RuleID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionTargetingDelete,
		ResourceType: audit.ResourceTypeTargetingRule,
		ResourceID:   in.RuleID,
		Before:       ruleToAudit(before),
		After:        nil,
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Service) resolveFlagEnv(ctx context.Context, orgID, flagID, envID string) (*repository.FlagEnvironment, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM flags WHERE id = $1 AND org_id = $2)`,
		flagID, orgID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	fe, err := s.flagEnvs.FindByFlagAndEnv(ctx, s.pool, flagID, envID)
	if err != nil {
		return nil, err
	}
	if fe == nil {
		return nil, ErrNotFound
	}
	return fe, nil
}

func validateRule(in RuleInput) error {
	if len(in.Attribute) == 0 || len(in.Attribute) > 100 {
		return ErrInvalidAttribute
	}
	if _, ok := validOperators[in.Operator]; !ok {
		return ErrInvalidOperator
	}
	if _, ok := validActions[in.Action]; !ok {
		return ErrInvalidAction
	}
	if err := validateValue(in.Operator, in.Value); err != nil {
		return err
	}
	if err := validateActionValue(in.Action, in.ActionValue); err != nil {
		return err
	}
	return nil
}

func validateValue(op string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return ErrInvalidValue
	}
	switch op {
	case "equals", "not_equals", "contains":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ErrInvalidValue
		}
	case "in", "not_in":
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil {
			return ErrInvalidValue
		}
	}
	return nil
}

func validateActionValue(action string, raw json.RawMessage) error {
	switch action {
	case "serve_enabled", "serve_disabled":
		if len(raw) > 0 && string(raw) != "null" {
			return ErrInvalidActionValue
		}
	case "serve_percent":
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return ErrInvalidActionValue
		}
		if n < 0 || n > 100 {
			return ErrInvalidActionValue
		}
	}
	return nil
}

func ruleToAudit(r *repository.TargetingRule) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"id":           r.ID,
		"priority":     r.Priority,
		"attribute":    r.Attribute,
		"operator":     r.Operator,
		"value":        rawToAny(r.Value),
		"action":       r.Action,
		"action_value": rawToAny(r.ActionValue),
	}
}

func rulesToAudit(rules []*repository.TargetingRule) []map[string]any {
	out := make([]map[string]any, len(rules))
	for i, r := range rules {
		out[i] = ruleToAudit(r)
	}
	return out
}

func rawToAny(r json.RawMessage) any {
	if len(r) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(r, &v); err != nil {
		return string(r)
	}
	return v
}
