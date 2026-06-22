# workflow middleware

`workflow` composes a sequence of handler steps and durable job handoff steps into one route handler.

## Import

```go
import "github.com/oarkflow/fh/mw/workflow"
```

## Usage

```go
flow := workflow.New("signup").
    Use("validate", func(c *fh.Ctx) error {
        // validate request
        return nil
    }).
    Use("create_user", func(c *fh.Ctx) error {
        // create user
        return nil
    }).
    Job("send_welcome_email", "email.send")

app.Post("/signup", flow.Handler())
```

## Conditional step

```go
flow := workflow.New("order")
flow.Steps = append(flow.Steps, workflow.Step{
    Name: "notify_vip",
    Handler: notifyVIP,
    Condition: func(c *fh.Ctx) bool {
        return c.Get("X-Plan") == "vip"
    },
})
```

## Behavior

- Marks lifecycle processing/completed using the core lifecycle API.
- Handler steps execute inline.
- Job steps call `fh.AtomicHandoff` and store `job_id` in locals.

## Best practice

Use this for simple linear workflows. For complex DAGs, retries per step, branching, or long-running orchestration, use a dedicated workflow engine or the core queue/reliability primitives directly.
