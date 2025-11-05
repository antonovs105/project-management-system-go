package main

import (
	"log"
	"net/http"
	"os"

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

	// Connecting DB
	db, err := sqlx.Connect("postgres", dbSource)
	if err != nil {
		log.Fatalf("Can't connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("DB connection successful")

	// User dependencies
	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, []byte(jwtSecret))
	userHandler := user.NewHandler(userService)

	// Project dependencies
	projectRepo := project.NewRepository(db)
	projectService := project.NewService(projectRepo)
	projectHandler := project.NewHandler(projectService)

	// Ticket dependencies
	ticketRepo := ticket.NewRepository(db)
	ticketService := ticket.NewService(ticketRepo, projectService)
	ticketHandler := ticket.NewHandler(ticketService)

	// Dependency injection
	server := &ApiServer{
		db:             db,
		userHandler:    userHandler,
		projectHandler: projectHandler,
		ticketHandler:  ticketHandler,
	}

	// New Echo
	e := echo.New()

	//Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/health", server.healthCheck)

	e.POST("/register", server.userHandler.Register)

	e.POST("/login", server.userHandler.Login)

	// protected routes
	api := e.Group("/api")

	api.Use(authMiddleware.JWTMiddleware([]byte(jwtSecret)))

	// routes that require auth
	api.GET("/me", server.getProfile) // for test
	api.POST("/projects", server.projectHandler.Create)
	api.GET("/projects/:id", server.projectHandler.Get)
	api.GET("/projects", server.projectHandler.List)
	api.PATCH("/projects/:id", server.projectHandler.Update)
	api.DELETE("/projects/:id", server.projectHandler.Delete)
	api.POST("/projects/:projectID/tickets", server.ticketHandler.Create)
	api.GET("/projects/:projectID/tickets", server.ticketHandler.List)
	api.GET("/tickets/:id", server.ticketHandler.Get)
	api.PATCH("/tickets/:id", server.ticketHandler.Update)
	api.DELETE("/tickets/:id", server.ticketHandler.Delete)

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
	userID := c.Get("userID").(int64)

	// Return ID
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Welcome!",
		"user_id": userID,
	})
}
