// Package main boots the HTTP API server and wires backend vertical slices together.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activityhistory"
	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/c2s"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	apfederation "github.com/antonovs105/project-management-system-go/internal/activitypub/federation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	apmoderation "github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/adminaudit"
	"github.com/antonovs105/project-management-system-go/internal/apiresponse"
	"github.com/antonovs105/project-management-system-go/internal/attachment"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/githubintegration"
	"github.com/antonovs105/project-management-system-go/internal/instance"
	"github.com/antonovs105/project-management-system-go/internal/label"
	authMiddleware "github.com/antonovs105/project-management-system-go/internal/middleware"
	"github.com/antonovs105/project-management-system-go/internal/notification"
	"github.com/antonovs105/project-management-system-go/internal/observability"
	"github.com/antonovs105/project-management-system-go/internal/project"
	appratelimit "github.com/antonovs105/project-management-system-go/internal/ratelimit"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/antonovs105/project-management-system-go/internal/webfinger"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

const (
	// defaultHTTPAddr is the in-container API listen address.
	defaultHTTPAddr = ":8080"
	// gracefulShutdownTimeout bounds HTTP and worker shutdown on container stop.
	gracefulShutdownTimeout = 10 * time.Second
	// defaultRequestBodyLimit is the maximum HTTP body size accepted by generic routes.
	defaultRequestBodyLimit = "12M"
	// defaultRequestBodyLimitBytes mirrors defaultRequestBodyLimit for tests and comments.
	defaultRequestBodyLimitBytes = 12 << 20
	// authRateLimitPerSecond limits public credential and registration attempts per IP.
	authRateLimitPerSecond = 2
	// authRateLimitBurst allows small legitimate login or setup bursts.
	authRateLimitBurst = 10
	// discoveryRateLimitPerSecond limits WebFinger discovery requests per IP.
	discoveryRateLimitPerSecond = 10
	// discoveryRateLimitBurst allows normal ActivityPub discovery fan-out bursts.
	discoveryRateLimitBurst = 50
	// inboxRateLimitPerSecond limits inbound federation POSTs per IP.
	inboxRateLimitPerSecond = 20
	// inboxRateLimitBurst allows remote servers to deliver short federation bursts.
	inboxRateLimitBurst = 100
	// metricsReadHeaderTimeout bounds standalone metrics server header reads.
	metricsReadHeaderTimeout = 5 * time.Second
	// apiReadHeaderTimeout bounds time spent reading request headers.
	apiReadHeaderTimeout = 5 * time.Second
	// apiReadTimeout bounds time spent reading complete requests.
	apiReadTimeout = 15 * time.Second
	// apiWriteTimeout bounds time spent writing responses.
	apiWriteTimeout = 30 * time.Second
	// apiIdleTimeout bounds idle keep-alive connections.
	apiIdleTimeout = 60 * time.Second
)

const (
	// appEnvDevelopment enables local defaults and relaxed deployment checks.
	appEnvDevelopment = "development"
	// appEnvTest marks test processes that still use development-grade defaults.
	appEnvTest = "test"
	// appEnvProduction enables strict deployment safety checks.
	appEnvProduction = "production"

	// appRoleAPI runs only the HTTP API process.
	appRoleAPI appRole = "api"
	// appRoleWorker runs only the ActivityPub delivery worker process.
	appRoleWorker appRole = "worker"
	// appRoleAll runs the HTTP API and the delivery worker in one process.
	appRoleAll appRole = "all"
)

// requiredDatabaseTables lists schema objects that must exist before the API is ready.
var requiredDatabaseTables = []string{
	"actors",
	"actor_keys",
	"users",
	"account_tokens",
	"user_sessions",
	"user_mfa_credentials",
	"auth_events",
	"email_outbox",
	"projects",
	"project_members",
	"project_roles",
	"project_role_permissions",
	"actor_follows",
	"tickets",
	"project_labels",
	"ticket_labels",
	"project_activity_events",
	"ticket_attachments",
	"ticket_assignees",
	"comments",
	"ap_objects",
	"ap_activities",
	"actor_inbox_items",
	"actor_outbox_items",
	"activity_deliveries",
	"oauth_identities",
	"oauth_login_codes",
	"project_github_repositories",
	"github_commits",
	"github_commit_ticket_links",
	"notifications",
	"notification_preferences",
}

// appRole selects which server responsibilities this process owns.
type appRole string

// ApiServer wires database-backed services into the HTTP server.
type ApiServer struct {
	db                  *sqlx.DB
	redisClient         *redis.Client
	metrics             *observability.Metrics
	metricsToken        string
	userHandler         *user.Handler
	accountHandler      *account.Handler
	activityHandler     *activityhistory.Handler
	attachmentHandler   *attachment.Handler
	instanceHandler     *instance.Handler
	projectHandler      *project.Handler
	labelHandler        *label.Handler
	ticketHandler       *ticket.Handler
	commentHandler      *comment.Handler
	notificationHandler *notification.Handler
	githubHandler       *githubintegration.Handler
	apHandler           *activitypub.Handler
	c2sHandler          *c2s.Handler
	inboxHandler        *remoteinbox.Handler
	federationHandler   *apfederation.Handler
	moderationHandler   *apmoderation.Handler
	deliveryHandler     *delivery.Handler
	auditHandler        *adminaudit.Handler
	deliverySvc         *delivery.Service
	wfHandler           *webfinger.Handler
}

// ticketEventBus combines ticket publish and subscribe behavior for HTTP wiring.
type ticketEventBus interface {
	ticket.EventPublisher
	ticket.EventSubscriber
}

// notificationEventBus combines notification publish and subscribe behavior for HTTP wiring.
type notificationEventBus interface {
	notification.EventPublisher
	notification.EventSubscriber
}

// signatureActorVerifier adapts HTTP signature verification to ActivityPub authorization.
type signatureActorVerifier struct {
	service *httpsig.Service
}

// VerifyActorID validates an HTTP signature request and returns the signer actor ID.
func (v signatureActorVerifier) VerifyActorID(ctx context.Context, req *http.Request) (string, error) {
	verified, err := v.service.VerifyRequest(ctx, req, nil)
	if err != nil {
		return "", err
	}
	return verified.ActorID, nil
}

// splitCSVEnv parses a comma-separated environment variable into trimmed values.
func splitCSVEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

// parseAppEnv validates the deployment environment name.
func parseAppEnv(raw string) (string, error) {
	env := strings.ToLower(strings.TrimSpace(raw))
	if env == "" {
		return appEnvDevelopment, nil
	}
	switch env {
	case appEnvDevelopment, appEnvTest, appEnvProduction:
		return env, nil
	default:
		return "", fmt.Errorf("APP_ENV must be one of development, test, or production")
	}
}

// parseAppRole validates the configured process role.
func parseAppRole(raw string) (appRole, error) {
	role := appRole(strings.ToLower(strings.TrimSpace(raw)))
	if role == "" {
		role = appRoleAll
	}
	switch role {
	case appRoleAPI, appRoleWorker, appRoleAll:
		return role, nil
	default:
		return "", fmt.Errorf("APP_ROLE must be one of api, worker, or all")
	}
}

// runsAPI reports whether the process role serves HTTP traffic.
func (r appRole) runsAPI() bool {
	return r == appRoleAPI || r == appRoleAll
}

// runsWorker reports whether the process role processes Asynq delivery tasks.
func (r appRole) runsWorker() bool {
	return r == appRoleWorker || r == appRoleAll
}

// validateRuntimeConfig rejects unsafe deployment configuration before the server starts.
func validateRuntimeConfig(production bool, jwtSecret, publicBaseURL, localDomain, metricsToken, actorPrivateKeyEncryptionKey string) error {
	parsedBaseURL, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an absolute HTTP URL")
	}
	if parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL must use http or https")
	}
	if strings.ContainsAny(localDomain, " \t\r\n/") {
		return fmt.Errorf("LOCAL_DOMAIN must be a host name, not a URL")
	}
	if !production {
		return nil
	}

	if jwtSecret == "your_secret_key_here" || len(jwtSecret) < 32 {
		return fmt.Errorf("JWT_SECRET_KEY must be a production-grade secret")
	}
	if parsedBaseURL.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL must use https in production")
	}
	if isLocalHost(parsedBaseURL.Hostname()) || isLocalHost(localDomain) {
		return fmt.Errorf("PUBLIC_BASE_URL and LOCAL_DOMAIN must not use localhost in production")
	}
	if len(metricsToken) < 32 {
		return fmt.Errorf("METRICS_TOKEN must be at least 32 characters in production")
	}
	if len(actorPrivateKeyEncryptionKey) < 32 {
		return fmt.Errorf("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY must be at least 32 characters in production")
	}
	return nil
}

// optionalBoolEnv parses a boolean environment variable when it is set.
func optionalBoolEnv(name string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, false, nil
	}
	switch strings.ToLower(raw) {
	case "true", "1", "yes":
		return true, true, nil
	case "false", "0", "no":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a boolean", name)
	}
}

// validateCORSConfig rejects browser origins that are unsafe for production.
func validateCORSConfig(production bool, origins []string) error {
	if !production {
		return nil
	}
	if len(origins) == 0 {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS environment variable is required in production")
	}
	for _, origin := range origins {
		if origin == "*" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must not include wildcard origins in production")
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must contain absolute HTTP origins")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must use http or https")
		}
		if isLocalHost(parsed.Hostname()) {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must not use localhost in production")
		}
	}
	return nil
}

// configureIPExtractor chooses how Echo resolves client IPs for logs and rate limits.
func configureIPExtractor(e *echo.Echo, trustedProxyCIDRs []string) error {
	extractor, err := trustedProxyIPExtractor(trustedProxyCIDRs)
	if err != nil {
		return err
	}
	e.IPExtractor = extractor
	return nil
}

// trustedProxyIPExtractor trusts forwarded IP headers only from configured proxies.
func trustedProxyIPExtractor(trustedProxyCIDRs []string) (echo.IPExtractor, error) {
	if len(trustedProxyCIDRs) == 0 {
		return echo.ExtractIPDirect(), nil
	}

	options := []echo.TrustOption{
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	}
	for _, cidr := range trustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain CIDR ranges")
		}
		options = append(options, echo.TrustIPRange(network))
	}
	return echo.ExtractIPFromXFFHeader(options...), nil
}

// isLocalHost reports whether host points to the loopback development host.
func isLocalHost(host string) bool {
	host = strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// main builds all backend services, registers routes, and starts the API server.
func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("failed to load .env: %v", err)
	}
	cfg, err := appconfig.Load()
	if err != nil {
		log.Fatal(err)
	}
	appEnv, err := parseAppEnv(cfg.App.Env)
	if err != nil {
		log.Fatal(err)
	}
	production := appEnv == appEnvProduction
	role, err := parseAppRole(cfg.App.Role)
	if err != nil {
		log.Fatal(err)
	}
	dbSource := cfg.Database.Source
	if dbSource == "" {
		log.Fatal("DB_SOURCE environment variable is not set")
	}
	jwtSecret := cfg.Security.JWTSecretKey
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET_KEY environment variable is not set")
	}
	publicBaseURL := cfg.Instance.PublicBaseURL
	if publicBaseURL == "" {
		log.Fatal("PUBLIC_BASE_URL environment variable is not set")
	}
	localDomain := cfg.Instance.LocalDomain
	if localDomain == "" {
		log.Fatal("LOCAL_DOMAIN environment variable is not set")
	}
	redisAddr := cfg.Redis.Addr
	metricsAddr := cfg.Metrics.Addr
	metricsToken := cfg.Metrics.Token
	actorPrivateKeyEncryptionKey := cfg.Security.ActorPrivateKeyEncryptionKey
	allowInsecureFederationHTTP := cfg.FederationAllowInsecureHTTP()
	if production && allowInsecureFederationHTTP {
		log.Fatal("FEDERATION_ALLOW_INSECURE_HTTP cannot be enabled in production")
	}
	allowPrivateFederationNetworks := cfg.FederationAllowPrivateNetworks()
	if production && allowPrivateFederationNetworks {
		log.Fatal("FEDERATION_ALLOW_PRIVATE_NETWORKS cannot be enabled in production")
	}
	requireHTTPSFederation := !allowInsecureFederationHTTP
	if err := validateRuntimeConfig(production, jwtSecret, publicBaseURL, localDomain, metricsToken, actorPrivateKeyEncryptionKey); err != nil {
		log.Fatal(err)
	}
	privateKeyCodec, err := secrets.NewPrivateKeyCodec(actorPrivateKeyEncryptionKey, cfg.Security.ActorPrivateKeyPreviousKeys...)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("backend_runtime_start app_env=%s role=%s public_base_url=%s local_domain=%s redis_configured=%t metrics_addr=%s federation_https_required=%t federation_private_networks_allowed=%t", appEnv, role, publicBaseURL, localDomain, redisAddr != "", metricsAddr, requireHTTPSFederation, allowPrivateFederationNetworks)
	apConfig := activitypub.NewConfig(publicBaseURL, localDomain)
	metrics := observability.NewMetrics()
	metricsServer := startMetricsServer(metrics, metricsAddr, metricsToken)
	defer shutdownMetricsServer(metricsServer, gracefulShutdownTimeout)

	db, err := sqlx.Connect("postgres", dbSource)
	if err != nil {
		log.Fatalf("Can't connect to DB: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.Database.MaxOpenConnections)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConnections)
	db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeSeconds) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(cfg.Database.ConnMaxIdleTimeSeconds) * time.Second)
	metrics.RegisterDBStats(db.DB, "primary")

	log.Println("DB connection successful")

	var redisClient *redis.Client
	if redisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
		defer redisClient.Close()
	}

	userRepo := user.NewRepository(db, apConfig, privateKeyCodec)
	accountRepository := account.NewRepository(db)
	var accountMailer account.Mailer
	if cfg.Email.Host != "" {
		accountMailer, err = account.NewSMTPMailer(account.SMTPConfig{
			Host:        cfg.Email.Host,
			Port:        cfg.Email.Port,
			Username:    cfg.Email.Username,
			Password:    cfg.Email.Password,
			FromAddress: cfg.Email.FromAddress,
			FromName:    cfg.Email.FromName,
			ImplicitTLS: cfg.Email.ImplicitTLS,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	accountService := account.NewService(accountRepository, publicBaseURL, cfg.Instance.Name, accountMailer, !production, account.WithSecretCodec(privateKeyCodec))
	userService := user.NewService(
		userRepo,
		[]byte(jwtSecret),
		apConfig,
		user.WithRegistrationEnabled(cfg.Registration.Enabled),
		user.WithAccountSecurity(accountService),
		user.WithOAuthConfig(user.OAuthConfig{
			FrontendCallbackURL: cfg.OAuth.FrontendCallbackURL,
			Google: user.OAuthProviderConfig{
				ClientID:     cfg.OAuth.Google.ClientID,
				ClientSecret: cfg.OAuth.Google.ClientSecret,
				RedirectURL:  cfg.OAuth.Google.RedirectURL,
			},
			GitHub: user.OAuthProviderConfig{
				ClientID:     cfg.OAuth.GitHub.ClientID,
				ClientSecret: cfg.OAuth.GitHub.ClientSecret,
				RedirectURL:  cfg.OAuth.GitHub.RedirectURL,
			},
		}),
	)
	userHandler := user.NewHandler(userService)
	accountHandler := account.NewHandler(accountService)
	instanceHandler := instance.NewHandler(cfg, userService, userService)

	projectRepo := project.NewRepository(db, apConfig, privateKeyCodec)
	projectService := project.NewService(projectRepo, apConfig)
	activityHandler := activityhistory.NewHandler(activityhistory.NewService(activityhistory.NewRepository(db), projectRepo))
	var attachmentHandler *attachment.Handler
	var attachmentService *attachment.Service
	if cfg.Attachments.Enabled {
		attachmentStore, err := attachment.NewLocalStore(cfg.Attachments.StoragePath)
		if err != nil {
			log.Fatal(err)
		}
		var attachmentScanner attachment.MalwareScanner
		if cfg.Attachments.ClamAVAddr != "" {
			attachmentScanner = attachment.NewClamAVScanner(cfg.Attachments.ClamAVAddr)
		}
		attachmentService = attachment.NewService(attachment.NewRepository(db), attachmentStore, projectRepo, attachmentScanner)
		if role.runsAPI() {
			attachmentHandler = attachment.NewHandler(attachmentService)
		}
	}
	projectHandler := project.NewHandler(
		projectService,
		project.WithInstanceRoleProvider(userService),
		project.WithProjectCreationPolicy(cfg.Projects.CreationPolicy),
	)
	labelHandler := label.NewHandler(label.NewService(label.NewRepository(db), projectService))

	ticketRepo := ticket.NewRepository(db, apConfig)
	ticketService := ticket.NewService(ticketRepo, projectService, apConfig)
	var ticketEvents ticketEventBus = ticket.NewEventHub()
	if redisClient != nil && role.runsAPI() {
		redisTicketEvents := ticket.NewRedisEventHub(redisClient)
		defer redisTicketEvents.Close()
		ticketEvents = redisTicketEvents
	}
	ticketService.SetEventPublisher(ticketEvents)
	ticketHandler := ticket.NewHandler(ticketService, ticket.WithEventSubscriber(ticketEvents))

	var notificationEvents notificationEventBus = notification.NewEventHub()
	if redisClient != nil && role.runsAPI() {
		redisNotificationEvents := notification.NewRedisEventHub(redisClient)
		defer redisNotificationEvents.Close()
		notificationEvents = redisNotificationEvents
	}
	notificationService := notification.NewService(
		notification.NewRepository(db),
		notification.WithEventPublisher(notificationEvents),
		notification.WithEmailQueue(accountRepository, publicBaseURL),
	)
	accountService.SetSecurityNotificationSink(notificationService)
	projectService.SetNotificationSink(notificationService)
	ticketService.SetNotificationSink(notificationService)
	notificationHandler := notification.NewHandler(notificationService, notification.WithEventSubscriber(notificationEvents))

	commentRepo := comment.NewRepository(db, apConfig)
	commentService := comment.NewService(commentRepo, ticketService, apConfig)
	commentService.SetNotificationSink(notificationService)
	commentHandler := comment.NewHandler(commentService)

	githubClient := githubintegration.NewHTTPClient(githubintegration.WithToken(cfg.GitHub.APIToken))
	githubService := githubintegration.NewService(githubintegration.NewRepository(db), projectService, githubClient)
	githubHandler := githubintegration.NewHandler(
		githubService,
		githubintegration.WithWebhookSecret(cfg.GitHub.WebhookSecret),
	)

	c2sHandler := c2s.NewHandler(db, apConfig, ticketService, commentService)

	// Remote ActivityPub signing/discovery dependencies
	remoteActorRepo := remoteactor.NewRepository(db)
	remoteActorOptions := []remoteactor.Option{
		remoteactor.WithRequireHTTPS(requireHTTPSFederation),
		remoteactor.WithAllowPrivateNetworks(allowPrivateFederationNetworks),
	}
	if allowInsecureFederationHTTP {
		remoteActorOptions = append(remoteActorOptions, remoteactor.WithWebFingerScheme("http"))
	}
	remoteActorService := remoteactor.NewService(remoteActorRepo, remoteActorOptions...)
	sigRepo := httpsig.NewRepository(db, privateKeyCodec)
	sigService := httpsig.NewService(
		sigRepo,
		httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
		httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
	)
	apHandler := activitypub.NewHandlerWithAuthorizer(
		db,
		apConfig,
		activitypub.NewAccessAuthorizer(db, []byte(jwtSecret), signatureActorVerifier{service: sigService}, userService),
	)

	// ActivityPub delivery dependencies. API roles enqueue deliveries, while worker roles process them from Redis.
	deliveryRepo := delivery.NewRecipientRepository(db)
	var deliveryQueue delivery.Queue = delivery.NoopQueue{}
	if redisAddr != "" {
		redisOpt := asynq.RedisClientOpt{Addr: redisAddr}
		deliveryQueue = delivery.NewAsynqQueue(redisOpt)
		defer deliveryQueue.Close()

		if role.runsWorker() {
			deliveryWorker := delivery.NewWorker(
				deliveryRepo,
				sigService,
				nil,
				delivery.WithRemoteActorRefresher(remoteActorService),
				delivery.WithMetrics(metrics),
				delivery.WithFailureNotifier(notificationService),
				delivery.WithRequireHTTPS(requireHTTPSFederation),
				delivery.WithAllowPrivateNetworks(allowPrivateFederationNetworks),
			)
			deliveryServer := delivery.NewAsynqServer(redisOpt)
			if err := deliveryServer.Start(delivery.NewServeMux(deliveryWorker)); err != nil {
				log.Fatalf("Can't start ActivityPub delivery worker: %v", err)
			}
			defer deliveryServer.Shutdown()

			log.Printf("ActivityPub delivery worker started with Redis at %s", redisAddr)
		}
	} else {
		if production || role.runsWorker() {
			log.Fatal("REDIS_ADDR environment variable is required for production or worker roles")
		}
		log.Println("REDIS_ADDR is not set; ActivityPub delivery worker disabled")
	}
	deliveryService := delivery.NewService(deliveryRepo, deliveryQueue)
	if redisAddr != "" {
		stopDeliveryRecovery := deliveryService.StartRecoveryLoop(context.Background(), 30*time.Second, 100)
		defer stopDeliveryRecovery()
	}
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)
	deliveryHandler := delivery.NewHandler(deliveryService)

	// Remote ActivityPub inbox dependencies
	inboxRepo := remoteinbox.NewRepository(db, apConfig)
	inboxService := remoteinbox.NewService(
		inboxRepo,
		sigService,
		remoteinbox.WithDelivery(deliveryService),
		remoteinbox.WithBlockedDomains(cfg.Federation.BlockedDomains),
	)
	inboxHandler := remoteinbox.NewHandler(inboxService, apConfig)
	federationHandler := apfederation.NewHandler(apfederation.NewService(
		apfederation.NewRepository(db),
		apfederation.WithConfig(apConfig),
		apfederation.WithRemoteActorResolver(remoteActorService),
		apfederation.WithDelivery(deliveryService),
		apfederation.WithSigner(sigService),
		apfederation.WithRemoteRequestPolicy(requireHTTPSFederation, allowPrivateFederationNetworks),
	))
	moderationHandler := apmoderation.NewHandler(apmoderation.NewService(apmoderation.NewRepository(db), deliveryQueue))
	auditHandler := adminaudit.NewHandler(adminaudit.NewService(adminaudit.NewRepository(db)))

	// WebFinger discovery dependencies
	wfRepo := webfinger.NewRepository(db)
	wfService := webfinger.NewService(wfRepo, apConfig)
	wfHandler := webfinger.NewHandler(wfService)
	if role.runsWorker() {
		stopEmailDispatcher := accountService.StartEmailDispatcher(context.Background(), 5*time.Second)
		defer stopEmailDispatcher()
		stopDueNotifications := notificationService.StartDueNotificationLoop(context.Background(), 15*time.Minute, func(processed int, err error) {
			if err != nil {
				log.Printf("Due notification dispatch failed: %v", err)
			} else if processed > 0 {
				log.Printf("Due notification dispatch processed %d candidates", processed)
			}
		})
		defer stopDueNotifications()
		if attachmentService != nil {
			stopAttachmentCleanup := attachmentService.StartOrphanCleanupLoop(context.Background(), time.Hour, time.Hour, func(deleted int, err error) {
				if err != nil {
					log.Printf("Attachment orphan cleanup failed: %v", err)
				} else if deleted > 0 {
					log.Printf("Attachment orphan cleanup removed %d objects", deleted)
				}
			})
			defer stopAttachmentCleanup()
		}
	}

	server := &ApiServer{
		db:                  db,
		redisClient:         redisClient,
		metrics:             metrics,
		metricsToken:        metricsToken,
		userHandler:         userHandler,
		accountHandler:      accountHandler,
		activityHandler:     activityHandler,
		attachmentHandler:   attachmentHandler,
		instanceHandler:     instanceHandler,
		projectHandler:      projectHandler,
		labelHandler:        labelHandler,
		ticketHandler:       ticketHandler,
		commentHandler:      commentHandler,
		notificationHandler: notificationHandler,
		githubHandler:       githubHandler,
		apHandler:           apHandler,
		c2sHandler:          c2sHandler,
		inboxHandler:        inboxHandler,
		federationHandler:   federationHandler,
		moderationHandler:   moderationHandler,
		deliveryHandler:     deliveryHandler,
		auditHandler:        auditHandler,
		deliverySvc:         deliveryService,
		wfHandler:           wfHandler,
	}

	if !role.runsAPI() {
		log.Printf("Backend process running as %s; HTTP API disabled", role)
		waitForShutdownSignal()
		return
	}

	e := echo.New()
	e.JSONSerializer = apiresponse.Serializer{}
	e.HTTPErrorHandler = apiresponse.HTTPErrorHandler
	if err := configureIPExtractor(e, cfg.Server.TrustedProxyCIDRs); err != nil {
		log.Fatal(err)
	}

	registerGlobalMiddlewareWithBodyLimit(e, os.Stdout, cfg.Server.RequestBodyLimit, server.metrics)

	corsOrigins := cfg.Server.CORSAllowedOrigins
	if err := validateCORSConfig(production, corsOrigins); err != nil {
		log.Fatal(err)
	}
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:5173"}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     corsOrigins,
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	e.GET("/health", server.healthCheck)
	e.GET("/ready", server.readinessCheck)
	e.GET("/metrics", server.metricsHandler)
	server.instanceHandler.RegisterPublicRoutes(e)

	server.userHandler.RegisterRoutes(
		e,
		newConfiguredRateLimiter("auth-ip", cfg.RateLimits.Auth, redisClient),
		newConfiguredRateLimiterWithIdentifier("auth-account", cfg.RateLimits.Auth, redisClient, authAccountRateLimitIdentifier),
	)
	if server.accountHandler != nil {
		server.accountHandler.RegisterPublicRoutes(e, newConfiguredRateLimiter("account-recovery", cfg.RateLimits.Auth, redisClient))
	}
	server.wfHandler.RegisterRoutes(e, newConfiguredRateLimiter("discovery", cfg.RateLimits.Discovery, redisClient))
	server.githubHandler.RegisterWebhookRoutes(e, newConfiguredRateLimiter("github-webhook", cfg.RateLimits.Auth, redisClient))

	// Local ActivityPub JSON-LD read routes and signed remote inbox POST foundation.
	server.apHandler.RegisterRoutes(e)
	server.c2sHandler.RegisterRoutes(e, authMiddleware.JWTMiddleware([]byte(jwtSecret), userService))
	server.inboxHandler.RegisterRoutes(e, newConfiguredRateLimiter("inbox", cfg.RateLimits.Inbox, redisClient))

	authenticatedAPI := e.Group("/api/v1")
	authenticatedAPI.Use(authMiddleware.TrustedOriginMiddleware(append(corsOrigins, publicBaseURL)))
	registerAuthenticatedAPIRoutes(authenticatedAPI, server, []byte(jwtSecret), userService)

	if err := runHTTPServer(e, cfg.Server.HTTPAddr, gracefulShutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

// waitForShutdownSignal blocks worker-only processes until the container stops them.
func waitForShutdownSignal() {
	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	signal := <-shutdownSignals
	log.Printf("Shutdown signal received: %s", signal)
}

// runHTTPServer starts Echo and stops it gracefully on SIGINT or SIGTERM.
func runHTTPServer(e *echo.Echo, addr string, timeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		server := &http.Server{
			Addr:              addr,
			Handler:           e,
			ReadHeaderTimeout: apiReadHeaderTimeout,
			ReadTimeout:       apiReadTimeout,
			WriteTimeout:      apiWriteTimeout,
			IdleTimeout:       apiIdleTimeout,
		}
		if err := e.StartServer(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	select {
	case signal := <-shutdownSignals:
		log.Printf("Shutdown signal received: %s", signal)
	case err := <-serverErrors:
		if err != nil {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP server shutdown failed: %w", err)
	}
	log.Println("HTTP server stopped gracefully")
	return nil
}

// startMetricsServer starts an optional standalone Prometheus endpoint.
func startMetricsServer(metrics *observability.Metrics, addr string, token string) *http.Server {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("/metrics", metricsHTTPHandler(metrics, token))
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTimeout,
	}
	go func() {
		log.Printf("Prometheus metrics server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Prometheus metrics server failed: %v", err)
		}
	}()
	return server
}

// shutdownMetricsServer stops the optional standalone metrics server.
func shutdownMetricsServer(server *http.Server, timeout time.Duration) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Prometheus metrics server shutdown failed: %v", err)
	}
}

// metricsHTTPHandler wraps the Prometheus handler with optional bearer-token auth.
func metricsHTTPHandler(metrics *observability.Metrics, token string) http.Handler {
	handler := metrics.Handler()
	if strings.TrimSpace(token) == "" {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearerToken(token, r.Header.Get(echo.HeaderAuthorization)) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "metrics authorization required", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// validBearerToken compares a metrics bearer token without leaking timing information.
func validBearerToken(expected, header string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	expectedHash := sha256.Sum256([]byte(expected))
	actualHash := sha256.Sum256([]byte(actual))
	return subtle.ConstantTimeCompare(expectedHash[:], actualHash[:]) == 1
}

// registerGlobalMiddleware adds request logging, request IDs, panic recovery, and optional metrics.
func registerGlobalMiddleware(e *echo.Echo, logOutput io.Writer, metrics ...*observability.Metrics) {
	registerGlobalMiddlewareWithBodyLimit(e, logOutput, defaultRequestBodyLimit, metrics...)
}

// registerGlobalMiddlewareWithBodyLimit adds global middleware with a configurable body limit.
func registerGlobalMiddlewareWithBodyLimit(e *echo.Echo, logOutput io.Writer, bodyLimit string, metrics ...*observability.Metrics) {
	e.Use(middleware.RequestID())
	e.Use(middleware.LoggerWithConfig(requestLoggerConfig(logOutput)))
	e.Use(middleware.Recover())
	if len(metrics) > 0 && metrics[0] != nil {
		e.Use(metricsMiddleware(metrics[0]))
	}
	if strings.TrimSpace(bodyLimit) == "" {
		bodyLimit = defaultRequestBodyLimit
	}
	e.Use(middleware.BodyLimit(bodyLimit))
}

// metricsMiddleware records completed HTTP requests in Prometheus collectors.
func metricsMiddleware(metrics *observability.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			startedAt := time.Now()
			metrics.IncHTTPInFlight()
			defer metrics.DecHTTPInFlight()

			err := next(c)
			route := c.Path()
			if route == "" {
				route = c.Request().URL.Path
			}
			metrics.ObserveHTTPRequest(c.Request().Method, route, responseStatus(c, err), time.Since(startedAt))
			return err
		}
	}
}

// responseStatus returns the status that Echo will write for a handler result.
func responseStatus(c echo.Context, err error) int {
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr.Code
		}
		if c.Response().Status > 0 {
			return c.Response().Status
		}
		return http.StatusInternalServerError
	}
	if c.Response().Status > 0 {
		return c.Response().Status
	}
	return http.StatusOK
}

// requestLoggerConfig builds the structured JSON request logger configuration.
func requestLoggerConfig(output io.Writer) middleware.LoggerConfig {
	if output == nil {
		output = os.Stdout
	}
	return middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","request_id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","route":"${route}",` +
			`"status":${status},"latency":${latency},"latency_human":"${latency_human}",` +
			`"bytes_in":${bytes_in},"bytes_out":${bytes_out},"error":"${error}"}` + "\n",
		Output: output,
	}
}

// newRateLimiter returns an in-memory per-IP rate limiter for one public surface.
func newRateLimiter(requestsPerSecond rate.Limit, burst int) echo.MiddlewareFunc {
	return newRateLimiterWithStore(newMemoryRateLimiterStore(requestsPerSecond, burst))
}

// newConfiguredRateLimiter returns a Redis-backed limiter when Redis is configured.
func newConfiguredRateLimiter(scope string, cfg appconfig.RateLimitConfig, redisClient *redis.Client) echo.MiddlewareFunc {
	return newConfiguredRateLimiterWithIdentifier(scope, cfg, redisClient, rateLimitIdentifier)
}

// newConfiguredRateLimiterWithIdentifier creates a limiter with a custom identity policy.
func newConfiguredRateLimiterWithIdentifier(scope string, cfg appconfig.RateLimitConfig, redisClient *redis.Client, extractor middleware.Extractor) echo.MiddlewareFunc {
	var store middleware.RateLimiterStore
	if redisClient != nil {
		store = appratelimit.NewRedisStore(redisClient, appratelimit.RedisStoreConfig{
			Prefix:            "progo:ratelimit:" + strings.TrimSpace(scope),
			RequestsPerSecond: cfg.RequestsPerSecond,
			Burst:             cfg.Burst,
			ExpiresIn:         5 * time.Minute,
		})
	} else {
		store = newMemoryRateLimiterStore(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)
	}
	return newRateLimiterWithStoreAndIdentifier(store, extractor)
}

// newRateLimiterWithStore wraps a concrete limiter store with the shared identifier policy.
func newRateLimiterWithStore(store middleware.RateLimiterStore) echo.MiddlewareFunc {
	return newRateLimiterWithStoreAndIdentifier(store, rateLimitIdentifier)
}

// newRateLimiterWithStoreAndIdentifier binds a limiter store to an identifier extractor.
func newRateLimiterWithStoreAndIdentifier(store middleware.RateLimiterStore, extractor middleware.Extractor) echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store:               store,
		IdentifierExtractor: extractor,
	})
}

// newMemoryRateLimiterStore returns the local-process fallback limiter store.
func newMemoryRateLimiterStore(requestsPerSecond rate.Limit, burst int) middleware.RateLimiterStore {
	return middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      requestsPerSecond,
		Burst:     burst,
		ExpiresIn: 5 * time.Minute,
	})
}

// rateLimitIdentifier groups unauthenticated requests by Echo's resolved client IP.
func rateLimitIdentifier(c echo.Context) (string, error) {
	return c.RealIP(), nil
}

// authAccountRateLimitIdentifier applies a second, account-scoped throttle without storing email addresses.
func authAccountRateLimitIdentifier(c echo.Context) (string, error) {
	if c.Request().Method != http.MethodPost || (c.Path() != "/login" && c.Path() != "/register") {
		return "request:" + c.Path() + ":" + c.RealIP(), nil
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return "request:" + c.Path() + ":" + c.RealIP(), nil
	}
	c.Request().Body = io.NopCloser(strings.NewReader(string(body)))
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.Email) == "" {
		return "request:" + c.Path() + ":" + c.RealIP(), nil
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(payload.Email))))
	return fmt.Sprintf("account:%x", sum[:]), nil
}

// registerAuthenticatedAPIRoutes mounts stable REST API routes under one prefix.
func registerAuthenticatedAPIRoutes(api *echo.Group, server *ApiServer, jwtSecret []byte, userService *user.Service) {
	api.Use(authMiddleware.JWTMiddleware(jwtSecret, userService))
	api.Use(authMiddleware.MFAEnrollmentMiddleware(userService))

	api.GET("/me", server.getProfile)
	server.instanceHandler.RegisterAuthenticatedRoutes(api)
	server.userHandler.RegisterAccountRoutes(api)
	if server.accountHandler != nil {
		server.accountHandler.RegisterAccountRoutes(api)
	}
	server.userHandler.RegisterAdminRoutes(api)
	server.federationHandler.RegisterRoutes(api)
	server.projectHandler.RegisterRoutes(api)
	if server.activityHandler != nil {
		server.activityHandler.RegisterRoutes(api)
	}
	server.labelHandler.RegisterRoutes(api)
	server.ticketHandler.RegisterRoutes(api)
	if server.attachmentHandler != nil {
		server.attachmentHandler.RegisterRoutes(api)
	}
	server.commentHandler.RegisterRoutes(api)
	server.notificationHandler.RegisterRoutes(api)
	server.githubHandler.RegisterRoutes(api)
	server.deliveryHandler.RegisterRoutes(api)
	server.moderationHandler.RegisterRoutes(api)
	server.auditHandler.RegisterRoutes(api)
}

// healthCheck is a cheap liveness probe. Dependency health belongs to /ready.
func (s *ApiServer) healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"system": "alive",
	})
}

// readinessCheck reports dependency readiness for load balancers and containers.
func (s *ApiServer) readinessCheck(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()

	statusCode := http.StatusOK
	status := "ready"
	checks := map[string]string{}
	var missingTables []string

	if err := s.db.PingContext(ctx); err != nil {
		log.Printf("Readiness check failed: database ping error: %v", err)
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
		checks["database"] = "error"
	} else {
		checks["database"] = "ok"
		var err error
		missingTables, err = missingRequiredDatabaseTables(ctx, s.db)
		if err != nil {
			log.Printf("Readiness check failed: database schema check error: %v", err)
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
			checks["database_schema"] = "error"
		} else if len(missingTables) > 0 {
			log.Printf("Readiness check failed: missing database tables: %s", strings.Join(missingTables, ","))
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
			checks["database_schema"] = "missing"
		} else {
			checks["database_schema"] = "ok"
		}
	}

	if s.redisClient == nil {
		checks["redis"] = "disabled"
	} else {
		if err := s.redisClient.Ping(ctx).Err(); err != nil {
			log.Printf("Readiness check failed: redis ping error: %v", err)
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
			checks["redis"] = "error"
		} else {
			checks["redis"] = "ok"
		}
	}

	payload := map[string]any{
		"status": status,
		"checks": checks,
	}
	if checks["database_schema"] == "missing" {
		payload["missing_tables"] = missingTables
	}
	return c.JSON(statusCode, payload)
}

// missingRequiredDatabaseTables returns required tables absent from the active schema.
func missingRequiredDatabaseTables(ctx context.Context, db *sqlx.DB) ([]string, error) {
	missing := make([]string, 0)
	for _, table := range requiredDatabaseTables {
		var exists bool
		if err := db.GetContext(ctx, &exists, `SELECT to_regclass($1) IS NOT NULL`, table); err != nil {
			return nil, err
		}
		if !exists {
			missing = append(missing, table)
		}
	}
	return missing, nil
}

// metricsHandler exposes the Prometheus scrape endpoint.
func (s *ApiServer) metricsHandler(c echo.Context) error {
	if s.metrics == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "metrics unavailable"})
	}
	return echo.WrapHandler(metricsHTTPHandler(s.metrics, s.metricsToken))(c)
}

// getProfile returns the authenticated user's basic session profile.
func (s *ApiServer) getProfile(c echo.Context) error {
	userID := c.Get("userID").(string)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Welcome!",
		"user_id": userID,
	})
}
