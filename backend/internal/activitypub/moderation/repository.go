package moderation

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	UserRole(ctx context.Context, userID string) (string, error)
	ListDomainBlocks(ctx context.Context) ([]DomainBlock, error)
	UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error)
	DeleteDomainBlock(ctx context.Context, domain string) error
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) UserRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `SELECT role FROM users WHERE id = $1`, userID)
	return role, err
}

func (r *PgRepository) ListDomainBlocks(ctx context.Context) ([]DomainBlock, error) {
	var blocks []DomainBlock
	if err := r.db.SelectContext(ctx, &blocks, `
		SELECT id::text, domain, reason, created_by::text, created_at, updated_at
		FROM federation_domain_blocks
		ORDER BY domain ASC
	`); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (r *PgRepository) UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error) {
	var block DomainBlock
	if err := r.db.GetContext(ctx, &block, `
		INSERT INTO federation_domain_blocks (domain, reason, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (domain) DO UPDATE SET
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			updated_at = now()
		RETURNING id::text, domain, reason, created_by::text, created_at, updated_at
	`, domain, reason, userID); err != nil {
		return nil, err
	}
	return &block, nil
}

func (r *PgRepository) DeleteDomainBlock(ctx context.Context, domain string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM federation_domain_blocks WHERE domain = $1`, domain)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
