package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type RefreshTokenRepo struct{}

func NewRefreshTokenRepo() *RefreshTokenRepo { return &RefreshTokenRepo{} }

func (r *RefreshTokenRepo) Create(ctx context.Context, q DBTX, userID, hash string, expiresAt time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, hash, expiresAt)
	return err
}

func (r *RefreshTokenRepo) FindByHash(ctx context.Context, q DBTX, hash string) (*RefreshToken, error) {
	t := &RefreshToken{}
	err := q.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`, hash).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Revoke marks the token as used (single-use rotation).
func (r *RefreshTokenRepo) Revoke(ctx context.Context, q DBTX, hash string) error {
	_, err := q.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, hash)
	return err
}

// RevokeAllForUser is called when a replay attack is detected.
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, q DBTX, userID string) error {
	_, err := q.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}
