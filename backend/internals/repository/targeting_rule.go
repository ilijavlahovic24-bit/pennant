package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type TargetingRule struct {
	ID          string
	FlagEnvID   string
	Priority    int
	Attribute   string
	Operator    string
	Value       json.RawMessage
	Action      string
	ActionValue json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TargetingRuleRepo struct{}

func NewTargetingRuleRepo() *TargetingRuleRepo { return &TargetingRuleRepo{} }

func (r *TargetingRuleRepo) ListByFlagEnv(ctx context.Context, q DBTX, flagEnvID string) ([]*TargetingRule, error) {
	rows, err := q.Query(ctx, `
		SELECT id, flag_env_id, priority, attribute, operator, value,
		       action, action_value, created_at, updated_at
		FROM targeting_rules
		WHERE flag_env_id = $1
		ORDER BY priority ASC
	`, flagEnvID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*TargetingRule
	for rows.Next() {
		tr := &TargetingRule{}
		if err := rows.Scan(&tr.ID, &tr.FlagEnvID, &tr.Priority, &tr.Attribute,
			&tr.Operator, &tr.Value, &tr.Action, &tr.ActionValue,
			&tr.CreatedAt, &tr.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (r *TargetingRuleRepo) Create(ctx context.Context, q DBTX, in CreateRuleParams) (*TargetingRule, error) {
	tr := &TargetingRule{}
	err := q.QueryRow(ctx, `
		INSERT INTO targeting_rules (flag_env_id, priority, attribute, operator, value, action, action_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, flag_env_id, priority, attribute, operator, value,
		          action, action_value, created_at, updated_at
	`, in.FlagEnvID, in.Priority, in.Attribute, in.Operator, in.Value, in.Action, in.ActionValue).
		Scan(&tr.ID, &tr.FlagEnvID, &tr.Priority, &tr.Attribute, &tr.Operator,
			&tr.Value, &tr.Action, &tr.ActionValue, &tr.CreatedAt, &tr.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

type CreateRuleParams struct {
	FlagEnvID   string
	Priority    int
	Attribute   string
	Operator    string
	Value       json.RawMessage
	Action      string
	ActionValue json.RawMessage
}

// ReplaceAll clears all rules for flag_env and writes a new set.
// Transactional - called inside BeginTx.
func (r *TargetingRuleRepo) ReplaceAll(ctx context.Context, q DBTX, flagEnvID string, rules []CreateRuleParams) error {
	if _, err := q.Exec(ctx, `DELETE FROM targeting_rules WHERE flag_env_id = $1`, flagEnvID); err != nil {
		return err
	}
	for i, rule := range rules {
		_, err := q.Exec(ctx, `
			INSERT INTO targeting_rules (flag_env_id, priority, attribute, operator, value, action, action_value)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, flagEnvID, i, rule.Attribute, rule.Operator, rule.Value, rule.Action, rule.ActionValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *TargetingRuleRepo) Delete(ctx context.Context, q DBTX, flagEnvID, ruleID string) (bool, error) {
	tag, err := q.Exec(ctx, `
		DELETE FROM targeting_rules WHERE id = $1 AND flag_env_id = $2
	`, ruleID, flagEnvID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// NextPriority returns the next available priority (max + 1, or 0 if empty).
func (r *TargetingRuleRepo) NextPriority(ctx context.Context, q DBTX, flagEnvID string) (int, error) {
	var next int
	err := q.QueryRow(ctx, `
		SELECT COALESCE(MAX(priority) + 1, 0) FROM targeting_rules WHERE flag_env_id = $1
	`, flagEnvID).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return next, err
}
