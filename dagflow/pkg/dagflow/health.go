package dagflow

import (
	"context"
	"time"
)

type HealthReport struct {
	Status    string              `json:"status"`
	Time      time.Time           `json:"time"`
	Store     string              `json:"store"`
	Broker    string              `json:"broker"`
	Queues    []QueueInfo         `json:"queues,omitempty"`
	Consumers []QueueConsumerInfo `json:"consumers,omitempty"`
	Error     string              `json:"error,omitempty"`
}

type healthCheckStore interface{ Health(context.Context) error }

func (e *Engine) Health(ctx context.Context) HealthReport {
	r := HealthReport{Status: "ok", Time: time.Now(), Store: "ok", Broker: "ok"}
	if hs, ok := e.store.(healthCheckStore); ok {
		if err := hs.Health(ctx); err != nil {
			r.Status = "degraded"
			r.Store = "error"
			r.Error = err.Error()
		}
	}
	if mb, ok := e.broker.(ManagedBroker); ok {
		r.Queues = mb.ListQueues()
		r.Consumers = mb.ListConsumers()
	}
	return r
}
