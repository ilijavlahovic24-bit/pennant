package members

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/audit"
	"pennant/backend/internals/config"
	"pennant/backend/internals/repository"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

var validRoles = map[string]struct{}{
	"owner":  {},
	"editor": {},
	"viewer": {},
}

type Service struct {
	pool        *pgxpool.Pool
	cfg         *config.Config
	members     *repository.MembershipRepo
	invitations *repository.InvitationRepo
	users       *repository.UserRepo
	orgs        *repository.OrgRepo
	audit       *audit.Service
}

func NewService(pool *pgxpool.Pool, cfg *config.Config, auditSvc *audit.Service) *Service {
	return &Service{
		pool:        pool,
		cfg:         cfg,
		members:     repository.NewMembershipRepo(),
		invitations: repository.NewInvitationRepo(),
		users:       repository.NewUserRepo(),
		orgs:        repository.NewOrgRepo(),
		audit:       auditSvc,
	}
}

// ---- Members ----

func (s *Service) ListMembers(ctx context.Context, orgID string) ([]*repository.MembershipWithUser, error) {
	return s.members.ListByOrg(ctx, s.pool, orgID)
}

type UpdateRoleInput struct {
	OrgID    string
	ActorID  string
	TargetID string
	NewRole  string
}

func (s *Service) UpdateRole(ctx context.Context, in UpdateRoleInput) error {
	if _, ok := validRoles[in.NewRole]; !ok {
		return ErrInvalidRole
	}
	if in.ActorID == in.TargetID {
		return ErrCannotModifySelf
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	target, err := s.members.Find(ctx, tx, in.TargetID, in.OrgID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrNotFound
	}

	// Ako demotujemo ownera, proveri da nije poslednji.
	if target.Role == "owner" && in.NewRole != "owner" {
		n, err := s.members.CountOwners(ctx, tx, in.OrgID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastOwner
		}
	}

	ok, err := s.members.UpdateRole(ctx, tx, in.TargetID, in.OrgID, in.NewRole)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionMembershipUpdate,
		ResourceType: audit.ResourceTypeMembership,
		ResourceID:   in.TargetID,
		Before:       map[string]any{"role": target.Role},
		After:        map[string]any{"role": in.NewRole},
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	return tx.Commit(ctx)
}

type RemoveInput struct {
	OrgID    string
	ActorID  string
	TargetID string
}

func (s *Service) RemoveMember(ctx context.Context, in RemoveInput) error {
	if in.ActorID == in.TargetID {
		return ErrCannotRemoveSelf
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	target, err := s.members.Find(ctx, tx, in.TargetID, in.OrgID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrNotFound
	}

	if target.Role == "owner" {
		n, err := s.members.CountOwners(ctx, tx, in.OrgID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastOwner
		}
	}

	ok, err := s.members.Delete(ctx, tx, in.TargetID, in.OrgID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionMembershipRemove,
		ResourceType: audit.ResourceTypeMembership,
		ResourceID:   in.TargetID,
		Before:       map[string]any{"role": target.Role},
		After:        nil,
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	return tx.Commit(ctx)
}

// ---- Invitations ----

type InviteInput struct {
	OrgID   string
	ActorID string
	Email   string
	Role    string
}

type InviteResult struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token"` // vraća se jednom, klijent šalje email
}

func (s *Service) Invite(ctx context.Context, in InviteInput) (*InviteResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailRe.MatchString(in.Email) {
		return nil, ErrInvalidEmail
	}
	if _, ok := validRoles[in.Role]; !ok {
		return nil, ErrInvalidRole
	}

	// Ako je korisnik već član — nema poente.
	existing, err := s.users.FindByEmail(ctx, s.pool, in.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		m, err := s.members.Find(ctx, s.pool, existing.ID, in.OrgID)
		if err != nil {
			return nil, err
		}
		if m != nil {
			return nil, ErrAlreadyMember
		}
	}

	raw, hash, err := generateInviteToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(s.cfg.InvitationTTL)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inv, err := s.invitations.Create(ctx, tx, in.OrgID, in.Email, in.Role, hash, in.ActorID, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("create invitation: %w", err)
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionMembershipInvite,
		ResourceType: audit.ResourceTypeInvitation,
		ResourceID:   inv.ID,
		Before:       nil,
		After:        map[string]any{"email": in.Email, "role": in.Role, "expires_at": expiresAt.UTC().Format(time.RFC3339)},
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &InviteResult{
		ID:        inv.ID,
		Email:     inv.Email,
		Role:      inv.Role,
		ExpiresAt: inv.ExpiresAt,
		Token:     raw,
	}, nil
}

func (s *Service) ListInvitations(ctx context.Context, orgID string) ([]*repository.Invitation, error) {
	return s.invitations.ListByOrg(ctx, s.pool, orgID)
}

type RevokeInviteInput struct {
	OrgID    string
	ActorID  string
	InviteID string
}

func (s *Service) RevokeInvite(ctx context.Context, in RevokeInviteInput) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ok, err := s.invitations.Delete(ctx, tx, in.OrgID, in.InviteID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInviteNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionMembershipRevoke,
		ResourceType: audit.ResourceTypeInvitation,
		ResourceID:   in.InviteID,
		Before:       map[string]any{"status": "pending"},
		After:        nil,
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	return tx.Commit(ctx)
}

type AcceptInviteInput struct {
	Token  string
	UserID string
}

type AcceptResult struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
}

func (s *Service) Accept(ctx context.Context, in AcceptInviteInput) (*AcceptResult, error) {
	hash := hashInviteToken(in.Token)

	inv, err := s.invitations.FindByTokenHash(ctx, s.pool, hash)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrInviteNotFound
	}
	if inv.AcceptedAt != nil {
		return nil, ErrInviteAlreadyUsed
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	user, err := s.users.FindByID(ctx, s.pool, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	if !strings.EqualFold(user.Email, inv.Email) {
		return nil, ErrEmailMismatch
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Single-use garancija.
	ok, err := s.invitations.MarkAccepted(ctx, tx, inv.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInviteAlreadyUsed
	}

	// Ako korisnik već je član (retry), ne pravi duplikat — ali smo već
	// markirali kao accepted, tako da je ovaj slučaj nemoguć u normalnom toku.
	if err := s.members.Create(ctx, tx, in.UserID, inv.OrgID, inv.Role); err != nil {
		return nil, fmt.Errorf("create membership: %w", err)
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        inv.OrgID,
		ActorID:      in.UserID,
		Action:       audit.ActionMembershipAccept,
		ResourceType: audit.ResourceTypeMembership,
		ResourceID:   in.UserID,
		Before:       nil,
		After:        map[string]any{"role": inv.Role, "email": user.Email},
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &AcceptResult{OrgID: inv.OrgID, Role: inv.Role}, nil
}

// ---- Helpers ----

func generateInviteToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashInviteToken(raw), nil
}

func hashInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
