package project

import (
	"context"
	"testing"

	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/stretchr/testify/require"
)

func TestHandlerProjectCreationPolicyAllowsEveryoneByDefault(t *testing.T) {
	handler := NewHandler(nil)

	err := handler.requireProjectCreationAllowed(context.Background(), "user-1")

	require.NoError(t, err)
}

func TestHandlerProjectCreationPolicyRequiresInstanceAdmin(t *testing.T) {
	ctx := context.Background()
	roles := fakeInstanceRoles{roles: map[string]string{
		"user-1":  user.InstanceRoleUser,
		"admin-1": user.InstanceRoleAdmin,
		"owner-1": user.InstanceRoleOwner,
	}}
	handler := NewHandler(
		nil,
		WithProjectCreationPolicy(appconfig.ProjectCreationAdminsOnly),
		WithInstanceRoleProvider(roles),
	)

	require.ErrorContains(t, handler.requireProjectCreationAllowed(ctx, "user-1"), "insufficient permissions")
	require.NoError(t, handler.requireProjectCreationAllowed(ctx, "admin-1"))
	require.NoError(t, handler.requireProjectCreationAllowed(ctx, "owner-1"))
}

func TestHandlerProjectCreationPolicyFailsClosedWithoutRoleProvider(t *testing.T) {
	handler := NewHandler(nil, WithProjectCreationPolicy(appconfig.ProjectCreationAdminsOnly))

	err := handler.requireProjectCreationAllowed(context.Background(), "user-1")

	require.ErrorContains(t, err, "instance role provider")
}

type fakeInstanceRoles struct {
	roles map[string]string
}

func (f fakeInstanceRoles) InstanceRole(ctx context.Context, userID string) (string, error) {
	return f.roles[userID], nil
}
