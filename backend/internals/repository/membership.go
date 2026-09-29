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

type MembershipWithUser struct {
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Role      string    `json:"role"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *MembershipRepo) ListByOrg(ctx context.Context, q DBTX, orgID string) ([]*MembershipWithUser, error) {
	rows, err := q.Query(ctx, `
		SELECT m.user_id, m.org_id, m.role, u.email, m.created_at, m.updated_at
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1
		ORDER BY m.created_at ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*MembershipWithUser
	for rows.Next() {
		m := &MembershipWithUser{}
		if err := rows.Scan(&m.UserID, &m.OrgID, &m.Role, &m.Email, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MembershipRepo) UpdateRole(ctx context.Context, q DBTX, userID, orgID, role string) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE memberships SET role = $3, updated_at = now()
		WHERE user_id = $1 AND org_id = $2
	`, userID, orgID, role)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *MembershipRepo) Delete(ctx context.Context, q DBTX, userID, orgID string) (bool, error) {
	tag, err := q.Exec(ctx, `
		DELETE FROM memberships WHERE user_id = $1 AND org_id = $2
	`, userID, orgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CountOwners returns the number of owners in the organization. It is used for protection
// since removing/demoting the last owner.
func (r *MembershipRepo) CountOwners(ctx context.Context, q DBTX, orgID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*) FROM memberships WHERE org_id = $1 AND role = 'owner'
	`, orgID).Scan(&n)
	return n, err
}
