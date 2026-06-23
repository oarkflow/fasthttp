# workflow middleware

`workflow` composes a sequence of handler steps and durable job handoff steps into a single route handler. It supports **linear pipelines**, **conditional steps**, **branching** (first-match sub-workflow routing), and **parallel fan-out** (concurrent sub-workflows).

## Import

```go
import "github.com/oarkflow/fh/mw/workflow"
```

## Step types

| Type | Builder | Description |
|------|---------|-------------|
| `StepSync` | `Use()` | Inline handler, executed sequentially |
| `StepAsync` | `Job()` | Enqueues a durable async job via `fh.AtomicHandoff` |
| `StepBranch` | `Branch()` | First-matching sub-workflow executes |
| `StepParallel` | `Parallel()` | All sub-workflows run concurrently |

---

## 1. Basic linear workflow

Steps execute in the order they are defined.

```go
wf := workflow.New("signup").
    Use("validate", func(c *fh.Ctx) error {
        // validate request
        return nil
    }).
    Use("create_user", func(c *fh.Ctx) error {
        // create user in database
        return nil
    }).
    Job("send_welcome_email", "email.send").
    Use("respond", func(c *fh.Ctx) error {
        return c.JSON(fh.Map{"status": "ok"})
    })

app.Post("/signup", wf.Handler())
```

Job steps call `fh.AtomicHandoff` and store the `job_id` in `c.Locals("job_id")`.

---

## 2. Conditional steps

Pass a condition function as the third argument to `Use()` or `Job()`. The step is skipped when the condition returns `false`.

```go
wf := workflow.New("order").
    Use("validate", validateOrder).
    Use("apply_discount", applyDiscount, func(c *fh.Ctx) bool {
        return c.Get("X-Plan") == "vip"
    }).
    Use("apply_standard", applyStandard, func(c *fh.Ctx) bool {
        return c.Get("X-Plan") != "vip"
    }).
    Use("respond", respond)
```

---

## 3. Branching (first-match routing)

`Branch()` evaluates sub-workflows in order. The first sub-workflow whose `Condition()` returns `true` is executed. If none match and a sub-workflow has no condition, it acts as a catch-all default.

```go
wf := workflow.New("routing").
    Use("validate", validate).
    Branch("plan_routing",
        workflow.New("vip").Condition(func(c *fh.Ctx) bool {
            return c.Get("X-Plan") == "vip"
        }).Use("handler", vipHandler).Job("notify", "vip.notify"),

        workflow.New("standard").Condition(func(c *fh.Ctx) bool {
            return c.Get("X-Plan") == "standard"
        }).Use("handler", standardHandler),

        workflow.New("default").Use("handler", defaultHandler),
    ).
    Use("respond", respond)
```

---

## 4. Parallel fan-out

`Parallel()` runs all sub-workflows concurrently using goroutines. Each branch can have its own linear steps and async jobs. `c.Locals()` is thread-safe (backed by a mutex) so parallel branches can safely read and write locals.

```go
wf := workflow.New("checkout").
    Use("validate", validate).
    Parallel("fulfill",
        workflow.New("payment").
            Use("charge", chargeCard).
            Job("invoice", "billing.invoice"),

        workflow.New("inventory").
            Use("reserve", reserveStock),

        workflow.New("notifications").
            Job("send", "notification.send"),
    ).
    Use("respond", respond)
```

Errors from any parallel branch are collected; the first error is returned.

---

## 5. Workflow-level condition

Set a condition on the workflow itself with `Condition()`. The entire workflow is skipped when it returns `false`.

```go
wf := workflow.New("beta_feature").
    Condition(func(c *fh.Ctx) bool {
        return c.Get("X-Beta") == "enabled"
    }).
    Use("process", process).
    Use("respond", respond)
```

---

## Behavior

- **Lifecycle**: sync steps mark `LifecycleProcessing`/`LifecycleCompleted` using the core lifecycle API.
- **Job steps**: call `fh.AtomicHandoff` and store the returned job ID in `c.Locals("job_id")`.
- **Branching**: evaluates conditions in declaration order; executes the first match; no-op if no match and no default.
- **Parallel**: launches goroutines with `sync.WaitGroup`; `Ctx.Locals()` is safe for concurrent access.
- **Error handling**: any step error halts the workflow (sync/branch) or is collected (parallel). The first error is returned to the caller.

## Best practices

- Use linear workflows for simple request-scoped pipelines.
- Use branching for request-routing or A/B test splits.
- Use parallel fan-out for independent side-effects (e.g. payment + inventory + notification).
- Use async jobs (`Job()`) for durable, retriable background work.
- For complex DAGs with retry-per-step or long-running orchestration, consider a dedicated workflow engine.
