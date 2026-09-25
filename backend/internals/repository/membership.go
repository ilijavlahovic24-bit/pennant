package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Membership struct {
	UserID    string
	OrgID     string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type MembershipRepo struct{}

func NewMembershipRepo() *MembershipRepo { return &MembershipRepo{} }

func (r *MembershipRepo) Create(ctx context.Context, q DBTX, userID, orgID, role string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO memberships (user_id, org_id, role)
		VALUES ($1, $2, $3)
	`, userID, orgID, role)
	return err
}

func (r *MembershipRepo) Find(ctx context.Context, q DBTX, userID, orgID string) (*Membership, error) {
	m := &Membership{}
	err := q.QueryRow(ctx, `
		SELECT user_id, org_id, role, created_at, updated_at
		FROM memberships WHERE user_id = $1 AND org_id = $2
	`, userID, orgID).Scan(&m.UserID, &m.OrgID, &m.Role, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// FindFirstByUser returns the user's first membership (default org at login).
func (r *MembershipRepo) FindFirstByUser(ctx context.Context, q DBTX, userID string) (*Membership, error) {
	m := &Membership{}
	err := q.QueryRow(ctx, `
		SELECT user_id, org_id, role, created_at, updated_at
		FROM memberships WHERE user_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, userID).Scan(&m.UserID, &m.OrgID, &m.Role, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}
