package targeting

import "errors"

var (
	ErrNotFound           = errors.New("flag environment not found")
	ErrInvalidAttribute   = errors.New("attribute must be non-empty and up to 100 chars")
	ErrInvalidOperator    = errors.New("invalid operator")
	ErrInvalidAction      = errors.New("invalid action")
	ErrInvalidValue       = errors.New("invalid value for operator")
	ErrInvalidActionValue = errors.New("invalid action_value for action")
	ErrTooManyRules       = errors.New("too many targeting rules (max 50)")
	ErrDuplicatePriority  = errors.New("duplicate priority")
)
