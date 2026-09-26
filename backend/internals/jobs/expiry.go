package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pennant/backend/internals/audit"
	"pennant/backend/internals/evaluation"
)

type ExpiryJob struct {
	pool      *pgxpool.Pool
	audit     *audit.Service
	publisher *evaluation.Publisher
}

func NewExpiryJob(pool *pgxpool.Pool, auditSvc *audit.Service, publisher *evaluation.Publisher) *ExpiryJob {
	return &ExpiryJob{
		pool:      pool,
		audit:     auditSvc,
		publisher: publisher,
	}
}

type expiredRow struct {
	FlagEnvID string
	FlagID    string
	EnvID     string
	OrgID     string
}

// Run pronalazi sve flag_environments čiji je expires_at u prošlosti,
// a enabled je i dalje true, i gasi ih. Sve izmene + audit idu u jednoj
// transakciji da bi bili atomični.
func (j *ExpiryJob) Run(ctx context.Context) error {
	rows, err := j.pool.Query(ctx, `
		SELECT fe.id, fe.flag_id, fe.env_id, f.org_id
		FROM flag_environments fe
		JOIN flags f ON f.id = fe.flag_id
		WHERE fe.expires_at IS NOT NULL
		  AND fe.expires_at <= now()
		  AND fe.enabled = true
	`)
	if err != nil {
		return fmt.Errorf("query expired: %w", err)
	}
	defer rows.Close()

	var expired []expiredRow
	for rows.Next() {
		var r expiredRow
		if err := rows.Scan(&r.FlagEnvID, &r.FlagID, &r.EnvID, &r.OrgID); err != nil {
			return fmt.Errorf("scan expired: %w", err)
		}
		expired = append(expired, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(expired) == 0 {
		return nil
	}

	slog.Info("expiry job: found expired flag environments", "count", len(expired))

	tx, err := j.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	orgsToInvalidate := make(map[string]struct{})

	for _, e := range expired {
		// Gasi flag; WHERE uslov ponavlja proveru da izbegnemo race
		// ako je korisnik u međuvremenu promenio stanje.
		tag, err := tx.Exec(ctx, `
			UPDATE flag_environments
			SET enabled = false, updated_at = now()
			WHERE id = $1
			  AND expires_at IS NOT NULL
			  AND expires_at <= now()
			  AND enabled = true
		`, e.FlagEnvID)
		if err != nil {
			return fmt.Errorf("disable %s: %w", e.FlagEnvID, err)
		}
		if tag.RowsAffected() == 0 {
			// Neko je u međuvremenu promenio — preskoči audit.
			continue
		}

		// Audit — ActorID "" znači system (user_id NULL u bazi).
		if err := j.audit.Log(ctx, tx, audit.Entry{
			OrgID:        e.OrgID,
			ActorID:      "", // system
			Action:       audit.ActionSystemFlagEnvExpire,
			ResourceType: audit.ResourceTypeFlagEnv,
			ResourceID:   e.FlagEnvID,
			Before:       map[string]any{"enabled": true},
			After:        map[string]any{"enabled": false, "reason": "expires_at"},
		}); err != nil {
			return fmt.Errorf("audit %s: %w", e.FlagEnvID, err)
		}

		orgsToInvalidate[e.OrgID] = struct{}{}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Invalidacija cache-a — best-effort, po organizaciji.
	for orgID := range orgsToInvalidate {
		if err := j.publisher.PublishFlagUpdate(ctx, orgID); err != nil {
			slog.Warn("publish flag update failed", "err", err, "org_id", orgID)
		}
	}

	slog.Info("expiry job: disabled expired flags", "count", len(expired))
	return nil
}
