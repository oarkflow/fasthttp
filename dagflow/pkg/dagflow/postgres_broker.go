package dagflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

type DurableQueueStore interface {
	EnqueueJob(context.Context, Job) error
	ClaimJob(context.Context, string, string, time.Duration) (Job, error)
	AckJob(context.Context, string) error
	NackJob(context.Context, string, error, time.Duration, int) error
	CompleteJob(context.Context, JobResult) error
	WaitJobResult(context.Context, string) (JobResult, error)
	RecoverExpiredJobs(context.Context) error
}

type PostgresBroker struct {
	store       DurableQueueStore
	workerID    string
	lease       time.Duration
	maxAttempts int
	mu          sync.RWMutex
	consumers   map[string]*memoryConsumer
	queues      map[string]QueueConfig
}

func NewPostgresBroker(store DurableQueueStore, workerID string) *PostgresBroker {
	if workerID == "" {
		workerID = "worker-" + newID("pg")
	}
	return &PostgresBroker{store: store, workerID: workerID, lease: 45 * time.Second, maxAttempts: 5, consumers: map[string]*memoryConsumer{}, queues: map[string]QueueConfig{}}
}

func (b *PostgresBroker) EnsureQueue(cfg QueueConfig) error {
	if cfg.ID == "" {
		return errors.New("queue id is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.queues[cfg.ID] = cfg
	return nil
}

func (b *PostgresBroker) Publish(ctx context.Context, j Job) error {
	queue := j.Queue
	if queue == "" {
		queue = j.WorkflowID
	}
	return b.PublishToQueue(ctx, queue, j)
}
func (b *PostgresBroker) PublishToQueue(ctx context.Context, queue string, j Job) error {
	if queue == "" {
		queue = j.WorkflowID
	}
	if j.ID == "" {
		j.ID = newID("job")
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}
	if j.Kind == "" {
		j.Kind = JobKindNode
	}
	j.Queue = queue
	b.mu.Lock()
	if _, ok := b.queues[queue]; !ok {
		b.queues[queue] = QueueConfig{ID: queue}
	}
	b.mu.Unlock()
	return b.store.EnqueueJob(ctx, j)
}
func (b *PostgresBroker) Subscribe(ctx context.Context, workflowID string) (<-chan Job, error) {
	return b.SubscribeQueue(ctx, workflowID)
}
func (b *PostgresBroker) SubscribeQueue(ctx context.Context, queue string) (<-chan Job, error) {
	ch := make(chan Job)
	go func() {
		defer close(ch)
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			_ = b.store.RecoverExpiredJobs(ctx)
			job, err := b.store.ClaimJob(ctx, queue, b.workerID, b.lease)
			if err == nil {
				if job.Queue == "" {
					job.Queue = queue
				}
				select {
				case ch <- job:
				case <-ctx.Done():
					return
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				time.Sleep(500 * time.Millisecond)
			}
			select {
			case <-tick.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
func (b *PostgresBroker) Ack(ctx context.Context, jobID string) error {
	return b.store.AckJob(ctx, jobID)
}
func (b *PostgresBroker) Nack(ctx context.Context, jobID string, err error) error {
	return b.store.NackJob(ctx, jobID, err, 2*time.Second, b.maxAttempts)
}
func (b *PostgresBroker) Complete(ctx context.Context, r JobResult) error {
	return b.store.CompleteJob(ctx, r)
}
func (b *PostgresBroker) WaitResult(ctx context.Context, jobID string) (JobResult, error) {
	return b.store.WaitJobResult(ctx, jobID)
}
func (b *PostgresBroker) StartConsumer(ctx context.Context, cfg QueueConsumerConfig, handler QueueConsumerHandler) error {
	if cfg.ID == "" {
		return errors.New("consumer id is required")
	}
	if cfg.Queue == "" {
		return errors.New("consumer queue is required")
	}
	if handler == nil {
		return errors.New("consumer handler is required")
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	b.mu.Lock()
	if old := b.consumers[cfg.ID]; old != nil && old.info.Status != ConsumerStopped {
		b.mu.Unlock()
		return fmt.Errorf("consumer %s already running", cfg.ID)
	}
	if _, ok := b.queues[cfg.Queue]; !ok {
		b.queues[cfg.Queue] = QueueConfig{ID: cfg.Queue}
	}
	cctx, cancel := context.WithCancel(ctx)
	mc := &memoryConsumer{info: QueueConsumerInfo{ID: cfg.ID, Queue: cfg.Queue, Workflow: cfg.Workflow, Concurrency: cfg.Concurrency, Status: ConsumerRunning, StartedAt: time.Now(), UpdatedAt: time.Now()}, cancel: cancel}
	b.consumers[cfg.ID] = mc
	b.mu.Unlock()
	jobs, err := b.SubscribeQueue(cctx, cfg.Queue)
	if err != nil {
		return err
	}
	for i := 0; i < cfg.Concurrency; i++ {
		go b.consumerLoop(cctx, cfg.ID, jobs, handler)
	}
	return nil
}
func (b *PostgresBroker) consumerLoop(ctx context.Context, id string, jobs <-chan Job, handler QueueConsumerHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		b.mu.RLock()
		status := ConsumerStopped
		if c := b.consumers[id]; c != nil {
			status = c.info.Status
		}
		b.mu.RUnlock()
		if status == ConsumerStopped {
			return
		}
		if status == ConsumerPaused {
			select {
			case <-time.After(100 * time.Millisecond):
				continue
			case <-ctx.Done():
				return
			}
		}
		select {
		case job, ok := <-jobs:
			if !ok {
				return
			}
			jr := handler(ctx, job)
			if jr.Queue == "" {
				jr.Queue = job.Queue
			}
			if jr.Error != "" {
				_ = b.Nack(ctx, job.ID, errors.New(jr.Error))
				b.bumpConsumer(id, false)
			} else {
				_ = b.Ack(ctx, job.ID)
				b.bumpConsumer(id, true)
			}
			_ = b.Complete(ctx, jr)
		case <-ctx.Done():
			return
		}
	}
}
func (b *PostgresBroker) bumpConsumer(id string, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c := b.consumers[id]; c != nil {
		if ok {
			c.info.Processed++
		} else {
			c.info.Failed++
		}
		c.info.UpdatedAt = time.Now()
	}
}
func (b *PostgresBroker) PauseConsumer(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.consumers[id]
	if c == nil {
		return fmt.Errorf("consumer %s not found", id)
	}
	if c.info.Status == ConsumerStopped {
		return fmt.Errorf("consumer %s is stopped", id)
	}
	c.info.Status = ConsumerPaused
	c.info.UpdatedAt = time.Now()
	return nil
}
func (b *PostgresBroker) ResumeConsumer(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.consumers[id]
	if c == nil {
		return fmt.Errorf("consumer %s not found", id)
	}
	if c.info.Status == ConsumerStopped {
		return fmt.Errorf("consumer %s is stopped", id)
	}
	c.info.Status = ConsumerRunning
	c.info.UpdatedAt = time.Now()
	return nil
}
func (b *PostgresBroker) StopConsumer(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.consumers[id]
	if c == nil {
		return fmt.Errorf("consumer %s not found", id)
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.info.Status = ConsumerStopped
	c.info.UpdatedAt = time.Now()
	return nil
}
func (b *PostgresBroker) ListConsumers() []QueueConsumerInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]QueueConsumerInfo, 0, len(b.consumers))
	for _, c := range b.consumers {
		out = append(out, c.info)
	}
	return out
}
func (b *PostgresBroker) ListQueues() []QueueInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]QueueInfo, 0, len(b.queues))
	for _, q := range b.queues {
		consumers := 0
		for _, c := range b.consumers {
			if c.info.Queue == q.ID && c.info.Status != ConsumerStopped {
				consumers++
			}
		}
		out = append(out, QueueInfo{ID: q.ID, Capacity: q.Capacity, Consumers: consumers, MaxAttempts: q.MaxAttempts, DLQ: q.DLQ})
	}
	return out
}
