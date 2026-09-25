package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/config"
	"pennant/backend/internals/repository"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Service struct {
	cfg     *config.Config
	pool    *pgxpool.Pool
	users   *repository.UserRepo
	orgs    *repository.OrgRepo
	members *repository.MembershipRepo
	envs    *repository.EnvironmentRepo
	tokens  *repository.RefreshTokenRepo
}

func NewService(cfg *config.Config, pool *pgxpool.Pool) *Service {
	return &Service{
		cfg:     cfg,
		pool:    pool,
		users:   repository.NewUserRepo(),
		orgs:    repository.NewOrgRepo(),
		members: repository.NewMembershipRepo(),
		envs:    repository.NewEnvironmentRepo(),
		tokens:  repository.NewRefreshTokenRepo(),
	}
}

type RegisterInput struct {
	Email    string
	Password string
	OrgName  string
}

type AuthResult struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	OrgID        string
	Role         string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.OrgName = strings.TrimSpace(in.OrgName)

	if !emailRe.MatchString(in.Email) {
		return nil, ErrInvalidEmail
	}
	if len(in.Password) < 8 {
		return nil, ErrWeakPassword
	}
	if in.OrgName == "" {
		in.OrgName = in.Email
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := s.users.FindByEmail(ctx, tx, in.Email)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailTaken
	}

	user, err := s.users.Create(ctx, tx, in.Email, hash)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	slug, err := s.uniqueSlug(ctx, tx, in.OrgName)
	if err != nil {
		return nil, err
	}

	org, err := s.orgs.Create(ctx, tx, in.OrgName, slug)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	if err := s.members.Create(ctx, tx, user.ID, org.ID, "owner"); err != nil {
		return nil, fmt.Errorf("create membership: %w", err)
	}

	if err := s.envs.CreateDefaults(ctx, tx, org.ID); err != nil {
		return nil, fmt.Errorf("create default envs: %w", err)
	}

	result, err := s.issueTokens(ctx, tx, user.ID, org.ID, "owner")
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	user, err := s.users.FindByEmail(ctx, s.pool, email)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil || !verifyPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	m, err := s.members.FindFirstByUser(ctx, s.pool, user.ID)
	if err != nil {
		return nil, fmt.Errorf("find membership: %w", err)
	}
	if m == nil {
		return nil, ErrNoMembership
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := s.issueTokens(ctx, tx, user.ID, m.OrgID, m.Role)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// Refresh rotira refresh token. Stari se revokuje, novi se izdaje.
// Ako je stari već revokovan → replay napad → revokujemo sve tokene korisnika.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResult, error) {
	hash := HashRefreshToken(refreshToken)

	stored, err := s.tokens.FindByHash(ctx, s.pool, hash)
	if err != nil {
		return nil, fmt.Errorf("find token: %w", err)
	}
	if stored == nil {
		return nil, ErrInvalidToken
	}
	if stored.RevokedAt != nil {
		// Replay — revokuj sve tokene korisnika.
		_ = s.tokens.RevokeAllForUser(ctx, s.pool, stored.UserID)
		return nil, ErrTokenRevoked
	}
	if time.Now().After(stored.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	m, err := s.members.FindFirstByUser(ctx, s.pool, stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("find membership: %w", err)
	}
	if m == nil {
		return nil, ErrNoMembership
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.tokens.Revoke(ctx, tx, hash); err != nil {
		return nil, fmt.Errorf("revoke old token: %w", err)
	}

	result, err := s.issueTokens(ctx, tx, stored.UserID, m.OrgID, m.Role)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.tokens.Revoke(ctx, s.pool, HashRefreshToken(refreshToken))
}

func (s *Service) issueTokens(ctx context.Context, q repository.DBTX, userID, orgID, role string) (*AuthResult, error) {
	access, err := GenerateAccessToken(s.cfg.JWTSecret, userID, orgID, role, s.cfg.AccessTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access: %w", err)
	}

	refresh, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	if err := s.tokens.Create(ctx, q, userID, HashRefreshToken(refresh), time.Now().Add(s.cfg.RefreshTTL)); err != nil {
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	return &AuthResult{
		AccessToken:  access,
		RefreshToken: refresh,
		UserID:       userID,
		OrgID:        orgID,
		Role:         role,
	}, nil
}

func (s *Service) uniqueSlug(ctx context.Context, q repository.DBTX, name string) (string, error) {
	base := slugify(name)
	for i := 0; i < 5; i++ {
		suffix := randomHex(3) // 6 hex chars
		slug := base + "-" + suffix
		existing, err := s.orgs.FindByID(ctx, q, "") // placeholder — see note below
		_ = existing
		_ = err
		_ = slug
		// Real check: see repository helper FindBySlug
		if !s.slugExists(ctx, q, slug) {
			return slug, nil
		}
	}
	return "", errors.New("could not generate unique slug")
}

func (s *Service) slugExists(ctx context.Context, q repository.DBTX, slug string) bool {
	var exists bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE slug = $1)`, slug).Scan(&exists)
	return exists
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "org"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
