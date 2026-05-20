package remoteactor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryRepository struct {
	actor *Actor
	err   error
}

func (m *memoryRepository) UpsertRemoteActor(ctx context.Context, actor *Actor) error {
	if m.err != nil {
		return m.err
	}
	copy := *actor
	m.actor = &copy
	return nil
}

func TestDiscoverFetchesAndCachesRemoteActor(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/webfinger":
			assert.Equal(t, "application/jrd+json", r.Header.Get("Accept"))
			writeJSON(t, w, map[string]any{
				"subject": r.URL.Query().Get("resource"),
				"links": []map[string]any{
					{
						"rel":  "self",
						"type": "application/activity+json",
						"href": server.URL + "/users/alice",
					},
				},
			})
		case "/users/alice":
			writeJSON(t, w, actorDocumentMap(server.URL, "Person", publicKey))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &memoryRepository{}
	service := NewService(repo, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	actor, err := service.Discover(context.Background(), " AcCt:alice@"+serverHost(server)+" ")

	require.NoError(t, err)
	require.NotNil(t, repo.actor)
	assert.Equal(t, "https://remote.example/users/alice#main-key", actor.PublicKeyID)
	assert.Equal(t, "https://remote.example/users/alice#main-key", repo.actor.PublicKeyID)
	assert.Equal(t, server.URL+"/users/alice", repo.actor.APID)
	assert.Equal(t, "Person", repo.actor.Type)
	assert.Equal(t, "alice@"+serverHost(server), repo.actor.Handle)
	assert.Equal(t, server.URL+"/users/alice/inbox", repo.actor.InboxURL)
	assert.Equal(t, server.URL+"/users/alice/outbox", repo.actor.OutboxURL)
	require.NotNil(t, repo.actor.FollowersURL)
	assert.Equal(t, server.URL+"/users/alice/followers", *repo.actor.FollowersURL)
	assert.JSONEq(t, mustJSON(t, actorDocumentMap(server.URL, "Person", publicKey)), string(repo.actor.Document))
}

func TestDiscoverAcceptsActivityTypeArray(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	server := newDiscoveryServer(t, actorDocumentMap("", []any{"Service", "Application"}, publicKey))
	defer server.Close()

	repo := &memoryRepository{}
	service := NewService(repo, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	actor, err := service.Discover(context.Background(), "acct:bot@"+serverHost(server))

	require.NoError(t, err)
	assert.Equal(t, "Service", actor.Type)
	assert.Equal(t, "bot@"+serverHost(server), actor.Handle)
}

func TestDiscoverRejectsWebFingerWithoutSelfLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"subject": r.URL.Query().Get("resource"), "links": []any{}})
	}))
	defer server.Close()

	service := NewService(&memoryRepository{}, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	_, err := service.Discover(context.Background(), "acct:alice@"+serverHost(server))

	require.ErrorIs(t, err, ErrInvalidWebFinger)
}

func TestDiscoverRejectsInvalidActorDocument(t *testing.T) {
	doc := actorDocumentMap("", "Person", "not a public key")

	server := newDiscoveryServer(t, doc)
	defer server.Close()

	service := NewService(&memoryRepository{}, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	_, err := service.Discover(context.Background(), "acct:alice@"+serverHost(server))

	require.ErrorIs(t, err, ErrInvalidActorDocument)
}

func TestNormalizeAcctResource(t *testing.T) {
	username, domain, normalized, err := normalizeAcctResource(" AcCt:Alice@EXAMPLE.test ")

	require.NoError(t, err)
	assert.Equal(t, "Alice", username)
	assert.Equal(t, "example.test", domain)
	assert.Equal(t, "acct:Alice@example.test", normalized)

	for _, resource := range []string{"", "alice@example.test", "acct:", "acct:alice", "acct:alice@example test"} {
		t.Run(resource, func(t *testing.T) {
			_, _, _, err := normalizeAcctResource(resource)
			require.ErrorIs(t, err, ErrInvalidResource)
		})
	}
}

func newDiscoveryServer(t *testing.T, doc map[string]any) *httptest.Server {
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/webfinger":
			writeJSON(t, w, map[string]any{
				"subject": r.URL.Query().Get("resource"),
				"links": []map[string]any{
					{
						"rel":  "self",
						"type": "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"",
						"href": server.URL + "/users/remote",
					},
				},
			})
		case "/users/remote":
			doc["id"] = server.URL + "/users/remote"
			doc["inbox"] = server.URL + "/users/remote/inbox"
			doc["outbox"] = server.URL + "/users/remote/outbox"
			if publicKey, ok := doc["publicKey"].(map[string]any); ok {
				publicKey["owner"] = server.URL + "/users/remote"
			}
			writeJSON(t, w, doc)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func actorDocumentMap(baseURL string, actorType any, publicKey string) map[string]any {
	if baseURL == "" {
		baseURL = "https://remote.example"
	}
	return map[string]any{
		"@context":          "https://www.w3.org/ns/activitystreams",
		"id":                baseURL + "/users/alice",
		"type":              actorType,
		"preferredUsername": "alice",
		"name":              "Alice",
		"summary":           "Remote actor",
		"inbox":             baseURL + "/users/alice/inbox",
		"outbox":            baseURL + "/users/alice/outbox",
		"followers":         baseURL + "/users/alice/followers",
		"publicKey": map[string]any{
			"id":           "https://remote.example/users/alice#main-key",
			"owner":        baseURL + "/users/alice",
			"publicKeyPem": publicKey,
		},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func serverHost(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "http://")
}
