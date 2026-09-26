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

	"pennant/backend/internals/audit"
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
	audit    *audit.Service
}

func NewService(pool *pgxpool.Pool, auditSvc *audit.Service) *Service {
	return &Service{
		pool:     pool,
		flags:    repository.NewFlagRepo(),
		flagEnvs: repository.NewFlagEnvRepo(),
		audit:    auditSvc,
	}
}

type CreateInput struct {
	OrgID       string
	ActorID     string
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

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionFlagCreate,
		ResourceType: audit.ResourceTypeFlag,
		ResourceID:   flag.ID,
		Before:       nil,
		After: map[string]any{
			"key":         flag.Key,
			"name":        flag.Name,
			"description": flag.Description,
			"type":        flag.FlagType,
			"archived":    flag.Archived,
		},
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
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
	ActorID     string
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

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	flag, err := s.flags.UpdateMetadata(ctx, tx, in.OrgID, in.FlagID, in.Name, in.Description)
	if err != nil {
		return nil, err
	}
	if flag == nil {
		return nil, ErrNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionFlagUpdate,
		ResourceType: audit.ResourceTypeFlag,
		ResourceID:   flag.ID,
		Before: map[string]any{
			"name":        existing.Name,
			"description": existing.Description,
		},
		After: map[string]any{
			"name":        flag.Name,
			"description": flag.Description,
		},
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	envs, err := s.flagEnvs.ListByFlag(ctx, tx, flag.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &FlagWithEnvs{Flag: flag, Environments: envs}, nil
}

type ArchiveInput struct {
	OrgID   string
	ActorID string
	FlagID  string
}

func (s *Service) Archive(ctx context.Context, in ArchiveInput) error {
	existing, err := s.flags.FindByID(ctx, s.pool, in.OrgID, in.FlagID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ok, err := s.flags.Archive(ctx, tx, in.OrgID, in.FlagID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionFlagArchive,
		ResourceType: audit.ResourceTypeFlag,
		ResourceID:   in.FlagID,
		Before:       map[string]any{"archived": false},
		After:        map[string]any{"archived": true},
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	return tx.Commit(ctx)
}

type UpdateEnvInput struct {
	OrgID          string
	ActorID        string
	FlagID         string
	EnvID          string
	Enabled        *bool
	RolloutPercent *int
	Value          json.RawMessage
	ValueSet       bool
	ExpiresAt      *string
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

	before, err := s.flagEnvs.FindByFlagAndEnv(ctx, s.pool, in.FlagID, in.EnvID)
	if err != nil {
		return nil, err
	}
	if before == nil {
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

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	after, err := s.flagEnvs.Update(ctx, tx, in.FlagID, in.EnvID, repository.UpdateFlagEnvParams{
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
	if after == nil {
		return nil, ErrEnvNotFound
	}

	if err := s.audit.Log(ctx, tx, audit.Entry{
		OrgID:        in.OrgID,
		ActorID:      in.ActorID,
		Action:       audit.ActionFlagEnvUpdate,
		ResourceType: audit.ResourceTypeFlagEnv,
		ResourceID:   after.ID,
		Before: map[string]any{
			"enabled":         before.Enabled,
			"rollout_percent": before.RolloutPercent,
			"value":           rawOrNil(before.Value),
			"expires_at":      timeOrNil(before.ExpiresAt),
		},
		After: map[string]any{
			"enabled":         after.Enabled,
			"rollout_percent": after.RolloutPercent,
			"value":           rawOrNil(after.Value),
			"expires_at":      timeOrNil(after.ExpiresAt),
		},
	}); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

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

// rawOrNil pretvara json.RawMessage u any za audit diff.
// Prazno postaje nil (JSON null).
func rawOrNil(r json.RawMessage) any {
	if len(r) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(r, &v); err != nil {
		return string(r)
	}
	return v
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
