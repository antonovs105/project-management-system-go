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
	require.NotEmpty(t, doc.Components["securitySchemes"]["cookieAuth"])
	require.NotEmpty(t, doc.Components["securitySchemes"]["metricsBearerAuth"])

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "get", path: "/health"},
		{method: "get", path: "/ready"},
		{method: "get", path: "/metrics"},
		{method: "post", path: "/webhooks/github"},
		{method: "get", path: "/instance"},
		{method: "post", path: "/register"},
		{method: "post", path: "/login"},
		{method: "get", path: "/auth/oauth/providers"},
		{method: "get", path: "/auth/{provider}/start"},
		{method: "get", path: "/auth/{provider}/callback"},
		{method: "post", path: "/auth/oauth/exchange"},
		{method: "post", path: "/auth/password/forgot"},
		{method: "post", path: "/auth/password/reset"},
		{method: "post", path: "/auth/email/verify"},
		{method: "get", path: "/api/v1/instance"},
		{method: "get", path: "/api/v1/me"},
		{method: "patch", path: "/api/v1/me/password"},
		{method: "post", path: "/api/v1/me/logout"},
		{method: "post", path: "/api/v1/me/email/verification"},
		{method: "get", path: "/api/v1/me/sessions"},
		{method: "delete", path: "/api/v1/me/sessions/{sessionID}"},
		{method: "get", path: "/api/v1/me/security-events"},
		{method: "get", path: "/api/v1/me/invites"},
		{method: "get", path: "/api/v1/me/federation/inbox"},
		{method: "post", path: "/api/v1/me/federation/discover"},
		{method: "get", path: "/api/v1/me/federation/follows"},
		{method: "post", path: "/api/v1/me/federation/follows"},
		{method: "get", path: "/api/v1/me/remote-project-invites"},
		{method: "post", path: "/api/v1/me/remote-project-invites/{inviteID}/accept"},
		{method: "post", path: "/api/v1/me/remote-project-invites/{inviteID}/reject"},
		{method: "get", path: "/api/v1/admin/users"},
		{method: "patch", path: "/api/v1/admin/users/{userID}/role"},
		{method: "get", path: "/api/v1/admin/audit-events"},
		{method: "get", path: "/api/v1/projects"},
		{method: "post", path: "/api/v1/projects"},
		{method: "get", path: "/api/v1/projects/{projectID}"},
		{method: "patch", path: "/api/v1/projects/{projectID}"},
		{method: "delete", path: "/api/v1/projects/{projectID}"},
		{method: "get", path: "/api/v1/projects/{projectID}/roles"},
		{method: "post", path: "/api/v1/projects/{projectID}/roles"},
		{method: "patch", path: "/api/v1/projects/{projectID}/roles/{roleID}"},
		{method: "delete", path: "/api/v1/projects/{projectID}/roles/{roleID}"},
		{method: "get", path: "/api/v1/projects/{projectID}/members"},
		{method: "post", path: "/api/v1/projects/{projectID}/members"},
		{method: "patch", path: "/api/v1/projects/{projectID}/members/{userID}"},
		{method: "delete", path: "/api/v1/projects/{projectID}/members/{userID}"},
		{method: "get", path: "/api/v1/projects/{projectID}/invites"},
		{method: "post", path: "/api/v1/invites/{inviteID}/accept"},
		{method: "post", path: "/api/v1/invites/{inviteID}/reject"},
		{method: "post", path: "/api/v1/invites/{inviteID}/revoke"},
		{method: "get", path: "/api/v1/projects/{projectID}/github/repositories"},
		{method: "post", path: "/api/v1/projects/{projectID}/github/repositories"},
		{method: "delete", path: "/api/v1/projects/{projectID}/github/repositories/{repositoryID}"},
		{method: "post", path: "/api/v1/projects/{projectID}/github/repositories/{repositoryID}/sync"},
		{method: "get", path: "/api/v1/projects/{projectID}/github/commits"},
		{method: "get", path: "/api/v1/projects/{projectID}/labels"},
		{method: "post", path: "/api/v1/projects/{projectID}/labels"},
		{method: "delete", path: "/api/v1/projects/{projectID}/labels/{labelID}"},
		{method: "get", path: "/api/v1/projects/{projectID}/tickets"},
		{method: "post", path: "/api/v1/projects/{projectID}/tickets"},
		{method: "get", path: "/api/v1/projects/{projectID}/tickets/events"},
		{method: "get", path: "/api/v1/projects/{projectID}/graph"},
		{method: "get", path: "/api/v1/tickets/{ticketID}"},
		{method: "patch", path: "/api/v1/tickets/{ticketID}"},
		{method: "delete", path: "/api/v1/tickets/{ticketID}"},
		{method: "post", path: "/api/v1/tickets/{ticketID}/links"},
		{method: "delete", path: "/api/v1/links/{linkID}"},
		{method: "get", path: "/api/v1/tickets/{ticketID}/github/commits"},
		{method: "post", path: "/api/v1/tickets/{ticketID}/github/commits"},
		{method: "delete", path: "/api/v1/tickets/{ticketID}/github/commits/{commitID}"},
		{method: "get", path: "/api/v1/tickets/{ticketID}/comments"},
		{method: "post", path: "/api/v1/tickets/{ticketID}/comments"},
		{method: "delete", path: "/api/v1/comments/{commentID}"},
		{method: "get", path: "/api/v1/projects/{projectID}/deliveries"},
		{method: "get", path: "/api/v1/projects/{projectID}/deliveries/summary"},
		{method: "post", path: "/api/v1/projects/{projectID}/deliveries/{deliveryID}/retry"},
		{method: "get", path: "/api/v1/admin/federation/domain-blocks"},
		{method: "post", path: "/api/v1/admin/federation/domain-blocks"},
		{method: "delete", path: "/api/v1/admin/federation/domain-blocks/{domain}"},
		{method: "get", path: "/api/v1/admin/federation/remote-actors"},
		{method: "get", path: "/api/v1/admin/federation/deliveries"},
		{method: "get", path: "/api/v1/admin/federation/deliveries/summary"},
		{method: "post", path: "/api/v1/admin/federation/deliveries/{deliveryID}/retry"},
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

	require.Equal(t, "#/components/schemas/ProjectDelivery", responseItemsRef(t, doc, "get", "/api/v1/projects/{projectID}/deliveries", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectDelivery", responseRef(t, doc, "post", "/api/v1/projects/{projectID}/deliveries/{deliveryID}/retry", "202", "application/json"))
	require.Equal(t, "#/components/schemas/FederationDelivery", responseItemsRef(t, doc, "get", "/api/v1/admin/federation/deliveries", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationDeliverySummary", responseRef(t, doc, "get", "/api/v1/admin/federation/deliveries/summary", "200", "application/json"))
	require.Equal(t, "#/components/schemas/Delivery", responseRef(t, doc, "post", "/api/v1/admin/federation/deliveries/{deliveryID}/retry", "202", "application/json"))
	require.Equal(t, "#/components/schemas/FederationInboxActivity", responseItemsRef(t, doc, "get", "/api/v1/me/federation/inbox", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationRemoteActor", responseRef(t, doc, "post", "/api/v1/me/federation/discover", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FederationRemoteFollow", responseItemsRef(t, doc, "get", "/api/v1/me/federation/follows", "200", "application/json"))
	require.Equal(t, "#/components/schemas/FollowRemoteActorResult", responseRef(t, doc, "post", "/api/v1/me/federation/follows", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteProjectInvite", responseItemsRef(t, doc, "get", "/api/v1/me/remote-project-invites", "200", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteProjectInviteResult", responseRef(t, doc, "post", "/api/v1/me/remote-project-invites/{inviteID}/accept", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteProjectInviteResult", responseRef(t, doc, "post", "/api/v1/me/remote-project-invites/{inviteID}/reject", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteProject", responseItemsRef(t, doc, "get", "/api/v1/remote-projects", "200", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteProject", responseRef(t, doc, "get", "/api/v1/remote-projects/{remoteProjectID}", "200", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicket", responseItemsRef(t, doc, "get", "/api/v1/remote-projects/{remoteProjectID}/tickets", "200", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicket", responseRef(t, doc, "get", "/api/v1/remote-projects/{remoteProjectID}/tickets/{remoteTicketID}", "200", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicketWriteResult", responseRef(t, doc, "post", "/api/v1/remote-projects/{remoteProjectID}/tickets", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicketWriteResult", responseRef(t, doc, "patch", "/api/v1/remote-projects/{remoteProjectID}/tickets/{remoteTicketID}", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicketWriteResult", responseRef(t, doc, "post", "/api/v1/remote-projects/{remoteProjectID}/tickets/{remoteTicketID}/move", "202", "application/json"))
	require.Equal(t, "#/components/schemas/RemoteTicketWriteResult", responseRef(t, doc, "delete", "/api/v1/remote-projects/{remoteProjectID}/tickets/{remoteTicketID}", "202", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectInviteInspection", responseItemsRef(t, doc, "get", "/api/v1/me/invites", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectMember", responseItemsRef(t, doc, "get", "/api/v1/projects/{projectID}/members", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectMember", responseRef(t, doc, "patch", "/api/v1/projects/{projectID}/members/{userID}", "200", "application/json"))
	require.Equal(t, "#/components/schemas/ProjectInviteInspection", responseItemsRef(t, doc, "get", "/api/v1/projects/{projectID}/invites", "200", "application/json"))

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

func TestOpenAPIContractDocumentsGraphMetadata(t *testing.T) {
	doc := loadOpenAPI(t)

	require.Equal(t, "#/components/schemas/GraphResponse", responseRef(t, doc, "get", "/api/v1/projects/{projectID}/graph", "200", "application/json"))
	graphProps := schemaProperties(t, doc, "GraphResponse")
	require.Contains(t, graphProps, "nodes")
	require.Contains(t, graphProps, "links")
	require.Contains(t, graphProps, "limit")
	require.Contains(t, graphProps, "truncated")
}

func TestOpenAPIContractRejectsUnversionedRESTAliases(t *testing.T) {
	doc := loadOpenAPI(t)

	for path := range doc.Paths {
		if strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/v1/") {
			t.Fatalf("legacy unversioned REST path documented: %s", path)
		}
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

func requireMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	require.True(t, ok, "expected map, got %T", value)
	return m
}
