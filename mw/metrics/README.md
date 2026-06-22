# metrics middleware

`metrics` tracks basic request counters and exposes metrics as JSON or Prometheus text format.

## Import

```go
import "github.com/oarkflow/fh/mw/metrics"
```

## Usage

```go
m := metrics.New()
app.Use(m.Middleware())
app.Get("/_fh/metrics", m.Handler())
```

## JSON output

```bash
curl http://localhost:3000/_fh/metrics
```

Returns uptime, total requests, in-flight requests, status counters, and route counters.

## Prometheus output

```bash
curl 'http://localhost:3000/_fh/metrics?format=prometheus'
```

## Best practice

Protect metrics endpoints on public deployments with `basicauth`, `apikey`, or IP allowlists. Put metrics early enough to capture most requests.
