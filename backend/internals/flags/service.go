package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/repository"
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,99}$`)

var validTypes = map[string]struct{}{
	"boolean": {},
	"string":  {},
	"number":  {},
	"json":    {},
}

type Service struct {
	pool     *pgxpool.Pool
	flags    *repository.FlagRepo
	flagEnvs *repository.FlagEnvRepo
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:     pool,
		flags:    repository.NewFlagRepo(),
		flagEnvs: repository.NewFlagEnvRepo(),
	}
}

type CreateInput struct {
	OrgID       string
	Key         string
	Name        string
	Description string
	Type        string
}

type FlagWithEnvs struct {
	*repository.Flag
	Environments []*repository.FlagEnvironmentDetailed `json:"environments"`
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*FlagWithEnvs, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)

	if !keyRe.MatchString(in.Key) {
		return nil, ErrInvalidKey
	}
	if _, ok := validTypes[in.Type]; !ok {
		return nil, ErrInvalidType
	}
	if in.Name == "" {
		in.Name = in.Key
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := s.flags.FindByKey(ctx, tx, in.OrgID, in.Key)
	if err != nil {
		return nil, fmt.Errorf("find flag: %w", err)
	}
	if existing != nil {
		return nil, ErrKeyTaken
	}

	flag, err := s.flags.Create(ctx, tx, in.OrgID, in.Key, in.Name, in.Description, in.Type)
	if err != nil {
		return nil, fmt.Errorf("create flag: %w", err)
	}

	if err := s.flagEnvs.CreateForAllEnvironments(ctx, tx, flag.ID, in.OrgID); err != nil {
		return nil, fmt.Errorf("create env states: %w", err)
	}

	envs, err := s.flagEnvs.ListByFlag(ctx, tx, flag.ID)
	if err != nil {
		return nil, fmt.Errorf("load env states: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &FlagWithEnvs{Flag: flag, Environments: envs}, nil
}

type ListParams struct {
	OrgID           string
	IncludeArchived bool
	Search          string
	Page            int
	PageSize        int
}

type ListResult struct {
	Items    []*FlagWithEnvs `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

func (s *Service) List(ctx context.Context, p ListParams) (*ListResult, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	offset := (p.Page - 1) * p.PageSize

	list, total, err := s.flags.List(ctx, s.pool, repository.ListFlagsParams{
		OrgID:           p.OrgID,
		IncludeArchived: p.IncludeArchived,
		Search:          strings.TrimSpace(p.Search),
		Limit:           p.PageSize,
		Offset:          offset,
	})
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(list))
	for i, f := range list {
		ids[i] = f.ID
	}

	envsByFlag, err := s.flagEnvs.ListByFlagIDs(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}

	items := make([]*FlagWithEnvs, len(list))
	for i, f := range list {
		items[i] = &FlagWithEnvs{Flag: f, Environments: envsByFlag[f.ID]}
	}

	return &ListResult{
		Items:    items,
		Total:    total,
		Page:     p.Page,
		PageSize: p.PageSize,
	}, nil
}

func (s *Service) Get(ctx context.Context, orgID, flagID string) (*FlagWithEnvs, error) {
	flag, err := s.flags.FindByID(ctx, s.pool, orgID, flagID)
	if err != nil {
		return nil, err
	}
	if flag == nil {
		return nil, ErrNotFound
	}
	envs, err := s.flagEnvs.ListByFlag(ctx, s.pool, flag.ID)
	if err != nil {
		return nil, err
	}
	return &FlagWithEnvs{Flag: flag, Environments: envs}, nil
}

type UpdateInput struct {
	OrgID       string
	FlagID      string
	Name        string
	Description string
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (*FlagWithEnvs, error) {
	existing, err := s.flags.FindByID(ctx, s.pool, in.OrgID, in.FlagID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.Archived {
		return nil, ErrArchivedNoUpdate
	}

	flag, err := s.flags.UpdateMetadata(ctx, s.pool, in.OrgID, in.FlagID, in.Name, in.Description)
	if err != nil {
		return nil, err
	}
	if flag == nil {
		return nil, ErrNotFound
	}
	envs, err := s.flagEnvs.ListByFlag(ctx, s.pool, flag.ID)
	if err != nil {
		return nil, err
	}
	return &FlagWithEnvs{Flag: flag, Environments: envs}, nil
}

func (s *Service) Archive(ctx context.Context, orgID, flagID string) error {
	ok, err := s.flags.Archive(ctx, s.pool, orgID, flagID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

type UpdateEnvInput struct {
	OrgID          string
	FlagID         string
	EnvID          string
	Enabled        *bool
	RolloutPercent *int
	Value          json.RawMessage
	ValueSet       bool
	ExpiresAt      *string // RFC3339 ili nil
	ClearExpiresAt bool
}

func (s *Service) UpdateEnvState(ctx context.Context, in UpdateEnvInput) (*repository.FlagEnvironmentDetailed, error) {
	flag, err := s.flags.FindByID(ctx, s.pool, in.OrgID, in.FlagID)
	if err != nil {
		return nil, err
	}
	if flag == nil {
		return nil, ErrNotFound
	}
	if flag.Archived {
		return nil, ErrArchivedNoUpdate
	}

	if in.RolloutPercent != nil && (*in.RolloutPercent < 0 || *in.RolloutPercent > 100) {
		return nil, ErrInvalidRollout
	}

	if in.ValueSet && len(in.Value) > 0 {
		if err := validateValue(flag.FlagType, in.Value); err != nil {
			return nil, err
		}
	}

	existing, err := s.flagEnvs.FindByFlagAndEnv(ctx, s.pool, in.FlagID, in.EnvID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrEnvNotFound
	}

	var expiresAt *time.Time
	if in.ExpiresAt != nil && *in.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *in.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("invalid expires_at: %w", err)
		}
		expiresAt = &t
	}

	_, err = s.flagEnvs.Update(ctx, s.pool, in.FlagID, in.EnvID, repository.UpdateFlagEnvParams{
		Enabled:        in.Enabled,
		RolloutPercent: in.RolloutPercent,
		Value:          in.Value,
		ValueSet:       in.ValueSet,
		ExpiresAt:      expiresAt,
		ClearExpiresAt: in.ClearExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	// Učitaj sa env slug/name za response.
	all, err := s.flagEnvs.ListByFlag(ctx, s.pool, in.FlagID)
	if err != nil {
		return nil, err
	}
	for _, fe := range all {
		if fe.EnvID == in.EnvID {
			return fe, nil
		}
	}
	return nil, ErrEnvNotFound
}

// validateValue proverava da li JSON vrednost odgovara tipu flaga.
func validateValue(flagType string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	switch flagType {
	case "boolean":
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return ErrInvalidValue
		}
	case "string":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ErrInvalidValue
		}
	case "number":
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return ErrInvalidValue
		}
	case "json":
		var anyVal any
		if err := json.Unmarshal(raw, &anyVal); err != nil {
			return ErrInvalidValue
		}
	}
	return nil
}
