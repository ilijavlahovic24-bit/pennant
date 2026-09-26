package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type EnvironmentRepo struct{}

func NewEnvironmentRepo() *EnvironmentRepo { return &EnvironmentRepo{} }

// CreateDefaults creates development/staging/production for a new org.
func (r *EnvironmentRepo) CreateDefaults(ctx context.Context, q DBTX, orgID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO environments (org_id, name, slug) VALUES
			($1, 'Development', 'development'),
			($1, 'Staging',     'staging'),
			($1, 'Production',  'production')
	`, orgID)
	return err
}
func (r *EnvironmentRepo) FindBySlug(ctx context.Context, q DBTX, orgID, slug string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `
		SELECT id FROM environments WHERE org_id = $1 AND slug = $2
	`, orgID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}
