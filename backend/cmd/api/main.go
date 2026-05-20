package main

import (
	"log"
	"net/http"
	"os"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	authMiddleware "github.com/antonovs105/project-management-system-go/internal/middleware"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
)

// Server structure
type ApiServer struct {
	db             *sqlx.DB
	userHandler    *user.Handler
	projectHandler *project.Handler
	ticketHandler  *ticket.Handler
	commentHandler *comment.Handler
	apHandler      *activitypub.Handler
}

func main() {
	// Load config
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		log.Fatal("DB_SOURCE environment variable is not set")
	}
	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET_KEY environment variable is not set")
	}
	apConfig := activitypub.NewConfig(os.Getenv("PUBLIC_BASE_URL"), os.Getenv("LOCAL_DOMAIN"))

	// Connecting DB
	db, err := sqlx.Connect("postgres", dbSource)
	if err != nil {
		log.Fatalf("Can't connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("DB connection successful")

	// User dependencies
	userRepo := user.NewRepository(db, apConfig)
	userService := user.NewService(userRepo, []byte(jwtSecret), apConfig)
	userHandler := user.NewHandler(userService)

	// project dependencies
	projectRepo := project.NewRepository(db, apConfig)
	projectService := project.NewService(projectRepo, apConfig)
	projectHandler := project.NewHandler(projectService)

	// Ticket dependencies
	ticketRepo := ticket.NewRepository(db, apConfig)
	ticketService := ticket.NewService(ticketRepo, projectService, apConfig)
	ticketHandler := ticket.NewHandler(ticketService)

	// Comment dependencies
	commentRepo := comment.NewRepository(db, apConfig)
	commentService := comment.NewService(commentRepo, ticketService, apConfig)
	commentHandler := comment.NewHandler(commentService)

	// ActivityPub JSON-LD read dependencies
	apHandler := activitypub.NewHandler(db, apConfig)

	// Dependency injection
	server := &ApiServer{
		db:             db,
		userHandler:    userHandler,
		projectHandler: projectHandler,
		ticketHandler:  ticketHandler,
		commentHandler: commentHandler,
		apHandler:      apHandler,
	}

	// New Echo
	e := echo.New()

	//Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// CORS
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:5173"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	// Routes
	e.GET("/health", server.healthCheck)

	server.userHandler.RegisterRoutes(e)

	// Local ActivityPub JSON-LD read routes. Remote inbox POST/S2S is a later milestone.
	server.apHandler.RegisterRoutes(e)

	// protected routes
	api := e.Group("/api")

	api.Use(authMiddleware.JWTMiddleware([]byte(jwtSecret)))

	// routes that require auth
	api.GET("/me", server.getProfile) // for test
	server.projectHandler.RegisterRoutes(api)
	server.ticketHandler.RegisterRoutes(api)
	server.commentHandler.RegisterRoutes(api)

	e.Logger.Fatal(e.Start(":8080"))
}

// Handler
func (s *ApiServer) healthCheck(c echo.Context) error {
	// Check DB
	if err := s.db.Ping(); err != nil {
		log.Printf("Health check failed: database ping error: %v", err)

		// Returns error status if DB is unreacheble
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error",
			"system": "database unreacheble",
		})
	}
	// Returns JSON
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"system": "working",
	})
}

func (s *ApiServer) getProfile(c echo.Context) error {
	// taking userID
	userID := c.Get("userID").(string)

	// Return ID
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Welcome!",
		"user_id": userID,
	})
}
