//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"pennant/backend/internals/auth"
)

func TestAuth_RegisterAndLogin(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{
		Email:    "alice@example.com",
		Password: "secret12345",
		OrgName:  "Acme Inc",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.AccessToken == "" || reg.RefreshToken == "" {
		t.Fatalf("expected tokens, got %+v", reg)
	}
	if reg.Role != "owner" {
		t.Fatalf("expected owner role, got %q", reg.Role)
	}
	if reg.UserID == "" || reg.OrgID == "" {
		t.Fatalf("expected user_id and org_id")
	}

	// Default environments su kreirani
	var envCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM environments WHERE org_id = $1`, reg.OrgID).Scan(&envCount); err != nil {
		t.Fatalf("count envs: %v", err)
	}
	if envCount != 3 {
		t.Fatalf("expected 3 default envs, got %d", envCount)
	}

	login, err := svc.Login(ctx, "alice@example.com", "secret12345")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.UserID != reg.UserID {
		t.Fatalf("user id mismatch: %s vs %s", login.UserID, reg.UserID)
	}
	if login.OrgID != reg.OrgID {
		t.Fatalf("org id mismatch")
	}
}

func TestAuth_RegisterDuplicateEmail(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	in := auth.RegisterInput{
		Email:    "bob@example.com",
		Password: "secret12345",
		OrgName:  "Bob Co",
	}
	if _, err := svc.Register(ctx, in); err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err := svc.Register(ctx, in)
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestAuth_RegisterWeakPassword(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	_, err := svc.Register(ctx, auth.RegisterInput{
		Email:    "c@example.com",
		Password: "short",
		OrgName:  "C",
	})
	if !errors.Is(err, auth.ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

func TestAuth_RegisterInvalidEmail(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	_, err := svc.Register(ctx, auth.RegisterInput{
		Email:    "not-an-email",
		Password: "secret12345",
		OrgName:  "X",
	})
	if !errors.Is(err, auth.ErrInvalidEmail) {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	if _, err := svc.Register(ctx, auth.RegisterInput{
		Email: "d@example.com", Password: "secret12345", OrgName: "D",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := svc.Login(ctx, "d@example.com", "wrong-password")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuth_LoginUnknownEmail(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	_, err := svc.Login(ctx, "nobody@example.com", "whatever123")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuth_RefreshRotatesToken(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{
		Email: "e@example.com", Password: "secret12345", OrgName: "E",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	ref1, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if ref1.RefreshToken == reg.RefreshToken {
		t.Fatalf("refresh token should be rotated")
	}
	if ref1.AccessToken == "" {
		t.Fatalf("expected new access token")
	}
	if ref1.UserID != reg.UserID {
		t.Fatalf("user id mismatch after refresh")
	}
}

func TestAuth_RefreshReplayDetection(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{
		Email: "f@example.com", Password: "secret12345", OrgName: "F",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Prvi refresh — uspešan, rotira token.
	ref1, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// Replay sa starim tokenom → detektovan, revokuje sve.
	_, err = svc.Refresh(ctx, reg.RefreshToken)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked on replay, got %v", err)
	}

	// Novi token (ref1) je takođe revokovan zbog replay detekcije.
	_, err = svc.Refresh(ctx, ref1.RefreshToken)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked after replay (all revoked), got %v", err)
	}
}

func TestAuth_RefreshInvalidToken(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	_, err := svc.Refresh(ctx, "not-a-real-token")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAuth_LogoutRevokesToken(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{
		Email: "g@example.com", Password: "secret12345", OrgName: "G",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := svc.Logout(ctx, reg.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}

	_, err = svc.Refresh(ctx, reg.RefreshToken)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked after logout, got %v", err)
	}
}

func TestAuth_LogoutIdempotent(t *testing.T) {
	resetDB(t)

	svc := auth.NewService(cfg, pool)
	ctx := context.Background()

	// Logout sa praznim tokenom ne sme da pukne.
	if err := svc.Logout(ctx, ""); err != nil {
		t.Fatalf("logout empty: %v", err)
	}
}
