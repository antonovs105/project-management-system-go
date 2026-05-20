package moderation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	role         string
	roleErr      error
	blocks       []DomainBlock
	upsertDomain string
	upsertReason string
	upsertUserID string
	deleteDomain string
	deleteErr    error
}

func (r *fakeRepository) UserRole(ctx context.Context, userID string) (string, error) {
	if r.roleErr != nil {
		return "", r.roleErr
	}
	return r.role, nil
}

func (r *fakeRepository) ListDomainBlocks(ctx context.Context) ([]DomainBlock, error) {
	return r.blocks, nil
}

func (r *fakeRepository) UpsertDomainBlock(ctx context.Context, domain, reason, userID string) (*DomainBlock, error) {
	r.upsertDomain = domain
	r.upsertReason = reason
	r.upsertUserID = userID
	return &DomainBlock{
		ID:        "block-1",
		Domain:    domain,
		Reason:    reason,
		CreatedBy: &userID,
		CreatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (r *fakeRepository) DeleteDomainBlock(ctx context.Context, domain string) error {
	r.deleteDomain = domain
	return r.deleteErr
}

func TestServiceBlockDomainRequiresAdmin(t *testing.T) {
	repo := &fakeRepository{role: "worker"}
	service := NewService(repo)

	_, err := service.BlockDomain(context.Background(), "user-1", "remote.example", "")

	require.ErrorIs(t, err, ErrAdminRequired)
	assert.Empty(t, repo.upsertDomain)
}

func TestServiceBlockDomainNormalizesDomain(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	service := NewService(repo)

	block, err := service.BlockDomain(context.Background(), "admin-1", "HTTPS://Remote.Example/users/alice", " spam ")

	require.NoError(t, err)
	assert.Equal(t, "remote.example", block.Domain)
	assert.Equal(t, "remote.example", repo.upsertDomain)
	assert.Equal(t, "spam", repo.upsertReason)
	assert.Equal(t, "admin-1", repo.upsertUserID)
}

func TestServiceRejectsInvalidDomain(t *testing.T) {
	service := NewService(&fakeRepository{role: RoleAdmin})

	_, err := service.BlockDomain(context.Background(), "admin-1", "bad/domain", "")

	require.ErrorIs(t, err, ErrInvalidDomainBlock)
}

func TestServiceUnblockMapsMissingDomain(t *testing.T) {
	service := NewService(&fakeRepository{role: RoleAdmin, deleteErr: sql.ErrNoRows})

	err := service.UnblockDomain(context.Background(), "admin-1", "remote.example")

	require.ErrorIs(t, err, ErrDomainBlockNotFound)
}
