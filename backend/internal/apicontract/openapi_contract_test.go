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
	doc := loadOpenAPI(t)
	require.Equal(t, "3.1.0", doc.OpenAPI)
	require.NotEmpty(t, doc.Components["securitySchemes"]["bearerAuth"])
	require.NotEmpty(t, doc.Components["securitySchemes"]["metricsBearerAuth"])

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "get", path: "/health"},
		{method: "get", path: "/ready"},
		{method: "get", path: "/metrics"},
		{method: "post", path: "/register"},
		{method: "post", path: "/login"},
		{method: "get", path: "/auth/oauth/providers"},
		{method: "get", path: "/auth/{provider}/start"},
		{method: "get", path: "/auth/{provider}/callback"},
		{method: "post", path: "/auth/oauth/exchange"},
		{method: "get", path: "/api/me"},
		{method: "patch", path: "/api/me/password"},
		{method: "get", path: "/api/me/invites"},
		{method: "get", path: "/api/me/federation/inbox"},
		{method: "post", path: "/api/me/federation/discover"},
		{method: "get", path: "/api/me/federation/follows"},
		{method: "post", path: "/api/me/federation/follows"},
		{method: "get", path: "/api/admin/users"},
		{method: "patch", path: "/api/admin/users/{userID}/role"},
		{method: "get", path: "/api/admin/audit-events"},
		{method: "get", path: "/api/projects"},
		{method: "post", path: "/api/projects"},
		{method: "get", path: "/api/projects/{projectID}"},
		{method: "patch", path: "/api/projects/{projectID}"},
		{method: "delete", path: "/api/projects/{projectID}"},
		{method: "get", path: "/api/projects/{projectID}/roles"},
		{method: "post", path: "/api/projects/{projectID}/roles"},
		{method: "patch", path: "/api/projects/{projectID}/roles/{roleID}"},
		{method: "delete", path: "/api/projects/{projectID}/roles/{roleID}"},
		{method: "get", path: "/api/projects/{projectID}/members"},
		{method: "post", path: "/api/projects/{projectID}/members"},
		{method: "patch", path: "/api/projects/{projectID}/members/{userID}"},
		{method: "delete", path: "/api/projects/{projectID}/members/{userID}"},
		{method: "get", path: "/api/projects/{projectID}/invites"},
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
		{method: "get", path: "/api/admin/federation/deliveries/summary"},
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

func TestOpenAPIContractDocumentsMetricsBearerAuth(t *testing.T) {
	doc := loadOpenAPI(t)
	operation := operation(t, doc, "get", "/metrics")
	security, ok := operation["security"].([]any)
	require.True(t, ok, "missing /metrics security declaration")
	raw, err := yaml.Marshal(security)
	require.NoError(t, err)
	require.Contains(t, string(raw), "metricsBearerAuth")
	require.NotContains(t, string(raw), "bearerAuth")
}

func TestOpenAPIContractDocumentsDeliveryResponseShapes(t *testing.T) {
	doc := loadOpenAPI(t)

	require.Equal(t, "#/components/schemas/ProjectDelivery", responseItemsRef(t, doc, "get", "/api/projects/{projectID}/deliveries", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectDelivery", responseRef(t, doc, "post", "/api/projects/{projectID}/deliveries/{deliveryID}/retry", "202", "application/json"))
	require.Equal(t, "#/components/schemas/FederationDelivery", responseItemsRef(t, doc, "get", "/api/admin/federation/deliveries", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationDeliverySummary", responseRef(t, doc, "get", "/api/admin/federation/deliveries/summary", "200", "application/json"))
	require.Equal(t, "#/components/schemas/Delivery", responseRef(t, doc, "post", "/api/admin/federation/deliveries/{deliveryID}/retry", "202", "application/json"))
	require.Equal(t, "#/components/schemas/FederationInboxActivity", responseItemsRef(t, doc, "get", "/api/me/federation/inbox", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationRemoteActor", responseRef(t, doc, "post", "/api/me/federation/discover", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationRemoteFollow", responseItemsRef(t, doc, "get", "/api/me/federation/follows", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FollowRemoteActorResult", responseRef(t, doc, "post", "/api/me/federation/follows", "202", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectInviteInspection", responseItemsRef(t, doc, "get", "/api/me/invites", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectMember", responseItemsRef(t, doc, "get", "/api/projects/{projectID}/members", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectMember", responseRef(t, doc, "patch", "/api/projects/{projectID}/members/{userID}", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectInviteInspection", responseItemsRef(t, doc, "get", "/api/projects/{projectID}/invites", "200", "application/json"))

	rawDeliveryProps := schemaProperties(t, doc, "Delivery")
	require.Contains(t, rawDeliveryProps, "activity_id")
	require.Contains(t, rawDeliveryProps, "actor_id")
	require.Contains(t, rawDeliveryProps, "actor_ap_id")
	require.NotContains(t, rawDeliveryProps, "activity_type")
	require.NotContains(t, rawDeliveryProps, "can_retry")

	projectDeliveryProps := schemaProperties(t, doc, "ProjectDelivery")
	require.Contains(t, projectDeliveryProps, "activity_type")
	require.Contains(t, projectDeliveryProps, "can_retry")
	require.NotContains(t, projectDeliveryProps, "activity_id")
	require.NotContains(t, projectDeliveryProps, "actor_id")
}

func TestOpenAPIContractDocumentsVersionedRESTAliases(t *testing.T) {
	doc := loadOpenAPI(t)

	for _, path := range []string{
		"/api/me",
		"/api/me/password",
		"/api/me/invites",
		"/api/me/federation/inbox",
		"/api/me/federation/discover",
		"/api/me/federation/follows",
		"/api/admin/users",
		"/api/admin/users/{userID}/role",
		"/api/admin/audit-events",
		"/api/projects",
		"/api/projects/{projectID}",
		"/api/projects/{projectID}/roles",
		"/api/projects/{projectID}/roles/{roleID}",
		"/api/projects/{projectID}/members",
		"/api/projects/{projectID}/members/{userID}",
		"/api/projects/{projectID}/invites",
		"/api/invites/{inviteID}/accept",
		"/api/invites/{inviteID}/reject",
		"/api/invites/{inviteID}/revoke",
		"/api/projects/{projectID}/tickets",
		"/api/projects/{projectID}/graph",
		"/api/tickets/{ticketID}",
		"/api/tickets/{ticketID}/links",
		"/api/links/{linkID}",
		"/api/tickets/{ticketID}/comments",
		"/api/comments/{commentID}",
		"/api/projects/{projectID}/deliveries",
		"/api/projects/{projectID}/deliveries/summary",
		"/api/projects/{projectID}/deliveries/{deliveryID}/retry",
		"/api/admin/federation/domain-blocks",
		"/api/admin/federation/domain-blocks/{domain}",
		"/api/admin/federation/remote-actors",
		"/api/admin/federation/deliveries",
		"/api/admin/federation/deliveries/summary",
		"/api/admin/federation/deliveries/{deliveryID}/retry",
	} {
		versionedPath := "/api/v1" + strings.TrimPrefix(path, "/api")
		pathItem, ok := doc.Paths[versionedPath]
		require.True(t, ok, "missing OpenAPI path %s", versionedPath)
		require.Equal(t, "#/paths/"+openAPIPathRef(path), pathItem["$ref"])
	}
}

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	require.NoError(t, err)

	var doc openAPIDocument
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

func responseRef(t *testing.T, doc openAPIDocument, method string, path string, status string, mediaType string) string {
	t.Helper()

	operation := operation(t, doc, method, path)
	responses := requireMap(t, operation["responses"])
	response := requireMap(t, responses[status])
	content := requireMap(t, response["content"])
	media := requireMap(t, content[mediaType])
	schema := requireMap(t, media["schema"])
	ref, ok := schema["$ref"].(string)
	require.True(t, ok, "missing schema ref for %s %s %s", method, path, status)
	return ref
}

func responseItemsRef(t *testing.T, doc openAPIDocument, method string, path string, status string, mediaType string) string {
	t.Helper()

	operation := operation(t, doc, method, path)
	responses := requireMap(t, operation["responses"])
	response := requireMap(t, responses[status])
	content := requireMap(t, response["content"])
	media := requireMap(t, content[mediaType])
	schema := requireMap(t, media["schema"])
	items := requireMap(t, schema["items"])
	ref, ok := items["$ref"].(string)
	require.True(t, ok, "missing items schema ref for %s %s %s", method, path, status)
	return ref
}

func operation(t *testing.T, doc openAPIDocument, method string, path string) map[string]any {
	t.Helper()

	operations, ok := doc.Paths[path]
	require.True(t, ok, "missing OpenAPI path %s", path)
	return requireMap(t, operations[strings.ToLower(method)])
}

func schemaProperties(t *testing.T, doc openAPIDocument, name string) map[string]any {
	t.Helper()

	schemas := doc.Components["schemas"]
	schema := requireMap(t, schemas[name])
	return requireMap(t, schema["properties"])
}

func openAPIPathRef(path string) string {
	return strings.ReplaceAll(path, "/", "~1")
}

func requireMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	require.True(t, ok, "expected map, got %T", value)
	return m
}
