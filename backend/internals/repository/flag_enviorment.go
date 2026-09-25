package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type FlagEnvironment struct {
	ID             string
	FlagID         string
	EnvID          string
	Enabled        bool
	RolloutPercent int
	Value          json.RawMessage // može biti nil
	ExpiresAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// FlagEnvironmentDetailed je za odgovor API-ja — sadrži i env slug/name.
type FlagEnvironmentDetailed struct {
	FlagEnvironment
	EnvSlug string
	EnvName string
}

type FlagEnvRepo struct{}

func NewFlagEnvRepo() *FlagEnvRepo { return &FlagEnvRepo{} }

// CreateForAllEnvironments pravi red u flag_environments za svaki env u organizaciji.
func (r *FlagEnvRepo) CreateForAllEnvironments(ctx context.Context, q DBTX, flagID, orgID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO flag_environments (flag_id, env_id)
		SELECT $1, e.id FROM environments e WHERE e.org_id = $2
	`, flagID, orgID)
	return err
}

func (r *FlagEnvRepo) ListByFlag(ctx context.Context, q DBTX, flagID string) ([]*FlagEnvironmentDetailed, error) {
	rows, err := q.Query(ctx, `
		SELECT fe.id, fe.flag_id, fe.env_id, fe.enabled, fe.rollout_percent,
		       fe.value, fe.expires_at, fe.created_at, fe.updated_at,
		       e.slug, e.name
		FROM flag_environments fe
		JOIN environments e ON e.id = fe.env_id
		WHERE fe.flag_id = $1
		ORDER BY e.slug ASC
	`, flagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*FlagEnvironmentDetailed
	for rows.Next() {
		fe := &FlagEnvironmentDetailed{}
		if err := rows.Scan(
			&fe.ID, &fe.FlagID, &fe.EnvID, &fe.Enabled, &fe.RolloutPercent,
			&fe.Value, &fe.ExpiresAt, &fe.CreatedAt, &fe.UpdatedAt,
			&fe.EnvSlug, &fe.EnvName,
		); err != nil {
			return nil, err
		}
		out = append(out, fe)
	}
	return out, rows.Err()
}

// ListByFlagIDs vraća env stanja za više flagova odjednom (za list view).
func (r *FlagEnvRepo) ListByFlagIDs(ctx context.Context, q DBTX, flagIDs []string) (map[string][]*FlagEnvironmentDetailed, error) {
	if len(flagIDs) == 0 {
		return map[string][]*FlagEnvironmentDetailed{}, nil
	}
	rows, err := q.Query(ctx, `
		SELECT fe.id, fe.flag_id, fe.env_id, fe.enabled, fe.rollout_percent,
		       fe.value, fe.expires_at, fe.created_at, fe.updated_at,
		       e.slug, e.name
		FROM flag_environments fe
		JOIN environments e ON e.id = fe.env_id
		WHERE fe.flag_id = ANY($1)
		ORDER BY fe.flag_id, e.slug ASC
	`, flagIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]*FlagEnvironmentDetailed)
	for rows.Next() {
		fe := &FlagEnvironmentDetailed{}
		if err := rows.Scan(
			&fe.ID, &fe.FlagID, &fe.EnvID, &fe.Enabled, &fe.RolloutPercent,
			&fe.Value, &fe.ExpiresAt, &fe.CreatedAt, &fe.UpdatedAt,
			&fe.EnvSlug, &fe.EnvName,
		); err != nil {
			return nil, err
		}
		out[fe.FlagID] = append(out[fe.FlagID], fe)
	}
	return out, rows.Err()
}

func (r *FlagEnvRepo) FindByFlagAndEnv(ctx context.Context, q DBTX, flagID, envID string) (*FlagEnvironment, error) {
	fe := &FlagEnvironment{}
	err := q.QueryRow(ctx, `
		SELECT id, flag_id, env_id, enabled, rollout_percent, value, expires_at, created_at, updated_at
		FROM flag_environments WHERE flag_id = $1 AND env_id = $2
	`, flagID, envID).
		Scan(&fe.ID, &fe.FlagID, &fe.EnvID, &fe.Enabled, &fe.RolloutPercent,
			&fe.Value, &fe.ExpiresAt, &fe.CreatedAt, &fe.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fe, nil
}

// FindByFlagAndEnvSlug — za evaluaciju kad imamo env_slug.
func (r *FlagEnvRepo) FindByFlagAndEnvSlug(ctx context.Context, q DBTX, flagID, envSlug string) (*FlagEnvironment, error) {
	fe := &FlagEnvironment{}
	err := q.QueryRow(ctx, `
		SELECT fe.id, fe.flag_id, fe.env_id, fe.enabled, fe.rollout_percent,
		       fe.value, fe.expires_at, fe.created_at, fe.updated_at
		FROM flag_environments fe
		JOIN environments e ON e.id = fe.env_id
		WHERE fe.flag_id = $1 AND e.slug = $2
	`, flagID, envSlug).
		Scan(&fe.ID, &fe.FlagID, &fe.EnvID, &fe.Enabled, &fe.RolloutPercent,
			&fe.Value, &fe.ExpiresAt, &fe.CreatedAt, &fe.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fe, nil
}

type UpdateFlagEnvParams struct {
	Enabled        *bool
	RolloutPercent *int
	// Value se menja samo ako je ValueSet == true (da razlikujemo "nije poslato" od "poslato null")
	Value    json.RawMessage
	ValueSet bool
	// ExpiresAt — nil znači "nije poslato"; za brisanje koristi ClearExpiresAt
	ExpiresAt      *time.Time
	ClearExpiresAt bool
}

func (r *FlagEnvRepo) Update(ctx context.Context, q DBTX, flagID, envID string, p UpdateFlagEnvParams) (*FlagEnvironment, error) {
	fe := &FlagEnvironment{}
	err := q.QueryRow(ctx, `
		UPDATE flag_environments SET
			enabled         = COALESCE($3, enabled),
			rollout_percent = COALESCE($4, rollout_percent),
			value           = CASE WHEN $5::bool THEN $6::jsonb ELSE value END,
			expires_at      = CASE
			                     WHEN $7::bool THEN NULL
			                     WHEN $8::timestamptz IS NOT NULL THEN $8
			                     ELSE expires_at
			                  END,
			updated_at      = now()
		WHERE flag_id = $1 AND env_id = $2
		RETURNING id, flag_id, env_id, enabled, rollout_percent, value, expires_at, created_at, updated_at
	`, flagID, envID, p.Enabled, p.RolloutPercent, p.ValueSet, p.Value, p.ClearExpiresAt, p.ExpiresAt).
		Scan(&fe.ID, &fe.FlagID, &fe.EnvID, &fe.Enabled, &fe.RolloutPercent,
			&fe.Value, &fe.ExpiresAt, &fe.CreatedAt, &fe.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fe, nil
}
