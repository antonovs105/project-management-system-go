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
	ActivityStreamsContext = "https://www.w3.org/ns/activitystreams"
	ForgeFedContext        = "https://forgefed.org/ns"
	PublicCollection       = "https://www.w3.org/ns/activitystreams#Public"
	ActivityJSONMediaType  = "application/activity+json"
)

type Config struct {
	BaseURL     string
	LocalDomain string
}

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

func UserAPID(cfg Config, username string) string {
	return fmt.Sprintf("%s/users/%s", cfg.BaseURL, url.PathEscape(username))
}

func ProjectAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/projects/%s", cfg.BaseURL, id)
}

func TicketAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/tickets/%s", cfg.BaseURL, id)
}

func CommentAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/comments/%s", cfg.BaseURL, id)
}

func ActivityAPID(cfg Config, id string) string {
	return fmt.Sprintf("%s/activities/%s", cfg.BaseURL, id)
}

func Inbox(apID string) string {
	return apID + "/inbox"
}

func Outbox(apID string) string {
	return apID + "/outbox"
}

func Followers(apID string) string {
	return apID + "/followers"
}

func Following(apID string) string {
	return apID + "/following"
}

func KeyID(apID string) string {
	return apID + "#main-key"
}

func Handle(username string, cfg Config) string {
	return username + "@" + cfg.LocalDomain
}

func Context() []any {
	return []any{
		ActivityStreamsContext,
		map[string]any{"forge": ForgeFedContext},
	}
}

func MarshalDocument(doc map[string]any) ([]byte, error) {
	return json.Marshal(doc)
}

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

func ProjectActorDocument(apID, name, summary, publicKey string) map[string]any {
	doc := ActorDocument("Group", apID, "project-"+lastPath(apID), name, summary, publicKey)
	doc["type"] = []string{"Group", "forge:Project", "forge:TicketTracker"}
	doc["tickets"] = apID + "/tickets"
	return doc
}

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

func TombstoneDocument(apID, formerType string, deletedAt time.Time) map[string]any {
	return map[string]any{
		"@context":   ActivityStreamsContext,
		"id":         apID,
		"type":       "Tombstone",
		"formerType": formerType,
		"deleted":    deletedAt.Format(time.RFC3339),
	}
}

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
