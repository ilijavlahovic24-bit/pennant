package flags

import "errors"

var (
	ErrNotFound         = errors.New("flag not found")
	ErrKeyTaken         = errors.New("flag key already exists")
	ErrInvalidKey       = errors.New("invalid flag key")
	ErrInvalidType      = errors.New("invalid flag type")
	ErrInvalidValue     = errors.New("value does not match flag type")
	ErrInvalidRollout   = errors.New("rollout percent must be between 0 and 100")
	ErrEnvNotFound      = errors.New("environment not found")
	ErrArchivedNoUpdate = errors.New("archived flags cannot be modified")
)
