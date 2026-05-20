//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/antonovs105/project-management-system-go/internal/webfinger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebFingerResolvesLocalUserAndProjectActors(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	webFingerService := webfinger.NewService(webfinger.NewRepository(db), cfg)

	owner, err := userService.RegisterUser(ctx, "wf-owner", "wf-owner@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "WebFinger Board", "", owner.ID)
	require.NoError(t, err)

	userJRD, err := webFingerService.Resolve(ctx, "acct:"+owner.Username+"@localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, "acct:"+owner.Username+"@localhost:8080", userJRD.Subject)
	assert.Equal(t, []string{owner.APID}, userJRD.Aliases)
	require.Len(t, userJRD.Links, 1)
	assert.Equal(t, owner.APID, userJRD.Links[0].Href)

	projectResource := "acct:project-" + createdProject.ID + "@localhost:8080"
	projectJRD, err := webFingerService.Resolve(ctx, projectResource)
	require.NoError(t, err)
	assert.Equal(t, projectResource, projectJRD.Subject)
	assert.Equal(t, []string{createdProject.APID}, projectJRD.Aliases)
	require.Len(t, projectJRD.Links, 1)
	assert.Equal(t, createdProject.APID, projectJRD.Links[0].Href)
}
