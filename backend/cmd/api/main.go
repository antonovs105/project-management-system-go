package main

import (
	"log"
	"net/http"
	"os"

	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
)

// Server structure
type ApiServer struct {
	db *sqlx.DB
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

	// Connecting DB
	db, err := sqlx.Connect("postgres", dbSource)
	if err != nil {
		log.Fatalf("Can't connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("DB connection successful")

	_ = user.NewRepository(db)

	// Dependency injection
	server := &ApiServer{
		db: db,
	}

	// New Echo
	e := echo.New()

	//Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/health", server.healthCheck)

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
