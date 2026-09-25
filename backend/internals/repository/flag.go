package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Flag struct {
	ID          string
	OrgID       string
	Key         string
	Name        string
	Description string
	FlagType    string // boolean | string | number | json
	Archived    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type FlagRepo struct{}

func NewFlagRepo() *FlagRepo { return &FlagRepo{} }

func (r *FlagRepo) Create(ctx context.Context, q DBTX, orgID, key, name, description, flagType string) (*Flag, error) {
	f := &Flag{}
	err := q.QueryRow(ctx, `
		INSERT INTO flags (org_id, key, name, description, flag_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, key, name, description, flag_type, archived, created_at, updated_at
	`, orgID, key, name, description, flagType).
		Scan(&f.ID, &f.OrgID, &f.Key, &f.Name, &f.Description, &f.FlagType, &f.Archived, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FlagRepo) FindByID(ctx context.Context, q DBTX, orgID, flagID string) (*Flag, error) {
	f := &Flag{}
	err := q.QueryRow(ctx, `
		SELECT id, org_id, key, name, description, flag_type, archived, created_at, updated_at
		FROM flags WHERE id = $1 AND org_id = $2
	`, flagID, orgID).
		Scan(&f.ID, &f.OrgID, &f.Key, &f.Name, &f.Description, &f.FlagType, &f.Archived, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FlagRepo) FindByKey(ctx context.Context, q DBTX, orgID, key string) (*Flag, error) {
	f := &Flag{}
	err := q.QueryRow(ctx, `
		SELECT id, org_id, key, name, description, flag_type, archived, created_at, updated_at
		FROM flags WHERE org_id = $1 AND key = $2
	`, orgID, key).
		Scan(&f.ID, &f.OrgID, &f.Key, &f.Name, &f.Description, &f.FlagType, &f.Archived, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

type ListFlagsParams struct {
	OrgID           string
	IncludeArchived bool
	Search          string // key or name
	Limit           int
	Offset          int
}

func (r *FlagRepo) List(ctx context.Context, q DBTX, p ListFlagsParams) ([]*Flag, int, error) {
	rows, err := q.Query(ctx, `
		SELECT id, org_id, key, name, description, flag_type, archived, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM flags
		WHERE org_id = $1
		  AND ($2::bool OR archived = false)
		  AND ($3::text = '' OR key ILIKE '%' || $3 || '%' OR name ILIKE '%' || $3 || '%')
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`, p.OrgID, p.IncludeArchived, p.Search, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*Flag
	var total int
	for rows.Next() {
		f := &Flag{}
		if err := rows.Scan(&f.ID, &f.OrgID, &f.Key, &f.Name, &f.Description, &f.FlagType,
			&f.Archived, &f.CreatedAt, &f.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

// UpdateMetadata menja samo name i description. Key i type su immutable.
func (r *FlagRepo) UpdateMetadata(ctx context.Context, q DBTX, orgID, flagID, name, description string) (*Flag, error) {
	f := &Flag{}
	err := q.QueryRow(ctx, `
		UPDATE flags
		SET name = $3, description = $4, updated_at = now()
		WHERE id = $1 AND org_id = $2
		RETURNING id, org_id, key, name, description, flag_type, archived, created_at, updated_at
	`, flagID, orgID, name, description).
		Scan(&f.ID, &f.OrgID, &f.Key, &f.Name, &f.Description, &f.FlagType, &f.Archived, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Archive postavlja archived = true (soft delete).
func (r *FlagRepo) Archive(ctx context.Context, q DBTX, orgID, flagID string) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE flags SET archived = true, updated_at = now()
		WHERE id = $1 AND org_id = $2 AND archived = false
	`, flagID, orgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
