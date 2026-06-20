package main

import (
	"log"

	"github.com/gofiber/fiber/v3"
)

func main() {
    // Fiber instance
    app := fiber.New()

    // Routes
    app.Get("/", hello)

    // Start server
    log.Fatal(app.Listen(":3000"))
}

// Handler
func hello(c fiber.Ctx) error {
    return c.SendString("Hello, World 👋!")
}
