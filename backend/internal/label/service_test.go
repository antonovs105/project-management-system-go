package label

import (
	"context"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/stretchr/testify/require"
)

type repositoryStub struct {
	created *Label
}

func (r *repositoryStub) List(context.Context, string) ([]Label, error) { return []Label{}, nil }
func (r *repositoryStub) Create(_ context.Context, item *Label) error {
	item.ID = "33333333-3333-4333-8333-333333333333"
	r.created = item
	return nil
}
func (r *repositoryStub) Delete(context.Context, string, string) (bool, error) { return true, nil }

type projectCheckerStub struct{ allowed bool }

func (p projectCheckerStub) GetProjectByID(context.Context, string, string) (*project.Project, error) {
	return &project.Project{}, nil
}
func (p projectCheckerStub) HasProjectPermission(context.Context, string, string, string) (bool, error) {
	return p.allowed, nil
}

func TestServiceCreateNormalizesLabel(t *testing.T) {
	repo := new(repositoryStub)
	service := NewService(repo, projectCheckerStub{allowed: true})

	item, err := service.Create(context.Background(), "project", "user", " Bug ", " #ff0000 ")

	require.NoError(t, err)
	require.Equal(t, "Bug", item.Name)
	require.Equal(t, "#FF0000", item.Color)
	require.Same(t, item, repo.created)
}

func TestServiceCreateRequiresProjectUpdate(t *testing.T) {
	service := NewService(new(repositoryStub), projectCheckerStub{allowed: false})

	_, err := service.Create(context.Background(), "project", "user", "Bug", "#FF0000")

	require.ErrorIs(t, err, apperror.ErrForbidden)
}

func TestServiceCreateRejectsInvalidColor(t *testing.T) {
	service := NewService(new(repositoryStub), projectCheckerStub{allowed: true})

	_, err := service.Create(context.Background(), "project", "user", "Bug", "red")

	require.ErrorIs(t, err, ErrInvalidInput)
}
