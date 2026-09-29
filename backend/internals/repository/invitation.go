package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Invitation struct {
	ID         string
	OrgID      string
	Email      string
	Role       string
	TokenHash  string
	InvitedBy  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

type InvitationRepo struct{}

func NewInvitationRepo() *InvitationRepo { return &InvitationRepo{} }

func (r *InvitationRepo) Create(
	ctx context.Context,
	q DBTX,
	orgID, email, role, tokenHash, invitedBy string,
	expiresAt time.Time,
) (*Invitation, error) {
	inv := &Invitation{}
	err := q.QueryRow(ctx, `
		INSERT INTO invitations (org_id, email, role, token, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, org_id, email, role, token, invited_by, expires_at, accepted_at, created_at
	`, orgID, email, role, tokenHash, invitedBy, expiresAt).
		Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash,
			&inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *InvitationRepo) FindByTokenHash(ctx context.Context, q DBTX, hash string) (*Invitation, error) {
	inv := &Invitation{}
	err := q.QueryRow(ctx, `
		SELECT id, org_id, email, role, token, invited_by, expires_at, accepted_at, created_at
		FROM invitations WHERE token = $1
	`, hash).
		Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash,
			&inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// Mark accepted sets accepted_at = now() only if it is not already set.
// Returns true if successful (single-use guarantee).
func (r *InvitationRepo) MarkAccepted(ctx context.Context, q DBTX, id string) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE invitations SET accepted_at = now()
		WHERE id = $1 AND accepted_at IS NULL
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *InvitationRepo) ListByOrg(ctx context.Context, q DBTX, orgID string) ([]*Invitation, error) {
	rows, err := q.Query(ctx, `
		SELECT id, org_id, email, role, token, invited_by, expires_at, accepted_at, created_at
		FROM invitations
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Invitation
	for rows.Next() {
		inv := &Invitation{}
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash,
			&inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Delete remove invitation (revoke). Owner can revoke a pending invitation.
func (r *InvitationRepo) Delete(ctx context.Context, q DBTX, orgID, id string) (bool, error) {
	tag, err := q.Exec(ctx, `
		DELETE FROM invitations WHERE id = $1 AND org_id = $2 AND accepted_at IS NULL
	`, id, orgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
