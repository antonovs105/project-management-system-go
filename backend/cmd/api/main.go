// Package main boots the HTTP API server and wires backend vertical slices together.
package main

import (
	"context"
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
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	apmoderation "github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/adminaudit"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	authMiddleware "github.com/antonovs105/project-management-system-go/internal/middleware"
	"github.com/antonovs105/project-management-system-go/internal/project"
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
)

const (
	// defaultHTTPAddr is the in-container API listen address.
	defaultHTTPAddr = ":8080"
	// gracefulShutdownTimeout bounds HTTP and worker shutdown on container stop.
	gracefulShutdownTimeout = 10 * time.Second
)

// ApiServer wires database-backed services into the HTTP server.
type ApiServer struct {
	db                *sqlx.DB
	redisAddr         string
	userHandler       *user.Handler
	projectHandler    *project.Handler
	ticketHandler     *ticket.Handler
	commentHandler    *comment.Handler
	apHandler         *activitypub.Handler
	c2sHandler        *c2s.Handler
	inboxHandler      *remoteinbox.Handler
	moderationHandler *apmoderation.Handler
	deliveryHandler   *delivery.Handler
	auditHandler      *adminaudit.Handler
	deliverySvc       *delivery.Service
	wfHandler         *webfinger.Handler
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

// normalizedEnv returns a lowercase environment value or fallback when the value is empty.
func normalizedEnv(name, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value
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

// validateRuntimeConfig rejects unsafe deployment configuration before the server starts.
func validateRuntimeConfig(production bool, jwtSecret, publicBaseURL, localDomain, adminBootstrapToken string) error {
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
	if adminBootstrapToken != "" && len(adminBootstrapToken) < 32 {
		return fmt.Errorf("ADMIN_BOOTSTRAP_TOKEN must be at least 32 characters in production")
	}
	return nil
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
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	appEnv := normalizedEnv("APP_ENV", "development")
	production := appEnv == "production"
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
	adminBootstrapToken := strings.TrimSpace(os.Getenv("ADMIN_BOOTSTRAP_TOKEN"))
	if err := validateRuntimeConfig(production, jwtSecret, publicBaseURL, localDomain, adminBootstrapToken); err != nil {
		log.Fatal(err)
	}
	apConfig := activitypub.NewConfig(publicBaseURL, localDomain)

	db, err := sqlx.Connect("postgres", dbSource)
	if err != nil {
		log.Fatalf("Can't connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("DB connection successful")

	userRepo := user.NewRepository(db, apConfig)
	userService := user.NewService(userRepo, []byte(jwtSecret), apConfig)
	userHandler := user.NewHandler(userService, adminBootstrapToken)

	projectRepo := project.NewRepository(db, apConfig)
	projectService := project.NewService(projectRepo, apConfig)
	projectHandler := project.NewHandler(projectService)

	ticketRepo := ticket.NewRepository(db, apConfig)
	ticketService := ticket.NewService(ticketRepo, projectService, apConfig)
	ticketHandler := ticket.NewHandler(ticketService)

	commentRepo := comment.NewRepository(db, apConfig)
	commentService := comment.NewService(commentRepo, ticketService, apConfig)
	commentHandler := comment.NewHandler(commentService)

	c2sHandler := c2s.NewHandler(db, apConfig, ticketService, commentService)

	// Remote ActivityPub signing/discovery dependencies
	remoteActorRepo := remoteactor.NewRepository(db)
	remoteActorService := remoteactor.NewService(remoteActorRepo)
	sigRepo := httpsig.NewRepository(db)
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

	// ActivityPub delivery dependencies. The worker runs in-process for now; the slice can be
	// moved to a separate worker container later without changing delivery internals.
	deliveryRepo := delivery.NewRecipientRepository(db)
	var deliveryQueue delivery.Queue = delivery.NoopQueue{}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		redisOpt := asynq.RedisClientOpt{Addr: redisAddr}
		deliveryQueue = delivery.NewAsynqQueue(redisOpt)
		defer deliveryQueue.Close()

		deliveryWorker := delivery.NewWorker(
			deliveryRepo,
			sigService,
			nil,
			delivery.WithRemoteActorRefresher(remoteActorService),
		)
		deliveryServer := delivery.NewAsynqServer(redisOpt)
		if err := deliveryServer.Start(delivery.NewServeMux(deliveryWorker)); err != nil {
			log.Fatalf("Can't start ActivityPub delivery worker: %v", err)
		}
		defer deliveryServer.Shutdown()

		log.Printf("ActivityPub delivery worker started with Redis at %s", redisAddr)
	} else {
		if production {
			log.Fatal("REDIS_ADDR environment variable is required in production")
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
	moderationHandler := apmoderation.NewHandler(apmoderation.NewService(apmoderation.NewRepository(db), deliveryQueue))
	auditHandler := adminaudit.NewHandler(adminaudit.NewService(adminaudit.NewRepository(db)))

	// WebFinger discovery dependencies
	wfRepo := webfinger.NewRepository(db)
	wfService := webfinger.NewService(wfRepo, apConfig)
	wfHandler := webfinger.NewHandler(wfService)

	server := &ApiServer{
		db:                db,
		redisAddr:         redisAddr,
		userHandler:       userHandler,
		projectHandler:    projectHandler,
		ticketHandler:     ticketHandler,
		commentHandler:    commentHandler,
		apHandler:         apHandler,
		c2sHandler:        c2sHandler,
		inboxHandler:      inboxHandler,
		moderationHandler: moderationHandler,
		deliveryHandler:   deliveryHandler,
		auditHandler:      auditHandler,
		deliverySvc:       deliveryService,
		wfHandler:         wfHandler,
	}

	e := echo.New()

	registerGlobalMiddleware(e, os.Stdout)

	corsOrigins := splitCSVEnv("CORS_ALLOWED_ORIGINS")
	if len(corsOrigins) == 0 {
		if production {
			log.Fatal("CORS_ALLOWED_ORIGINS environment variable is required in production")
		}
		corsOrigins = []string{"http://localhost:5173"}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsOrigins,
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, user.AdminBootstrapTokenHeader},
	}))

	e.GET("/health", server.healthCheck)
	e.GET("/ready", server.readinessCheck)

	server.userHandler.RegisterRoutes(e)
	server.wfHandler.RegisterRoutes(e)

	// Local ActivityPub JSON-LD read routes and signed remote inbox POST foundation.
	server.apHandler.RegisterRoutes(e)
	server.c2sHandler.RegisterRoutes(e, authMiddleware.JWTMiddleware([]byte(jwtSecret), userService))
	server.inboxHandler.RegisterRoutes(e)

	api := e.Group("/api")

	api.Use(authMiddleware.JWTMiddleware([]byte(jwtSecret), userService))

	api.GET("/me", server.getProfile)
	server.userHandler.RegisterAccountRoutes(api)
	server.userHandler.RegisterAdminRoutes(api)
	server.projectHandler.RegisterRoutes(api)
	server.ticketHandler.RegisterRoutes(api)
	server.commentHandler.RegisterRoutes(api)
	server.deliveryHandler.RegisterRoutes(api)
	server.moderationHandler.RegisterRoutes(api)
	server.auditHandler.RegisterRoutes(api)

	if err := runHTTPServer(e, defaultHTTPAddr, gracefulShutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

// runHTTPServer starts Echo and stops it gracefully on SIGINT or SIGTERM.
func runHTTPServer(e *echo.Echo, addr string, timeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// registerGlobalMiddleware adds request logging, request IDs, and panic recovery.
func registerGlobalMiddleware(e *echo.Echo, logOutput io.Writer) {
	e.Use(middleware.RequestID())
	e.Use(middleware.LoggerWithConfig(requestLoggerConfig(logOutput)))
	e.Use(middleware.Recover())
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

	if err := s.db.PingContext(ctx); err != nil {
		log.Printf("Readiness check failed: database ping error: %v", err)
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
		checks["database"] = "error"
	} else {
		checks["database"] = "ok"
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

	return c.JSON(statusCode, map[string]any{
		"status": status,
		"checks": checks,
	})
}

// getProfile returns the authenticated user's basic session profile.
func (s *ApiServer) getProfile(c echo.Context) error {
	userID := c.Get("userID").(string)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Welcome!",
		"user_id": userID,
	})
}
