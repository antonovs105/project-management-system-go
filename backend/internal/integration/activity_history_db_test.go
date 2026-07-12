//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activityhistory"
	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestVersionedActivityArchiveLifecycleAgainstPostgreSQL(t *testing.T) {
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
	userService := user.NewService(user.NewRepository(db, config), []byte("integration-secret"), config)
	owner, err := userService.RegisterUser(ctx, "history_"+suffix, "history_"+suffix+"@example.test", "password123")
	require.NoError(t, err)

	projectRepository := project.NewRepository(db, config)
	projectService := project.NewService(projectRepository, config)
	createdProject, err := projectService.CreateProject(ctx, "History "+suffix, "activity test", owner.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM actors WHERE id = $1`, createdProject.ID)
		_, _ = db.Exec(`DELETE FROM actors WHERE id = $1`, owner.ID)
	})

	ticketRepository := ticket.NewRepository(db, config)
	ticketService := ticket.NewService(ticketRepository, projectService, config)
	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{Title: "Versioned", Priority: "medium", Type: "task"}, createdProject.ID, owner.ID)
	require.NoError(t, err)

	activityRepository := activityhistory.NewRepository(db)
	events, err := activityRepository.List(ctx, createdProject.ID, 50, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 2)
	require.NotNil(t, events[0].ActorID)

	storedTicket, err := ticketRepository.GetByID(ctx, createdTicket.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, storedTicket.Version)
	require.NoError(t, activityRepository.SetTicketArchived(ctx, storedTicket.ID, owner.ID, storedTicket.Version, true))
	require.ErrorIs(t, activityRepository.SetTicketArchived(ctx, storedTicket.ID, owner.ID, storedTicket.Version, false), activityhistory.ErrVersionConflict)
	archivedTickets, err := activityRepository.ListArchivedTickets(ctx, createdProject.ID)
	require.NoError(t, err)
	require.Len(t, archivedTickets, 1)
	require.NoError(t, activityRepository.SetTicketArchived(ctx, storedTicket.ID, owner.ID, archivedTickets[0].Version, false))

	storedProject, err := projectRepository.GetByID(ctx, createdProject.ID)
	require.NoError(t, err)
	require.NoError(t, activityRepository.SetProjectArchived(ctx, storedProject.ID, owner.ID, storedProject.Version, true))
	archivedProjects, err := activityRepository.ListArchivedProjects(ctx, owner.ID)
	require.NoError(t, err)
	require.NotEmpty(t, archivedProjects)
	require.NoError(t, activityRepository.SetProjectArchived(ctx, storedProject.ID, owner.ID, archivedProjects[0].Version, false))

	var archivedAt *string
	err = db.Get(&archivedAt, `SELECT archived_at::text FROM projects WHERE id = $1`, storedProject.ID)
	require.NoError(t, err)
	require.Nil(t, archivedAt)
}
