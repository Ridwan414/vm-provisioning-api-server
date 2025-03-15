package main

import (
	"ignite-api/internal/api"
	"ignite-api/internal/logger"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

func main() {
	app := fiber.New()

	// Add request ID middleware
	app.Use(requestid.New())

	// Add request logging middleware
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		logger.RequestLog(c.Method(), c.Path(), c.IP(), duration)
		return err
	})

	// Define API endpoints
	app.Get("/health", api.HealthHandler)
	app.Post("/master/provision", api.ProvisionHandler("master"))
	app.Post("/worker/provision", api.ProvisionHandler("worker"))
	app.Delete("/vm/:name", api.DeleteVMHandler)

	// Start server
	port := "5090"
	logger.Info("Starting Ignite API server on port %s...", port)
	if err := app.Listen(":" + port); err != nil {
		logger.Fatal("Failed to start server: %v", err)
	}
}
