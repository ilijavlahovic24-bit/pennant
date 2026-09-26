package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/repository"
)

// Action konstante — koristimo ih da izbegnemo typo greške.
const (
	ActionFlagCreate          = "flag.create"
	ActionFlagUpdate          = "flag.update"
	ActionFlagArchive         = "flag.archive"
	ActionFlagEnvUpdate       = "flag_env.update"
	ActionTargetingAdd        = "targeting_rule.add"
	ActionTargetingReplace    = "targeting_rule.replace"
	ActionTargetingDelete     = "targeting_rule.delete"
	ActionSystemFlagEnvExpire = "system.flag_env.expire"
)

// ResourceType konstante.
const (
	ResourceTypeFlag          = "flag"
	ResourceTypeFlagEnv       = "flag_environment"
	ResourceTypeTargetingRule = "targeting_rule"
	ResourceTypeMembership    = "membership"
)

type Entry struct {
	OrgID        string
	ActorID      string // "" = system
	Action       string
	ResourceType string
	ResourceID   string
	Before       any
	After        any
}

type Service struct {
	pool *pgxpool.Pool
	repo *repository.AuditLogRepo
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, repo: repository.NewAuditLogRepo()}
}

// Log upisuje zapis. Poziva se UNUTAR postojeće transakcije da bi
// promena i audit bili atomični.
func (s *Service) Log(ctx context.Context, q repository.DBTX, e Entry) error {
	diff := map[string]any{
		"before": e.Before,
		"after":  e.After,
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("marshal audit diff: %w", err)
	}

	var userID *string
	if e.ActorID != "" {
		userID = &e.ActorID
	}
	var resourceID *string
	if e.ResourceID != "" {
		resourceID = &e.ResourceID
	}

	return s.repo.Insert(ctx, q, e.OrgID, userID, e.Action, e.ResourceType, resourceID, raw)
}

type ListParams struct {
	OrgID        string
	ResourceType string
	ResourceID   string
	From         *time.Time
	To           *time.Time
	Page         int
	PageSize     int
}

type ListResult struct {
	Items    []*repository.AuditLog `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

func (s *Service) List(ctx context.Context, p ListParams) (*ListResult, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 50
	}
	offset := (p.Page - 1) * p.PageSize

	items, total, err := s.repo.List(ctx, s.pool, repository.ListAuditParams{
		OrgID:        p.OrgID,
		ResourceType: p.ResourceType,
		ResourceID:   p.ResourceID,
		From:         p.From,
		To:           p.To,
		Limit:        p.PageSize,
		Offset:       offset,
	})
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Total: total, Page: p.Page, PageSize: p.PageSize}, nil
}
