package repository

import (
	"context"
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
