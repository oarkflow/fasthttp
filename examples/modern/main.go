package main

import (
    "errors"
    "log"
    "time"

    "github.com/oarkflow/fh"
    "github.com/oarkflow/fh/mw/cors"
    "github.com/oarkflow/fh/mw/requestid"
    "github.com/oarkflow/fh/mw/security"
)

type CreateUserRequest struct {
    Name  string `json:"name" validate:"required" description:"Display name"`
    Email string `json:"email" validate:"required" format:"email"`
}
func (r CreateUserRequest) Validate() error {
    if r.Name == "" || r.Email == "" { return errors.New("name and email are required") }
    return nil
}

type UserResponse struct { ID string `json:"id"`; Name string `json:"name"`; Email string `json:"email"` }

func main() {
    app := fh.New(fh.Config{
        ReadTimeout: 5*time.Second, WriteTimeout: 10*time.Second, IdleTimeout: 60*time.Second,
        MaxRequestBodySize: 1<<20, MaxHeaderListSize: 32<<10, MaxHeaderCount: 64, MaxRequestLineSize: 8<<10,
        Reliability: fh.ReliabilityConfig{Enabled:true, DataDir:".fh-data", QueueWorkers:2},
    })

    app.Use(requestid.New())
    app.Use(security.New())
    app.Use(cors.New())
    app.Use(fh.APIKey(fh.APIKeyConfig{Header:"X-API-Key", Keys: []string{"dev-key"}}))
    app.Use(fh.ReplayProtection(nil, "X-Nonce", 5*time.Minute))

    app.PostTyped("/users", func(c *fh.Ctx, req CreateUserRequest) (UserResponse, error) {
        return UserResponse{ID:"usr_123", Name:req.Name, Email:req.Email}, nil
    })

    paymentPolicy := fh.ReliabilityPolicy{Enabled:true, RequireIdempotency:true, Journal:true, ReplayResponse:true, ConflictOnBodyDrift:true, MaxReplayAge:24*time.Hour}
    app.Post("/payments",
        fh.DeterministicIdempotency(func(c *fh.Ctx) string { return c.Get("X-Tenant-ID") + ":payment:" + c.Get("X-External-ID") }),
        fh.Reliable(paymentPolicy),
        func(c *fh.Ctx) error { return c.JSON(fh.Map{"status":"ok"}) },
    )

    app.Post("/emails", fh.Reliable(fh.ReliabilityPolicy{Enabled:true, RequireIdempotency:true, Journal:true, ReplayResponse:true}), func(c *fh.Ctx) error {
        job, err := fh.AtomicJob(c, fh.AtomicJobOptions{Type:"email.send", Body:c.BodyCopy(), Priority:fh.PriorityHigh})
        if err != nil { return err }
        return c.Status(202).JSON(fh.Map{"status":"accepted", "job_id":job.ID})
    })

    app.Get("/events", func(c *fh.Ctx) error { return c.SSE(func(s *fh.SSE) error { return s.Event("queue.stats", fh.Map{"ok":true}) }) })
    app.Get("/static/*", fh.StaticAdvanced("./public", fh.StaticAdvancedConfig{ETag:true, LastModified:true, CacheControl:"public, max-age=31536000, immutable"}))
    app.EnableOpenAPI("/openapi.json", fh.OpenAPIConfig{Title:"Modern fh API", Version:"2026-06-22"})
    app.EnableDocs("/docs")
    app.EnableRouteList("/_fh/routes")
    app.EnableMetrics("/_fh/metrics")

    log.Fatal(app.Listen(":3000"))
}
