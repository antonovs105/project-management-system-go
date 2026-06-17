// Package main boots the HTTP API server and wires backend vertical slices together.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
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

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/c2s"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	apfederation "github.com/antonovs105/project-management-system-go/internal/activitypub/federation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	apmoderation "github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/adminaudit"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/githubintegration"
	authMiddleware "github.com/antonovs105/project-management-system-go/internal/middleware"
	"github.com/antonovs105/project-management-system-go/internal/notification"
	"github.com/antonovs105/project-management-system-go/internal/observability"
	"github.com/antonovs105/project-management-system-go/internal/project"
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
	defaultRequestBodyLimit = "2M"
	// defaultRequestBodyLimitBytes mirrors defaultRequestBodyLimit for tests and comments.
	defaultRequestBodyLimitBytes = 2 << 20
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
	"projects",
	"project_members",
	"project_roles",
	"project_role_permissions",
	"actor_follows",
	"tickets",
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
}

// appRole selects which server responsibilities this process owns.
type appRole string

// ApiServer wires database-backed services into the HTTP server.
type ApiServer struct {
	db                  *sqlx.DB
	redisAddr           string
	metrics             *observability.Metrics
	metricsToken        string
	userHandler         *user.Handler
	projectHandler      *project.Handler
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
	appEnv, err := parseAppEnv(os.Getenv("APP_ENV"))
	if err != nil {
		log.Fatal(err)
	}
	production := appEnv == appEnvProduction
	role, err := parseAppRole(os.Getenv("APP_ROLE"))
	if err != nil {
		log.Fatal(err)
	}
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		log.Fatal("DB_SOURCE environment variable is not set")
	}
	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET_KEY environment variable is not set")
	}
	publicBaseURL := os.Getenv("PUBLIC_BASE_URL")
	if publicBaseURL == "" {
		log.Fatal("PUBLIC_BASE_URL environment variable is not set")
	}
	localDomain := os.Getenv("LOCAL_DOMAIN")
	if localDomain == "" {
		log.Fatal("LOCAL_DOMAIN environment variable is not set")
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	metricsAddr := strings.TrimSpace(os.Getenv("METRICS_ADDR"))
	metricsToken := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	actorPrivateKeyEncryptionKey := strings.TrimSpace(os.Getenv("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY"))
	allowInsecureFederationHTTP := !production
	if value, ok, err := optionalBoolEnv("FEDERATION_ALLOW_INSECURE_HTTP"); err != nil {
		log.Fatal(err)
	} else if ok {
		allowInsecureFederationHTTP = value
	}
	if production && allowInsecureFederationHTTP {
		log.Fatal("FEDERATION_ALLOW_INSECURE_HTTP cannot be enabled in production")
	}
	allowPrivateFederationNetworks := false
	if value, ok, err := optionalBoolEnv("FEDERATION_ALLOW_PRIVATE_NETWORKS"); err != nil {
		log.Fatal(err)
	} else if ok {
		allowPrivateFederationNetworks = value
	}
	if production && allowPrivateFederationNetworks {
		log.Fatal("FEDERATION_ALLOW_PRIVATE_NETWORKS cannot be enabled in production")
	}
	requireHTTPSFederation := !allowInsecureFederationHTTP
	if err := validateRuntimeConfig(production, jwtSecret, publicBaseURL, localDomain, metricsToken, actorPrivateKeyEncryptionKey); err != nil {
		log.Fatal(err)
	}
	privateKeyCodec, err := secrets.NewPrivateKeyCodec(actorPrivateKeyEncryptionKey)
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

	log.Println("DB connection successful")

	userRepo := user.NewRepository(db, apConfig, privateKeyCodec)
	userService := user.NewService(userRepo, []byte(jwtSecret), apConfig, user.WithOAuthConfig(user.OAuthConfig{
		FrontendCallbackURL: strings.TrimSpace(os.Getenv("OAUTH_FRONTEND_CALLBACK_URL")),
		Google: user.OAuthProviderConfig{
			ClientID:     strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_REDIRECT_URL")),
		},
		GitHub: user.OAuthProviderConfig{
			ClientID:     strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("GITHUB_OAUTH_REDIRECT_URL")),
		},
	}))
	userHandler := user.NewHandler(userService)

	projectRepo := project.NewRepository(db, apConfig, privateKeyCodec)
	projectService := project.NewService(projectRepo, apConfig)
	projectHandler := project.NewHandler(projectService)

	ticketRepo := ticket.NewRepository(db, apConfig)
	ticketService := ticket.NewService(ticketRepo, projectService, apConfig)
	ticketEvents := ticket.NewEventHub()
	ticketService.SetEventPublisher(ticketEvents)
	ticketHandler := ticket.NewHandler(ticketService, ticket.WithEventSubscriber(ticketEvents))

	notificationEvents := notification.NewEventHub()
	notificationService := notification.NewService(
		notification.NewRepository(db),
		notification.WithEventPublisher(notificationEvents),
	)
	ticketService.SetNotificationSink(notificationService)
	notificationHandler := notification.NewHandler(notificationService, notification.WithEventSubscriber(notificationEvents))

	commentRepo := comment.NewRepository(db, apConfig)
	commentService := comment.NewService(commentRepo, ticketService, apConfig)
	commentHandler := comment.NewHandler(commentService)

	githubClient := githubintegration.NewHTTPClient(githubintegration.WithToken(os.Getenv("GITHUB_API_TOKEN")))
	githubService := githubintegration.NewService(githubintegration.NewRepository(db), projectService, githubClient)
	githubHandler := githubintegration.NewHandler(
		githubService,
		githubintegration.WithWebhookSecret(os.Getenv("GITHUB_WEBHOOK_SECRET")),
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
		remoteinbox.WithBlockedDomains(splitCSVEnv("FEDERATION_BLOCKED_DOMAINS")),
	)
	inboxHandler := remoteinbox.NewHandler(inboxService, apConfig)
	federationHandler := apfederation.NewHandler(apfederation.NewService(
		apfederation.NewRepository(db),
		apfederation.WithConfig(apConfig),
		apfederation.WithRemoteActorResolver(remoteActorService),
		apfederation.WithDelivery(deliveryService),
	))
	moderationHandler := apmoderation.NewHandler(apmoderation.NewService(apmoderation.NewRepository(db), deliveryQueue))
	auditHandler := adminaudit.NewHandler(adminaudit.NewService(adminaudit.NewRepository(db)))

	// WebFinger discovery dependencies
	wfRepo := webfinger.NewRepository(db)
	wfService := webfinger.NewService(wfRepo, apConfig)
	wfHandler := webfinger.NewHandler(wfService)

	server := &ApiServer{
		db:                  db,
		redisAddr:           redisAddr,
		metrics:             metrics,
		metricsToken:        metricsToken,
		userHandler:         userHandler,
		projectHandler:      projectHandler,
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
	if err := configureIPExtractor(e, splitCSVEnv("TRUSTED_PROXY_CIDRS")); err != nil {
		log.Fatal(err)
	}

	registerGlobalMiddleware(e, os.Stdout, server.metrics)

	corsOrigins := splitCSVEnv("CORS_ALLOWED_ORIGINS")
	if err := validateCORSConfig(production, corsOrigins); err != nil {
		log.Fatal(err)
	}
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:5173"}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsOrigins,
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	e.GET("/health", server.healthCheck)
	e.GET("/ready", server.readinessCheck)
	e.GET("/metrics", server.metricsHandler)

	server.userHandler.RegisterRoutes(e, newRateLimiter(authRateLimitPerSecond, authRateLimitBurst))
	server.wfHandler.RegisterRoutes(e, newRateLimiter(discoveryRateLimitPerSecond, discoveryRateLimitBurst))
	server.githubHandler.RegisterWebhookRoutes(e, newRateLimiter(authRateLimitPerSecond, authRateLimitBurst))

	// Local ActivityPub JSON-LD read routes and signed remote inbox POST foundation.
	server.apHandler.RegisterRoutes(e)
	server.c2sHandler.RegisterRoutes(e, authMiddleware.JWTMiddleware([]byte(jwtSecret), userService))
	server.inboxHandler.RegisterRoutes(e, newRateLimiter(inboxRateLimitPerSecond, inboxRateLimitBurst))

	registerAuthenticatedAPIRoutes(e.Group("/api"), server, []byte(jwtSecret), userService)
	registerAuthenticatedAPIRoutes(e.Group("/api/v1"), server, []byte(jwtSecret), userService)

	if err := runHTTPServer(e, defaultHTTPAddr, gracefulShutdownTimeout); err != nil {
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
	e.Use(middleware.RequestID())
	e.Use(middleware.LoggerWithConfig(requestLoggerConfig(logOutput)))
	e.Use(middleware.Recover())
	if len(metrics) > 0 && metrics[0] != nil {
		e.Use(metricsMiddleware(metrics[0]))
	}
	e.Use(middleware.BodyLimit(defaultRequestBodyLimit))
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
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      requestsPerSecond,
			Burst:     burst,
			ExpiresIn: 5 * time.Minute,
		}),
		IdentifierExtractor: rateLimitIdentifier,
	})
}

// rateLimitIdentifier groups unauthenticated requests by Echo's resolved client IP.
func rateLimitIdentifier(c echo.Context) (string, error) {
	return c.RealIP(), nil
}

// registerAuthenticatedAPIRoutes mounts stable REST API routes under one prefix.
func registerAuthenticatedAPIRoutes(api *echo.Group, server *ApiServer, jwtSecret []byte, userService *user.Service) {
	api.Use(authMiddleware.JWTMiddleware(jwtSecret, userService))

	api.GET("/me", server.getProfile)
	server.userHandler.RegisterAccountRoutes(api)
	server.userHandler.RegisterAdminRoutes(api)
	server.federationHandler.RegisterRoutes(api)
	server.projectHandler.RegisterRoutes(api)
	server.ticketHandler.RegisterRoutes(api)
	server.commentHandler.RegisterRoutes(api)
	server.notificationHandler.RegisterRoutes(api)
	server.githubHandler.RegisterRoutes(api)
	server.deliveryHandler.RegisterRoutes(api)
	server.moderationHandler.RegisterRoutes(api)
	server.auditHandler.RegisterRoutes(api)
}

// healthCheck reports whether the API process can still reach PostgreSQL.
func (s *ApiServer) healthCheck(c echo.Context) error {
	if err := s.db.Ping(); err != nil {
		log.Printf("Health check failed: database ping error: %v", err)

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error",
			"system": "database unreachable",
		})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"system": "working",
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

	if strings.TrimSpace(s.redisAddr) == "" {
		checks["redis"] = "disabled"
	} else {
		client := redis.NewClient(&redis.Options{Addr: s.redisAddr})
		defer client.Close()
		if err := client.Ping(ctx).Err(); err != nil {
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
