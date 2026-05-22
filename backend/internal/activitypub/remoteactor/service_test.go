package remoteactor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryRepository struct {
	actor            *Actor
	lookupActor      *Actor
	err              error
	findErr          error
	fetchFailureAPID string
	fetchError       string
	upsertCount      int
}

func (m *memoryRepository) UpsertRemoteActor(ctx context.Context, actor *Actor) error {
	if m.err != nil {
		return m.err
	}
	copy := *actor
	m.actor = &copy
	m.upsertCount++
	return nil
}

func (m *memoryRepository) RemoteActorByAPID(ctx context.Context, apID string) (*Actor, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	source := m.lookupActor
	if source == nil {
		source = m.actor
	}
	if source == nil || source.APID != apID {
		return nil, ErrNotFound
	}
	copy := *source
	return &copy, nil
}

func (m *memoryRepository) RecordRemoteActorFetchFailure(ctx context.Context, apID string, fetchError string) error {
	m.fetchFailureAPID = apID
	m.fetchError = fetchError
	return nil
}

type httpClientFunc func(req *http.Request) (*http.Response, error)

func (f httpClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
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

func TestResolveKeyFetchesAndCachesActor(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		doc := actorDocumentMap(server.URL, "Person", publicKey)
		doc["publicKey"].(map[string]any)["id"] = server.URL + "/users/alice#main-key"
		writeJSON(t, w, doc)
	}))
	defer server.Close()

	repo := &memoryRepository{}
	service := NewService(repo, WithHTTPClient(server.Client()))

	err = service.ResolveKey(context.Background(), server.URL+"/users/alice#main-key")

	require.NoError(t, err)
	require.NotNil(t, repo.actor)
	assert.Equal(t, server.URL+"/users/alice", repo.actor.APID)
	assert.Equal(t, server.URL+"/users/alice#main-key", repo.actor.PublicKeyID)
}

func TestRefreshKeyRequiresExpectedActor(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/bob" {
			http.NotFound(w, r)
			return
		}
		doc := actorDocumentMap(server.URL, "Person", publicKey)
		doc["publicKey"].(map[string]any)["id"] = server.URL + "/users/alice#main-key"
		writeJSON(t, w, doc)
	}))
	defer server.Close()

	repo := &memoryRepository{}
	service := NewService(repo, WithHTTPClient(server.Client()))

	err = service.RefreshKey(context.Background(), server.URL+"/users/alice#main-key", server.URL+"/users/bob")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
	assert.Nil(t, repo.actor)
}

func TestRefreshKeyFetchesExpectedActorURL(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var requestedPath string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		doc := actorDocumentMap(server.URL, "Person", publicKey)
		doc["publicKey"].(map[string]any)["id"] = server.URL + "/keys/alice-main"
		writeJSON(t, w, doc)
	}))
	defer server.Close()

	repo := &memoryRepository{}
	service := NewService(repo, WithHTTPClient(server.Client()))

	err = service.RefreshKey(context.Background(), server.URL+"/keys/alice-main", server.URL+"/users/alice")

	require.NoError(t, err)
	assert.Equal(t, "/users/alice", requestedPath)
	require.NotNil(t, repo.actor)
	assert.Equal(t, server.URL+"/users/alice", repo.actor.APID)
	assert.Equal(t, server.URL+"/keys/alice-main", repo.actor.PublicKeyID)
}

func TestResolveKeyRejectsUnsupportedScheme(t *testing.T) {
	service := NewService(&memoryRepository{})

	err := service.ResolveKey(context.Background(), "ftp://remote.example/users/alice#main-key")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
}

func TestResolveKeyRejectsKeyMismatch(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		doc := actorDocumentMap(server.URL, "Person", publicKey)
		doc["publicKey"].(map[string]any)["id"] = server.URL + "/users/alice#other-key"
		writeJSON(t, w, doc)
	}))
	defer server.Close()

	repo := &memoryRepository{}
	err = NewService(repo, WithHTTPClient(server.Client())).ResolveKey(context.Background(), server.URL+"/users/alice#main-key")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
	assert.Nil(t, repo.actor)
}

func TestFetchRejectsPublicKeyOwnerMismatch(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		doc := actorDocumentMap(server.URL, "Person", publicKey)
		doc["publicKey"].(map[string]any)["owner"] = server.URL + "/users/mallory"
		writeJSON(t, w, doc)
	}))
	defer server.Close()

	repo := &memoryRepository{}
	_, err = NewService(repo, WithHTTPClient(server.Client())).Fetch(context.Background(), server.URL+"/users/alice")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
	assert.Nil(t, repo.actor)
	assert.Equal(t, server.URL+"/users/alice", repo.fetchFailureAPID)
	assert.Contains(t, repo.fetchError, "publicKey owner mismatch")
}

func TestFetchRejectsUnsupportedActorScheme(t *testing.T) {
	service := NewService(&memoryRepository{})

	_, err := service.Fetch(context.Background(), "ftp://remote.example/users/alice")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
}

func TestFetchRejectsHTTPWhenHTTPSRequired(t *testing.T) {
	repo := &memoryRepository{}
	service := NewService(repo, WithRequireHTTPS(true))

	_, err := service.Fetch(context.Background(), "http://93.184.216.34/users/alice")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
	assert.Equal(t, "http://93.184.216.34/users/alice", repo.fetchFailureAPID)
}

func TestFetchRejectsUnsafeActorHost(t *testing.T) {
	service := NewService(&memoryRepository{})

	_, err := service.Fetch(context.Background(), "http://127.0.0.1/users/alice")

	require.ErrorIs(t, err, netguard.ErrUnsafeURL)
}

func TestFetchRejectsHTTPEndpointsWhenHTTPSRequired(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	repo := &memoryRepository{}
	service := NewService(repo, WithRequireHTTPS(true), WithHTTPClient(httpClientFunc(func(req *http.Request) (*http.Response, error) {
		doc := actorDocumentMap("https://remote.example", "Person", publicKey)
		doc["inbox"] = "http://93.184.216.34/inbox"
		return jsonResponse(t, http.StatusOK, doc), nil
	})))

	_, err = service.Fetch(context.Background(), "https://remote.example/users/alice")

	require.ErrorIs(t, err, ErrInvalidActorDocument)
	assert.Nil(t, repo.actor)
	assert.Contains(t, repo.fetchError, "actor inbox url")
}

func TestRefreshIfStaleSkipsFreshRemoteActor(t *testing.T) {
	repo := &memoryRepository{
		lookupActor: &Actor{
			APID:          "https://remote.example/users/alice",
			LastFetchedAt: timePtr(time.Now().UTC()),
		},
	}
	service := NewService(repo, WithHTTPClient(httpClientFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("fresh actor should not be fetched")
		return nil, nil
	})))

	err := service.RefreshIfStale(context.Background(), "https://remote.example/users/alice", 24*time.Hour)

	require.NoError(t, err)
	assert.Zero(t, repo.upsertCount)
}

func TestRefreshIfStaleFetchesAndCachesStaleRemoteActor(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, actorDocumentMap(server.URL, "Person", publicKey))
	}))
	defer server.Close()

	repo := &memoryRepository{
		lookupActor: &Actor{
			APID:          server.URL + "/users/alice",
			LastFetchedAt: timePtr(time.Now().UTC().Add(-25 * time.Hour)),
		},
	}
	service := NewService(repo, WithHTTPClient(server.Client()))

	err = service.RefreshIfStale(context.Background(), server.URL+"/users/alice", 24*time.Hour)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.upsertCount)
	require.NotNil(t, repo.actor)
	assert.Equal(t, server.URL+"/users/alice", repo.actor.APID)
	assert.Equal(t, server.URL+"/users/alice/inbox", repo.actor.InboxURL)
}

func TestRefreshIfStaleRecordsFetchFailure(t *testing.T) {
	repo := &memoryRepository{
		lookupActor: &Actor{
			APID:          "https://remote.example/users/alice",
			LastFetchedAt: timePtr(time.Now().UTC().Add(-25 * time.Hour)),
		},
	}
	service := NewService(repo, WithHTTPClient(httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusGone,
			Status:     "410 Gone",
			Body:       http.NoBody,
		}, nil
	})))

	err := service.RefreshIfStale(context.Background(), "https://remote.example/users/alice", 24*time.Hour)

	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, "https://remote.example/users/alice", repo.fetchFailureAPID)
	assert.Contains(t, repo.fetchError, "remote actor not found")
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

func TestDiscoverRejectsHTTPWebFingerWhenHTTPSRequired(t *testing.T) {
	service := NewService(&memoryRepository{}, WithWebFingerScheme("http"), WithRequireHTTPS(true))

	_, err := service.Discover(context.Background(), "acct:alice@example.test")

	require.ErrorIs(t, err, ErrInvalidWebFinger)
}

func TestNewServiceRecordsPrivateNetworkPolicy(t *testing.T) {
	service := NewService(&memoryRepository{}, WithAllowPrivateNetworks(true))

	assert.True(t, service.allowPrivateNetworks)
}

func TestDiscoverRejectsUnsupportedSelfLinkScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"subject": r.URL.Query().Get("resource"),
			"links": []map[string]any{
				{
					"rel":  "self",
					"type": "application/activity+json",
					"href": "ftp://remote.example/users/alice",
				},
			},
		})
	}))
	defer server.Close()

	service := NewService(&memoryRepository{}, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	_, err := service.Discover(context.Background(), "acct:alice@"+serverHost(server))

	require.ErrorIs(t, err, ErrInvalidActorDocument)
}

func TestDiscoverRejectsInvalidActorDocument(t *testing.T) {
	doc := actorDocumentMap("", "Person", "not a public key")

	server := newDiscoveryServer(t, doc)
	defer server.Close()

	service := NewService(&memoryRepository{}, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	_, err := service.Discover(context.Background(), "acct:alice@"+serverHost(server))

	require.ErrorIs(t, err, ErrInvalidActorDocument)
}

func TestDiscoverRejectsInvalidActorEndpoints(t *testing.T) {
	publicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/webfinger":
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
			doc := actorDocumentMap(server.URL, "Person", publicKey)
			doc["inbox"] = "file:///tmp/inbox"
			writeJSON(t, w, doc)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(&memoryRepository{}, WithHTTPClient(server.Client()), WithWebFingerScheme("http"))

	_, err = service.Discover(context.Background(), "acct:alice@"+serverHost(server))

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

func jsonResponse(t *testing.T, statusCode int, value any) *http.Response {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
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

func timePtr(value time.Time) *time.Time {
	return &value
}
