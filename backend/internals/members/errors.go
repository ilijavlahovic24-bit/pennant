package members

import "errors"

var (
	ErrNotFound          = errors.New("membership not found")
	ErrInviteNotFound    = errors.New("invitation not found")
	ErrInviteExpired     = errors.New("invitation expired")
	ErrInviteAlreadyUsed = errors.New("invitation already used")
	ErrEmailMismatch     = errors.New("invitation is for a different email address")
	ErrInvalidRole       = errors.New("invalid role")
	ErrCannotModifySelf  = errors.New("you cannot change your own role")
	ErrCannotRemoveSelf  = errors.New("you cannot remove yourself")
	ErrLastOwner         = errors.New("cannot remove or demote the last owner")
	ErrAlreadyMember     = errors.New("user is already a member of this organization")
	ErrInvalidEmail      = errors.New("invalid email address")
)
