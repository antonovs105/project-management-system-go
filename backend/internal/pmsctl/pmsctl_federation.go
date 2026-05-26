package pmsctl

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/jmoiron/sqlx"
)

// federationUserAgent identifies outbound ActivityPub requests made by pmsctl.
const federationUserAgent = "project-management-system-go/pmsctl"

// FederationDiscoverOptions carries remote actor discovery input.
type FederationDiscoverOptions struct {
	EnvFile  string
	Resource string
}

// FederationFollowOptions carries input for sending a signed Follow activity.
type FederationFollowOptions struct {
	EnvFile  string
	FromUser string
	Target   string
}

// FederationFollowResult describes a completed remote Follow POST.
type FederationFollowResult struct {
	ActivityAPID   string
	ActorAPID      string
	TargetAPID     string
	TargetInboxURL string
	StatusCode     int
	ResponseBody   string
}

// FederationAcceptFollowOptions carries input for approving a remote project follower.
type FederationAcceptFollowOptions struct {
	EnvFile      string
	ProjectID    string
	Actor        string
	SendResponse bool
}

// FederationAcceptFollowResult describes an accepted remote follow.
type FederationAcceptFollowResult struct {
	ResponseActivityAPID string
	ProjectAPID          string
	ActorAPID            string
	TargetInboxURL       string
	ResponseStatusCode   int
	ResponseBody         string
}

// localActor is the local actor identity needed for federation signing.
type localActor struct {
	ID       string `db:"id"`
	Username string `db:"username"`
	APID     string `db:"ap_id"`
}

// projectFollowActivity carries the stored inbound Follow selected for approval.
type projectFollowActivity struct {
	ProjectActorID string `db:"project_actor_id"`
	ProjectAPID    string `db:"project_ap_id"`
	FollowerID     string `db:"follower_id"`
	FollowerAPID   string `db:"follower_ap_id"`
	FollowAPID     string `db:"follow_ap_id"`
}

// runFederation dispatches federation maintenance subcommands.
func (r *Runner) runFederation(ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.printFederationUsage()
		return 2
	}
	switch args[0] {
	case "discover":
		return r.runFederationDiscover(ctx, args[1:])
	case "follow":
		return r.runFederationFollow(ctx, args[1:])
	case "accept-follow":
		return r.runFederationAcceptFollow(ctx, args[1:])
	case "help", "-h", "--help":
		r.printFederationUsage()
		return 0
	default:
		fmt.Fprintf(r.Stderr, "unknown federation command %q\n\n", args[0])
		r.printFederationUsage()
		return 2
	}
}

// runFederationDiscover parses and executes remote actor discovery.
func (r *Runner) runFederationDiscover(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("pmsctl federation discover", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	envFile := fs.String("env-file", defaultEnvFile, "environment file to load before connecting")
	resourceFlag := fs.String("resource", "", "remote acct: resource, bare handle, or actor URL")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl federation discover [--env-file FILE] RESOURCE")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	resource, ok := singleResourceArgument(fs, *resourceFlag)
	if !ok {
		fmt.Fprintln(r.Stderr, "missing or ambiguous remote resource")
		fs.Usage()
		return 2
	}
	options := FederationDiscoverOptions{EnvFile: strings.TrimSpace(*envFile), Resource: resource}
	if err := r.LoadEnvFile(options.EnvFile); err != nil {
		fmt.Fprintf(r.Stderr, "failed to load env file: %v\n", err)
		return 1
	}
	actor, err := r.DiscoverRemoteActor(ctx, options)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to discover remote actor: %v\n", err)
		return 1
	}
	fmt.Fprintf(r.Stdout, "actor_discovered ap_id=%s type=%s handle=%s inbox=%s outbox=%s\n", actor.APID, actor.Type, actor.Handle, actor.InboxURL, actor.OutboxURL)
	return 0
}

// runFederationFollow parses and sends a local signed Follow activity.
func (r *Runner) runFederationFollow(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("pmsctl federation follow", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	envFile := fs.String("env-file", defaultEnvFile, "environment file to load before connecting")
	fromUser := fs.String("from-user", "", "local username or email to sign the Follow")
	targetFlag := fs.String("target", "", "remote acct: resource, bare handle, or actor URL")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl federation follow --from-user USER [--env-file FILE] TARGET")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target, ok := singleResourceArgument(fs, *targetFlag)
	if !ok {
		fmt.Fprintln(r.Stderr, "missing or ambiguous follow target")
		fs.Usage()
		return 2
	}
	options := FederationFollowOptions{
		EnvFile:  strings.TrimSpace(*envFile),
		FromUser: strings.TrimSpace(*fromUser),
		Target:   target,
	}
	if options.FromUser == "" {
		fmt.Fprintln(r.Stderr, "missing required option: --from-user")
		fs.Usage()
		return 2
	}
	if err := r.LoadEnvFile(options.EnvFile); err != nil {
		fmt.Fprintf(r.Stderr, "failed to load env file: %v\n", err)
		return 1
	}
	result, err := r.FollowRemoteActor(ctx, options)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to follow remote actor: %v\n", err)
		return 1
	}
	fmt.Fprintf(r.Stdout, "follow_sent activity=%s actor=%s target=%s inbox=%s status=%d\n", result.ActivityAPID, result.ActorAPID, result.TargetAPID, result.TargetInboxURL, result.StatusCode)
	return 0
}

// runFederationAcceptFollow parses and accepts a remote project Follow.
func (r *Runner) runFederationAcceptFollow(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("pmsctl federation accept-follow", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	envFile := fs.String("env-file", defaultEnvFile, "environment file to load before connecting")
	projectID := fs.String("project-id", "", "local project UUID whose follower should be accepted")
	actor := fs.String("actor", "", "remote follower actor URL, acct: resource, or bare handle")
	sendResponse := fs.Bool("send-response", true, "send the Accept activity to the remote actor inbox")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl federation accept-follow --project-id PROJECT_ID --actor ACTOR [--env-file FILE]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*projectID) == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(r.Stderr, "missing required option(s): --project-id, --actor")
		fs.Usage()
		return 2
	}
	options := FederationAcceptFollowOptions{
		EnvFile:      strings.TrimSpace(*envFile),
		ProjectID:    strings.TrimSpace(*projectID),
		Actor:        strings.TrimSpace(*actor),
		SendResponse: *sendResponse,
	}
	if err := r.LoadEnvFile(options.EnvFile); err != nil {
		fmt.Fprintf(r.Stderr, "failed to load env file: %v\n", err)
		return 1
	}
	result, err := r.AcceptProjectFollow(ctx, options)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to accept project follow: %v\n", err)
		return 1
	}
	fmt.Fprintf(r.Stdout, "follow_accepted response_activity=%s project=%s actor=%s inbox=%s", result.ResponseActivityAPID, result.ProjectAPID, result.ActorAPID, result.TargetInboxURL)
	if result.ResponseStatusCode != 0 {
		fmt.Fprintf(r.Stdout, " status=%d", result.ResponseStatusCode)
	}
	fmt.Fprintln(r.Stdout)
	return 0
}

// discoverRemoteActor resolves and caches a remote ActivityPub actor.
func discoverRemoteActor(ctx context.Context, options FederationDiscoverOptions) (*remoteactor.Actor, error) {
	cfg, db, cleanup, err := openRuntimeDatabase(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return resolveRemoteActor(ctx, newRemoteActorService(db, cfg), options.Resource)
}

// followRemoteActor sends a signed Follow activity from a local user actor.
func followRemoteActor(ctx context.Context, options FederationFollowOptions) (*FederationFollowResult, error) {
	cfg, db, cleanup, err := openRuntimeDatabase(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	privateKeyCodec, err := secrets.NewPrivateKeyCodec(cfg.ActorPrivateKeyEncryptionKey)
	if err != nil {
		return nil, err
	}
	actor, err := findLocalUserActor(ctx, db, options.FromUser)
	if err != nil {
		return nil, err
	}
	remote, err := resolveRemoteActor(ctx, newRemoteActorService(db, cfg), options.Target)
	if err != nil {
		return nil, err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(activitypub.NewConfig(cfg.PublicBaseURL, cfg.LocalDomain), activityID)
	body, err := activitypub.MarshalDocument(map[string]any{
		"@context": activitypub.Context(),
		"id":       activityAPID,
		"type":     "Follow",
		"actor":    actor.APID,
		"object":   remote.APID,
	})
	if err != nil {
		return nil, err
	}

	if err := storeOutgoingFollow(ctx, db, actor.ID, remote.ID, activityID, activityAPID, remote.APID, body); err != nil {
		return nil, err
	}

	statusCode, responseBody, err := postSignedActivity(ctx, cfg, httpsig.NewService(httpsig.NewRepository(db, privateKeyCodec)), actor.ID, remote.InboxURL, body)
	result := &FederationFollowResult{
		ActivityAPID:   activityAPID,
		ActorAPID:      actor.APID,
		TargetAPID:     remote.APID,
		TargetInboxURL: remote.InboxURL,
		StatusCode:     statusCode,
		ResponseBody:   responseBody,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// storeOutgoingFollow records a local Follow before sending it to a remote actor.
func storeOutgoingFollow(ctx context.Context, db *sqlx.DB, actorID, remoteActorID, activityID, activityAPID, remoteActorAPID string, body []byte) error {
	if actorID == "" || remoteActorID == "" || activityID == "" || activityAPID == "" || remoteActorAPID == "" {
		return errors.New("cannot store incomplete follow activity")
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingState string
	err = tx.GetContext(ctx, &existingState, `
		SELECT state
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
		FOR UPDATE
	`, actorID, remoteActorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingState == "accepted" {
		return fmt.Errorf("local actor already follows %s", remoteActorAPID)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, document)
		VALUES ($1, $2, 'Follow', $3, $4, $5)
		ON CONFLICT (ap_id) DO NOTHING
	`, activityID, activityAPID, actorID, remoteActorAPID, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (actor_id, activity_ap_id) DO NOTHING
	`, actorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (follower_actor_id, followed_actor_id)
		DO UPDATE SET state = 'pending'
		WHERE actor_follows.state <> 'accepted'
	`, actorID, remoteActorID); err != nil {
		return err
	}

	return tx.Commit()
}

// acceptProjectFollow approves a stored remote Follow for a local project actor.
func acceptProjectFollow(ctx context.Context, options FederationAcceptFollowOptions) (*FederationAcceptFollowResult, error) {
	cfg, db, cleanup, err := openRuntimeDatabase(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	privateKeyCodec, err := secrets.NewPrivateKeyCodec(cfg.ActorPrivateKeyEncryptionKey)
	if err != nil {
		return nil, err
	}
	actorAPID, err := resolveActorAPIDForAccept(ctx, db, cfg, options.Actor)
	if err != nil {
		return nil, err
	}
	follow, err := findProjectFollowActivity(ctx, db, options.ProjectID, actorAPID)
	if err != nil {
		return nil, err
	}
	inbound := &remoteinbox.InboundActivity{
		ID:         follow.FollowAPID,
		Type:       "Follow",
		ActorAPID:  follow.FollowerAPID,
		ActorID:    follow.FollowerID,
		ObjectAPID: &follow.ProjectAPID,
	}
	response, err := remoteinbox.NewRepository(db, activitypub.NewConfig(cfg.PublicBaseURL, cfg.LocalDomain)).AcceptProjectFollow(ctx, follow.ProjectActorID, inbound)
	if err != nil {
		return nil, err
	}
	result := &FederationAcceptFollowResult{
		ResponseActivityAPID: response.ActivityAPID,
		ProjectAPID:          follow.ProjectAPID,
		ActorAPID:            follow.FollowerAPID,
		TargetInboxURL:       response.TargetInboxURL,
	}
	if !options.SendResponse {
		return result, nil
	}
	body, err := activityDocumentByID(ctx, db, response.ActivityID)
	if err != nil {
		return nil, err
	}
	statusCode, responseBody, err := postSignedActivity(ctx, cfg, httpsig.NewService(httpsig.NewRepository(db, privateKeyCodec)), follow.ProjectActorID, response.TargetInboxURL, body)
	result.ResponseStatusCode = statusCode
	result.ResponseBody = responseBody
	if err != nil {
		return result, err
	}
	return result, nil
}

// openRuntimeDatabase loads runtime configuration and connects to PostgreSQL.
func openRuntimeDatabase(ctx context.Context) (RuntimeConfig, *sqlx.DB, func(), error) {
	cfg, err := LoadRuntimeConfig()
	if err != nil {
		return RuntimeConfig{}, nil, nil, err
	}
	db, err := sqlx.Open("postgres", cfg.DBSource)
	if err != nil {
		return RuntimeConfig{}, nil, nil, err
	}
	cleanup := func() { _ = db.Close() }
	if err := db.PingContext(ctx); err != nil {
		cleanup()
		return RuntimeConfig{}, nil, nil, err
	}
	return cfg, db, cleanup, nil
}

// newRemoteActorService builds remote discovery with the current federation policy.
func newRemoteActorService(db *sqlx.DB, cfg RuntimeConfig) *remoteactor.Service {
	options := []remoteactor.Option{
		remoteactor.WithRequireHTTPS(!cfg.FederationAllowInsecureHTTP),
		remoteactor.WithAllowPrivateNetworks(cfg.FederationAllowPrivateNetworks),
	}
	if cfg.FederationAllowInsecureHTTP {
		options = append(options, remoteactor.WithWebFingerScheme("http"))
	}
	return remoteactor.NewService(remoteactor.NewRepository(db), options...)
}

// resolveRemoteActor resolves an acct handle or direct actor URL.
func resolveRemoteActor(ctx context.Context, service *remoteactor.Service, resource string) (*remoteactor.Actor, error) {
	resource = normalizeRemoteResource(resource)
	if strings.HasPrefix(strings.ToLower(resource), "acct:") {
		return service.Discover(ctx, resource)
	}
	if isHTTPURL(resource) {
		return service.Fetch(ctx, resource)
	}
	return nil, fmt.Errorf("%w: expected acct: resource, bare handle, or actor URL", remoteactor.ErrInvalidResource)
}

// resolveActorAPIDForAccept converts accept-follow actor input into an actor AP ID.
func resolveActorAPIDForAccept(ctx context.Context, db *sqlx.DB, cfg RuntimeConfig, actor string) (string, error) {
	actor = normalizeRemoteResource(actor)
	if isHTTPURL(actor) {
		return actor, nil
	}
	remote, err := resolveRemoteActor(ctx, newRemoteActorService(db, cfg), actor)
	if err != nil {
		return "", err
	}
	return remote.APID, nil
}

// postSignedActivity signs and posts one ActivityPub activity to an inbox.
func postSignedActivity(ctx context.Context, cfg RuntimeConfig, signer *httpsig.Service, actorID string, inboxURL string, body []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inboxURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", activitypub.ActivityJSONMediaType)
	req.Header.Set("Content-Type", activitypub.ActivityJSONMediaType)
	req.Header.Set("User-Agent", federationUserAgent)
	if err := signer.SignRequest(ctx, actorID, req, body); err != nil {
		return 0, "", err
	}
	resp, err := newFederationHTTPClient(cfg).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	responseBody, readErr := readResponseSnippet(resp.Body)
	if readErr != nil {
		return resp.StatusCode, responseBody, readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, responseBody, fmt.Errorf("remote inbox returned status %d: %s", resp.StatusCode, responseBody)
	}
	return resp.StatusCode, responseBody, nil
}

// newFederationHTTPClient builds an outbound client from runtime federation policy.
func newFederationHTTPClient(cfg RuntimeConfig) *http.Client {
	policy := make([]netguard.URLPolicyOption, 0, 2)
	if !cfg.FederationAllowInsecureHTTP {
		policy = append(policy, netguard.RequireHTTPS())
	}
	if cfg.FederationAllowPrivateNetworks {
		policy = append(policy, netguard.AllowPrivateNetworks())
	}
	return netguard.NewHTTPClientWithPolicy(20*time.Second, policy...)
}

// findLocalUserActor loads the local user actor used to sign a federation request.
func findLocalUserActor(ctx context.Context, db *sqlx.DB, usernameOrEmail string) (*localActor, error) {
	var actor localActor
	err := db.GetContext(ctx, &actor, `
		SELECT users.id::text AS id, users.username, actors.ap_id
		FROM users
		JOIN actors ON actors.id = users.id
		WHERE lower(users.username) = lower($1) OR lower(users.email) = lower($1)
		ORDER BY CASE WHEN lower(users.username) = lower($1) THEN 0 ELSE 1 END
		LIMIT 1
	`, strings.TrimSpace(usernameOrEmail))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("local user %q not found", usernameOrEmail)
	}
	if err != nil {
		return nil, err
	}
	return &actor, nil
}

// findProjectFollowActivity finds the latest stored Follow for a project and actor.
func findProjectFollowActivity(ctx context.Context, db *sqlx.DB, projectID string, actorAPID string) (*projectFollowActivity, error) {
	var follow projectFollowActivity
	err := db.GetContext(ctx, &follow, `
		SELECT
			project_actor.id::text AS project_actor_id,
			project_actor.ap_id AS project_ap_id,
			follower.id::text AS follower_id,
			follower.ap_id AS follower_ap_id,
			activity.ap_id AS follow_ap_id
		FROM projects project
		JOIN actors project_actor ON project_actor.id = project.id
		JOIN actor_inbox_items inbox_item ON inbox_item.actor_id = project_actor.id
		JOIN ap_activities activity ON activity.id = inbox_item.activity_id
		JOIN actors follower ON follower.id = activity.actor_id
		WHERE project.id = $1
			AND activity.activity_type = 'Follow'
			AND activity.object_ap_id = project_actor.ap_id
			AND follower.ap_id = $2
		ORDER BY inbox_item.received_at DESC
		LIMIT 1
	`, projectID, actorAPID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no pending follow from %s for project %s", actorAPID, projectID)
	}
	if err != nil {
		return nil, err
	}
	return &follow, nil
}

// activityDocumentByID loads the canonical JSON document for a stored activity.
func activityDocumentByID(ctx context.Context, db *sqlx.DB, activityID string) ([]byte, error) {
	var raw json.RawMessage
	if err := db.GetContext(ctx, &raw, `SELECT document FROM ap_activities WHERE id = $1`, activityID); err != nil {
		return nil, err
	}
	return raw, nil
}

// readResponseSnippet returns a bounded response body for CLI diagnostics.
func readResponseSnippet(body io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 2048))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// singleResourceArgument accepts either a resource flag or one positional resource.
func singleResourceArgument(fs *flag.FlagSet, resourceFlag string) (string, bool) {
	resource := strings.TrimSpace(resourceFlag)
	switch {
	case resource != "" && fs.NArg() == 0:
		return resource, true
	case resource == "" && fs.NArg() == 1:
		return strings.TrimSpace(fs.Arg(0)), strings.TrimSpace(fs.Arg(0)) != ""
	default:
		return "", false
	}
}

// normalizeRemoteResource expands bare ActivityPub handles into acct resources.
func normalizeRemoteResource(resource string) string {
	resource = strings.TrimSpace(resource)
	lower := strings.ToLower(resource)
	if strings.HasPrefix(lower, "acct:") || isHTTPURL(resource) {
		return resource
	}
	if strings.Contains(resource, "@") {
		return "acct:" + resource
	}
	return resource
}

// isHTTPURL reports whether raw is an absolute HTTP(S) URL.
func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

// printFederationUsage writes federation command help.
func (r *Runner) printFederationUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl federation <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  discover       Resolve and cache a remote ActivityPub actor")
	fmt.Fprintln(r.Stderr, "  follow         Send a signed Follow from a local user")
	fmt.Fprintln(r.Stderr, "  accept-follow  Accept a remote Follow for a local project")
}
