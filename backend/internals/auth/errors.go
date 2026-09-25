package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrNoMembership       = errors.New("user has no organization membership")
	ErrForbidden          = errors.New("forbidden")
	ErrUnauthorized       = errors.New("unauthorized")
)
