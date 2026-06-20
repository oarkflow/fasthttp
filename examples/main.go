package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	fh "github.com/orgware/fasthttp"
	"github.com/orgware/fasthttp/middleware"
)

func main() {
	app := fh.New(fh.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	// ── Lifecycle hooks ──────────────────────────────────────────────────
	app.OnListen(func() error {
		log.Println("🚀 Server is ready to accept connections")
		return nil
	})

	app.OnShutdown(func() error {
		log.Println("🛑 Server shutting down gracefully")
		return nil
	})

	app.OnConnect(func(_ net.Conn) {
		// track new connections if needed
	})

	app.OnError(func(err error) {
		log.Printf("Server error: %v", err)
	})

	// ── Global middleware ─────────────────────────────────────────────────
	app.Use(middleware.Recover())
	app.Use(middleware.RequestID())
	app.Use(middleware.Logger(middleware.LoggerConfig{
		Format: "[${ip}] ${method} ${path} → ${status} (${latency})\n",
	}))
	app.Use(middleware.CORS(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
		MaxAge:       3600,
	}))

	// ── Health check ──────────────────────────────────────────────────────
	app.Get("/health", func(ctx *fh.Ctx) error {
		return ctx.JSON(map[string]string{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// ── API v1 group ──────────────────────────────────────────────────────
	v1 := app.Group("/api/v1",
		middleware.RateLimiter(middleware.RateLimiterConfig{
			Max:    1000,
			Window: time.Minute,
		}),
	)

	// Users resource
	users := v1.Group("/users")

	users.Get("", listUsers)
	users.Get("/:id", getUser)
	users.Post("", createUser)
	users.Put("/:id", updateUser)
	users.Patch("/:id", patchUser)
	users.Delete("/:id", deleteUser)

	// Protected admin group
	admin := v1.Group("/admin",
		middleware.BasicAuth("admin", "secret"),
	)

	admin.Get("/stats", func(ctx *fh.Ctx) error {
		return ctx.JSON(map[string]any{
			"uptime":    time.Since(start).String(),
			"requestID": ctx.Locals("requestID"),
		})
	})

	// Nested params
	app.Get("/orgs/:org/repos/:repo/commits/:sha", func(ctx *fh.Ctx) error {
		return ctx.JSON(map[string]string{
			"org":  ctx.Param("org"),
			"repo": ctx.Param("repo"),
			"sha":  ctx.Param("sha"),
		})
	})

	// Wildcard
	app.Get("/static/*", func(ctx *fh.Ctx) error {
		return ctx.SendString("Serving: " + ctx.Param("*"))
	})

	// Query params
	app.Get("/search", func(ctx *fh.Ctx) error {
		q := ctx.Query("q")
		page := ctx.Query("page")
		if page == "" {
			page = "1"
		}
		return ctx.JSON(map[string]string{"query": q, "page": page})
	})

	// ── Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Signal received, initiating graceful shutdown...")
		if err := app.ShutdownWithTimeout(30 * time.Second); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	fmt.Println("fasthttp server starting on :8080")
	if err := app.Listen(":8080"); err != nil {
		log.Printf("Server stopped: %v", err)
	}
}

var start = time.Now()

// ── Handlers ──────────────────────────────────────────────────────────────

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func listUsers(ctx *fh.Ctx) error {
	users := []User{
		{ID: "1", Name: "Alice", Email: "alice@example.com"},
		{ID: "2", Name: "Bob", Email: "bob@example.com"},
	}
	return ctx.JSON(users)
}

func getUser(ctx *fh.Ctx) error {
	id := ctx.Param("id")
	return ctx.JSON(User{ID: id, Name: "Alice", Email: "alice@example.com"})
}

func createUser(ctx *fh.Ctx) error {
	var u User
	if err := ctx.BodyParser(&u); err != nil {
		return ctx.Status(400).SendString("Invalid JSON: " + err.Error())
	}
	u.ID = "3"
	return ctx.Status(201).JSON(u)
}

func updateUser(ctx *fh.Ctx) error {
	var u User
	if err := ctx.BodyParser(&u); err != nil {
		return ctx.Status(400).SendString("Invalid JSON: " + err.Error())
	}
	u.ID = ctx.Param("id")
	return ctx.JSON(u)
}

func patchUser(ctx *fh.Ctx) error {
	return ctx.JSON(map[string]string{"id": ctx.Param("id"), "status": "patched"})
}

func deleteUser(ctx *fh.Ctx) error {
	return ctx.SendStatus(204)
}
