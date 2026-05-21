package activitypub

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// ActivityStreamsContext is the canonical JSON-LD context for ActivityStreams.
	ActivityStreamsContext = "https://www.w3.org/ns/activitystreams"
	// ForgeFedContext is the JSON-LD context used for ForgeFed ticket vocabulary.
	ForgeFedContext = "https://forgefed.org/ns"
	// PublicCollection is the ActivityStreams public audience identifier.
	PublicCollection = "https://www.w3.org/ns/activitystreams#Public"
	// ActivityJSONMediaType is the preferred media type for ActivityPub documents.
	ActivityJSONMediaType = "application/activity+json"
)

// Config contains local ActivityPub URL construction settings.
type Config struct {
	BaseURL     string
	LocalDomain string
}

// NewConfig normalizes ActivityPub base URL and local domain settings.
func NewConfig(baseURL, localDomain string) Config {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if localDomain == "" {
		if parsed, err := url.Parse(baseURL); err == nil {
			localDomain = parsed.Host
		}
	}
	if localDomain == "" {
		localDomain = "localhost:8080"
	}
	return Config{BaseURL: baseURL, LocalDomain: localDomain}
}

// NewID returns a random version 4 UUID string for local objects.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}

// UserAPID builds the canonical ActivityPub ID for a local user actor.
func UserAPID(cfg Config, username string) string {
	return fmt.Sprintf("%s/users/%s", cfg.BaseURL, url.PathEscape(username))
}

// ProjectAPID builds the canonical ActivityPub ID for a local project actor.
func ProjectAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/projects/%s", cfg.BaseURL, id)
}

// TicketAPID builds the canonical ActivityPub ID for a local ticket object.
func TicketAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/tickets/%s", cfg.BaseURL, id)
}

// CommentAPID builds the canonical ActivityPub ID for a local comment object.
func CommentAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/comments/%s", cfg.BaseURL, id)
}

// ActivityAPID builds the canonical ActivityPub ID for a local activity.
func ActivityAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/activities/%s", cfg.BaseURL, id)
}

// Inbox returns the inbox collection URL for an actor ID.
func Inbox(apID string) string {
	return apID + "/inbox"
}

// Outbox returns the outbox collection URL for an actor ID.
func Outbox(apID string) string {
	return apID + "/outbox"
}

// Followers returns the followers collection URL for an actor ID.
func Followers(apID string) string {
	return apID + "/followers"
}

// Following returns the following collection URL for an actor ID.
func Following(apID string) string {
	return apID + "/following"
}

// ProjectTickets returns the ticket collection URL for a project actor ID.
func ProjectTickets(apID string) string {
	return apID + "/tickets"
}

// KeyID returns the primary public-key fragment URL for an actor ID.
func KeyID(apID string) string {
	return apID + "#main-key"
}

// Handle builds a local actor handle from username and domain.
func Handle(username string, cfg Config) string {
	return username + "@" + cfg.LocalDomain
}

// Context returns the JSON-LD context shared by local ActivityPub documents.
func Context() []any {
	return []any{
		ActivityStreamsContext,
		map[string]any{"forge": ForgeFedContext},
	}
}

// MarshalDocument serializes an ActivityPub document.
func MarshalDocument(doc map[string]any) ([]byte, error) {
	return json.Marshal(doc)
}

// ActorDocument builds a Person or Group actor JSON-LD document.
func ActorDocument(actorType, apID, preferredUsername, name, summary, publicKey string) map[string]any {
	doc := map[string]any{
		"@context":                  Context(),
		"id":                        apID,
		"type":                      actorType,
		"preferredUsername":         preferredUsername,
		"name":                      name,
		"summary":                   summary,
		"inbox":                     Inbox(apID),
		"outbox":                    Outbox(apID),
		"followers":                 Followers(apID),
		"following":                 Following(apID),
		"manuallyApprovesFollowers": true,
	}
	if publicKey != "" {
		doc["publicKey"] = map[string]any{
			"id":           KeyID(apID),
			"owner":        apID,
			"publicKeyPem": publicKey,
		}
	}
	return doc
}

// ProjectActorDocument builds the JSON-LD document for a project actor.
func ProjectActorDocument(apID, name, summary, publicKey string) map[string]any {
	doc := ActorDocument("Group", apID, "project-"+lastPath(apID), name, summary, publicKey)
	doc["type"] = []string{"Group", "forge:Project", "forge:TicketTracker"}
	doc["tickets"] = ProjectTickets(apID)
	return doc
}

// TicketDocument builds a ForgeFed ticket JSON-LD document.
func TicketDocument(apID, projectAPID, attributedTo, title, description, status, priority, ticketType string, parentAPID *string, assignedTo []string, createdAt time.Time, isResolved bool) map[string]any {
	doc := map[string]any{
		"@context":         Context(),
		"id":               apID,
		"type":             "forge:Ticket",
		"attributedTo":     attributedTo,
		"context":          projectAPID,
		"name":             title,
		"content":          description,
		"forge:status":     status,
		"forge:priority":   priority,
		"forge:ticketType": ticketType,
		"forge:isResolved": isResolved,
		"published":        createdAt.Format(time.RFC3339),
	}
	if parentAPID != nil {
		doc["inReplyTo"] = *parentAPID
	}
	if len(assignedTo) > 0 {
		doc["forge:assignedTo"] = assignedTo
	}
	return doc
}

// NoteDocument builds a comment Note JSON-LD document.
func NoteDocument(apID, ticketAPID, attributedTo, content string, createdAt time.Time) map[string]any {
	return map[string]any{
		"@context":     Context(),
		"id":           apID,
		"type":         "Note",
		"attributedTo": attributedTo,
		"inReplyTo":    ticketAPID,
		"content":      content,
		"published":    createdAt.Format(time.RFC3339),
	}
}

// TombstoneDocument builds a Tombstone JSON-LD document for a deleted object.
func TombstoneDocument(apID, formerType string, deletedAt time.Time) map[string]any {
	return map[string]any{
		"@context":   ActivityStreamsContext,
		"id":         apID,
		"type":       "Tombstone",
		"formerType": formerType,
		"deleted":    deletedAt.Format(time.RFC3339),
	}
}

// ActivityDocument builds a generic ActivityStreams activity document.
func ActivityDocument(activityType, apID, actorAPID string, object any, target any, createdAt time.Time) map[string]any {
	doc := map[string]any{
		"@context":  Context(),
		"id":        apID,
		"type":      activityType,
		"actor":     actorAPID,
		"object":    object,
		"published": createdAt.Format(time.RFC3339),
	}
	if target != nil {
		doc["target"] = target
	}
	return doc
}

// CollectionDocument builds a simple ordered collection document with embedded items.
func CollectionDocument(apID, collectionType string, items []map[string]any) map[string]any {
	if collectionType == "" {
		collectionType = "OrderedCollection"
	}
	return map[string]any{
		"@context":     ActivityStreamsContext,
		"id":           apID,
		"type":         collectionType,
		"totalItems":   len(items),
		"orderedItems": items,
	}
}

// OrderedCollectionDocument builds an ActivityStreams OrderedCollection summary.
func OrderedCollectionDocument(apID string, totalItems int, first string) map[string]any {
	doc := map[string]any{
		"@context":   ActivityStreamsContext,
		"id":         apID,
		"type":       "OrderedCollection",
		"totalItems": totalItems,
	}
	if first != "" {
		doc["first"] = first
	}
	return doc
}

// OrderedCollectionPageDocument builds a paginated ActivityStreams collection page.
func OrderedCollectionPageDocument(apID, partOf string, totalItems int, orderedItems []any, next, prev string) map[string]any {
	doc := map[string]any{
		"@context":     ActivityStreamsContext,
		"id":           apID,
		"type":         "OrderedCollectionPage",
		"partOf":       partOf,
		"totalItems":   totalItems,
		"orderedItems": orderedItems,
	}
	if next != "" {
		doc["next"] = next
	}
	if prev != "" {
		doc["prev"] = prev
	}
	return doc
}

// GenerateRSAKeyPair creates PEM-encoded RSA key material for a local actor.
func GenerateRSAKeyPair() (publicPEM string, privatePEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	privateBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	privatePEM = string(pem.EncodeToMemory(privateBlock))

	publicBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes}
	publicPEM = string(pem.EncodeToMemory(publicBlock))
	return publicPEM, privatePEM, nil
}

func lastPath(apID string) string {
	idx := strings.LastIndex(apID, "/")
	if idx == -1 || idx == len(apID)-1 {
		return apID
	}
	return apID[idx+1:]
}
