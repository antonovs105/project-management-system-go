package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	//Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/health", healthCheck)

	e.Logger.Fatal(e.Start(":8080"))
}

// Handler
func healthCheck(c echo.Context) error {
	// Возвращаем JSON-ответ
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
		"system": "odin",
	})
}
