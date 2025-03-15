package main

import (
	"ignite-api/internal/api"
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	// Define API endpoints
	app.Get("/health", api.HealthHandler)
	app.Post("/master/provision", api.ProvisionHandler("master"))
	app.Post("/worker/provision", api.ProvisionHandler("worker"))
	app.Delete("/vm/:name", api.DeleteVMHandler)

	// Start server
	port := "5090"
	log.Printf("Starting Ignite API server on port %s...\n", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v\n", err)
	}
}
