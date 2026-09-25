package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Organization struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OrgRepo struct{}

func NewOrgRepo() *OrgRepo { return &OrgRepo{} }

func (r *OrgRepo) Create(ctx context.Context, q DBTX, name, slug string) (*Organization, error) {
	o := &Organization{}
	err := q.QueryRow(ctx, `
		INSERT INTO organizations (name, slug)
		VALUES ($1, $2)
		RETURNING id, name, slug, created_at, updated_at
	`, name, slug).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *OrgRepo) FindByID(ctx context.Context, q DBTX, id string) (*Organization, error) {
	o := &Organization{}
	err := q.QueryRow(ctx, `
		SELECT id, name, slug, created_at, updated_at
		FROM organizations WHERE id = $1
	`, id).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return o, nil
}
