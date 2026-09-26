package targeting

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:     pool,
		flagEnvs: repository.NewFlagEnvRepo(),
		rules:    repository.NewTargetingRuleRepo(),
	}
}

// RuleInput is the input for a single rule (without priority — priority is assigned by order).
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

// ReplaceAll replaces the entire set of rules in one go.
// Priority is assigned in the order of the input array (0, 1, 2, ...).
func (s *Service) ReplaceAll(ctx context.Context, orgID, flagID, envID string, inputs []RuleInput) ([]*repository.TargetingRule, error) {
	if len(inputs) > maxRulesPerEnv {
		return nil, ErrTooManyRules
	}

	flagEnv, err := s.resolveFlagEnv(ctx, orgID, flagID, envID)
	if err != nil {
		return nil, err
	}

	params := make([]repository.CreateRuleParams, len(inputs))
	for i, in := range inputs {
		if err := validateRule(in); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		params[i] = repository.CreateRuleParams{
			FlagEnvID:   flagEnv.ID,
			Priority:    i,
			Attribute:   in.Attribute,
			Operator:    in.Operator,
			Value:       in.Value,
			Action:      in.Action,
			ActionValue: in.ActionValue,
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.rules.ReplaceAll(ctx, tx, flagEnv.ID, params); err != nil {
		return nil, fmt.Errorf("replace rules: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.rules.ListByFlagEnv(ctx, s.pool, flagEnv.ID)
}

// Add dodaje jedno pravilo na kraj (next priority).
func (s *Service) Add(ctx context.Context, orgID, flagID, envID string, in RuleInput) (*repository.TargetingRule, error) {
	if err := validateRule(in); err != nil {
		return nil, err
	}

	flagEnv, err := s.resolveFlagEnv(ctx, orgID, flagID, envID)
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
		Attribute:   in.Attribute,
		Operator:    in.Operator,
		Value:       in.Value,
		Action:      in.Action,
		ActionValue: in.ActionValue,
	})
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return rule, nil
}

func (s *Service) Delete(ctx context.Context, orgID, flagID, envID, ruleID string) error {
	flagEnv, err := s.resolveFlagEnv(ctx, orgID, flagID, envID)
	if err != nil {
		return err
	}
	ok, err := s.rules.Delete(ctx, s.pool, flagEnv.ID, ruleID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// resolveFlagEnv checks that the flag belongs to the org and that the env exists for that flag.
func (s *Service) resolveFlagEnv(ctx context.Context, orgID, flagID, envID string) (*repository.FlagEnvironment, error) {
	// Prvo proveri da flag pripada org-i.
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

// validateRule checks attribute, operator, action, value, action_value.
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
		// action_value mora biti null ili prazno
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
