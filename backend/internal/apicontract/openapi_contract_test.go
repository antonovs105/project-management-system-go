package apicontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	OpenAPI    string                               `yaml:"openapi"`
	Paths      map[string]map[string]any            `yaml:"paths"`
	Components map[string]map[string]map[string]any `yaml:"components"`
}

func TestOpenAPIContractDocumentsRegisteredRoutes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	require.NoError(t, err)

	var doc openAPIDocument
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	require.Equal(t, "3.1.0", doc.OpenAPI)
	require.NotEmpty(t, doc.Components["securitySchemes"]["bearerAuth"])

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "get", path: "/health"},
		{method: "get", path: "/ready"},
		{method: "post", path: "/register"},
		{method: "post", path: "/login"},
		{method: "get", path: "/api/me"},
		{method: "get", path: "/api/projects"},
		{method: "post", path: "/api/projects"},
		{method: "get", path: "/api/projects/{projectID}"},
		{method: "patch", path: "/api/projects/{projectID}"},
		{method: "delete", path: "/api/projects/{projectID}"},
		{method: "post", path: "/api/projects/{projectID}/members"},
		{method: "delete", path: "/api/projects/{projectID}/members/{userID}"},
		{method: "post", path: "/api/invites/{inviteID}/accept"},
		{method: "post", path: "/api/invites/{inviteID}/reject"},
		{method: "post", path: "/api/invites/{inviteID}/revoke"},
		{method: "get", path: "/api/projects/{projectID}/tickets"},
		{method: "post", path: "/api/projects/{projectID}/tickets"},
		{method: "get", path: "/api/projects/{projectID}/graph"},
		{method: "get", path: "/api/tickets/{ticketID}"},
		{method: "patch", path: "/api/tickets/{ticketID}"},
		{method: "delete", path: "/api/tickets/{ticketID}"},
		{method: "post", path: "/api/tickets/{ticketID}/links"},
		{method: "delete", path: "/api/links/{linkID}"},
		{method: "get", path: "/api/tickets/{ticketID}/comments"},
		{method: "post", path: "/api/tickets/{ticketID}/comments"},
		{method: "delete", path: "/api/comments/{commentID}"},
		{method: "get", path: "/api/projects/{projectID}/deliveries"},
		{method: "get", path: "/api/projects/{projectID}/deliveries/summary"},
		{method: "post", path: "/api/projects/{projectID}/deliveries/{deliveryID}/retry"},
		{method: "get", path: "/api/admin/federation/domain-blocks"},
		{method: "post", path: "/api/admin/federation/domain-blocks"},
		{method: "delete", path: "/api/admin/federation/domain-blocks/{domain}"},
		{method: "get", path: "/api/admin/federation/remote-actors"},
		{method: "get", path: "/api/admin/federation/deliveries"},
		{method: "post", path: "/api/admin/federation/deliveries/{deliveryID}/retry"},
		{method: "get", path: "/.well-known/webfinger"},
		{method: "get", path: "/users/{username}"},
		{method: "get", path: "/users/{username}/inbox"},
		{method: "post", path: "/users/{username}/inbox"},
		{method: "get", path: "/users/{username}/outbox"},
		{method: "post", path: "/users/{username}/outbox"},
		{method: "get", path: "/users/{username}/followers"},
		{method: "get", path: "/projects/{projectID}"},
		{method: "get", path: "/projects/{projectID}/inbox"},
		{method: "post", path: "/projects/{projectID}/inbox"},
		{method: "get", path: "/projects/{projectID}/outbox"},
		{method: "get", path: "/projects/{projectID}/followers"},
		{method: "get", path: "/projects/{projectID}/tickets"},
		{method: "get", path: "/tickets/{ticketID}"},
		{method: "get", path: "/comments/{commentID}"},
		{method: "get", path: "/activities/{activityID}"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			operations, ok := doc.Paths[route.path]
			require.True(t, ok, "missing OpenAPI path %s", route.path)
			_, ok = operations[strings.ToLower(route.method)]
			require.True(t, ok, "missing OpenAPI operation %s %s", route.method, route.path)
		})
	}
}
