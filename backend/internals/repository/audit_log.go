package repository

import (
	"context"
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID           string
	OrgID        string
	UserID       *string
	Action       string
	ResourceType string
	ResourceID   *string
	Diff         json.RawMessage
	CreatedAt    time.Time
	ActorEmail   *string // JOIN iz users, null za system
}

type AuditLogRepo struct{}

func NewAuditLogRepo() *AuditLogRepo { return &AuditLogRepo{} }

// Insert upisuje audit zapis. Poziva se unutar tx.
func (r *AuditLogRepo) Insert(
	ctx context.Context,
	q DBTX,
	orgID string,
	userID *string,
	action, resourceType string,
	resourceID *string,
	diff json.RawMessage,
) error {
	_, err := q.Exec(ctx, `
		INSERT INTO audit_logs (org_id, user_id, action, resource_type, resource_id, diff)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, orgID, userID, action, resourceType, resourceID, diff)
	return err
}

type ListAuditParams struct {
	OrgID        string
	ResourceType string // "" = sve
	ResourceID   string // "" = sve
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}

func (r *AuditLogRepo) List(ctx context.Context, q DBTX, p ListAuditParams) ([]*AuditLog, int, error) {
	rows, err := q.Query(ctx, `
		SELECT a.id, a.org_id, a.user_id, a.action, a.resource_type, a.resource_id,
		       a.diff, a.created_at, u.email,
		       COUNT(*) OVER() AS total
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		WHERE a.org_id = $1
		  AND ($2::text = '' OR a.resource_type = $2)
		  AND ($3::text = '' OR a.resource_id::text = $3)
		  AND ($4::timestamptz IS NULL OR a.created_at >= $4)
		  AND ($5::timestamptz IS NULL OR a.created_at <= $5)
		ORDER BY a.created_at DESC
		LIMIT $6 OFFSET $7
	`, p.OrgID, p.ResourceType, p.ResourceID, p.From, p.To, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*AuditLog
	var total int
	for rows.Next() {
		a := &AuditLog{}
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.UserID, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.Diff, &a.CreatedAt, &a.ActorEmail, &total,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
