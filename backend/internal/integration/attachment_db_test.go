//go:build integration

package integration

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/attachment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// acceptingScanner proves the scanner is invoked before storage.
type acceptingScanner struct{ called bool }

// Scan consumes the content and accepts it.
func (s *acceptingScanner) Scan(_ context.Context, source io.Reader) error {
	s.called = true
	_, err := io.Copy(io.Discard, source)
	return err
}

func TestAttachmentLifecycleAgainstPostgreSQL(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests")
	}
	ctx := context.Background()
	db, err := sqlx.Connect("postgres", source)
	require.NoError(t, err)
	defer db.Close()
	config := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	suffix := uuid.NewString()[:8]
	owner, err := user.NewService(user.NewRepository(db, config), []byte("integration-secret"), config).RegisterUser(ctx, "attachment_"+suffix, "attachment_"+suffix+"@example.test", "password123")
	require.NoError(t, err)
	projectRepository := project.NewRepository(db, config)
	projectService := project.NewService(projectRepository, config)
	createdProject, err := projectService.CreateProject(ctx, "Attachment "+suffix, "storage test", owner.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM actors WHERE id = $1`, createdProject.ID)
		_, _ = db.Exec(`DELETE FROM actors WHERE id = $1`, owner.ID)
	})
	ticketService := ticket.NewService(ticket.NewRepository(db, config), projectService, config)
	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{Title: "Evidence", Priority: "medium", Type: "task"}, createdProject.ID, owner.ID)
	require.NoError(t, err)
	store, err := attachment.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	scanner := &acceptingScanner{}
	service := attachment.NewService(attachment.NewRepository(db), store, projectRepository, scanner)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 600)...)
	created, err := service.Upload(ctx, createdTicket.ID, owner.ID, `..\evidence.png`, bytes.NewReader(png))
	require.NoError(t, err)
	require.True(t, scanner.called)
	require.Equal(t, "evidence.png", created.Filename)
	require.Equal(t, "image/png", created.ContentType)
	values, err := service.List(ctx, createdTicket.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, values, 1)
	metadata, reader, err := service.Open(ctx, created.ID, owner.ID)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, created.SHA256, metadata.SHA256)
	require.Equal(t, png, content)
	require.NoError(t, service.Delete(ctx, created.ID, owner.ID))
	_, _, err = service.Open(ctx, created.ID, owner.ID)
	require.ErrorIs(t, err, attachment.ErrNotFound)

	orphaned, err := service.Upload(ctx, createdTicket.ID, owner.ID, "orphan.png", bytes.NewReader(png))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM ticket_attachments WHERE id = $1`, orphaned.ID)
	require.NoError(t, err)
	deleted, err := service.CleanupOrphans(ctx, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	_, err = store.Open(ctx, orphaned.ObjectKey)
	require.Error(t, err)
}
