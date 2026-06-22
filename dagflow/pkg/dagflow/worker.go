package dagflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const QueueWorkflowNode = "__workflow__"

type JobKind string

type ConsumerStatus string

type QueueConsumerHandler func(context.Context, Job) JobResult

const (
	JobKindNode     JobKind = "node"
	JobKindWorkflow JobKind = "workflow"

	ConsumerRunning ConsumerStatus = "running"
	ConsumerPaused  ConsumerStatus = "paused"
	ConsumerStopped ConsumerStatus = "stopped"
)

type Job struct {
	ID          string         `json:"id"`
	Kind        JobKind        `json:"kind,omitempty"`
	Queue       string         `json:"queue,omitempty"`
	TaskID      string         `json:"task_id"`
	WorkflowID  string         `json:"workflow_id"`
	NodeID      string         `json:"node_id"`
	Handler     string         `json:"handler"`
	Type        NodeType       `json:"type"`
	Params      map[string]any `json:"params,omitempty"`
	Input       any            `json:"input,omitempty"`
	Attempt     int            `json:"attempt"`
	MaxAttempts int            `json:"max_attempts,omitempty"`
	Priority    int            `json:"priority,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	AvailableAt time.Time      `json:"available_at,omitempty"`
}

type JobResult struct {
	JobID      string    `json:"job_id"`
	Queue      string    `json:"queue,omitempty"`
	TaskID     string    `json:"task_id"`
	WorkflowID string    `json:"workflow_id"`
	NodeID     string    `json:"node_id"`
	Result     any       `json:"result,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type Broker interface {
	Publish(context.Context, Job) error
	Subscribe(context.Context, string) (<-chan Job, error)
	Ack(context.Context, string) error
	Nack(context.Context, string, error) error
	Complete(context.Context, JobResult) error
	WaitResult(context.Context, string) (JobResult, error)
}

type ManagedBroker interface {
	Broker
	PublishToQueue(context.Context, string, Job) error
	SubscribeQueue(context.Context, string) (<-chan Job, error)
	StartConsumer(context.Context, QueueConsumerConfig, QueueConsumerHandler) error
	PauseConsumer(string) error
	ResumeConsumer(string) error
	StopConsumer(string) error
	ListConsumers() []QueueConsumerInfo
	ListQueues() []QueueInfo
}

type QueueConfig struct {
	ID                string `bcl:",id" json:"id"`
	Capacity          int    `bcl:"capacity,omitempty" json:"capacity,omitempty"`
	MaxAttempts       int    `bcl:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	VisibilityTimeout string `bcl:"visibility_timeout,omitempty" json:"visibility_timeout,omitempty"`
	DLQ               string `bcl:"dlq,omitempty" json:"dlq,omitempty"`
}

type QueueConsumerConfig struct {
	ID          string  `bcl:",id" json:"id"`
	Queue       string  `bcl:"queue" json:"queue"`
	Workflow    string  `bcl:"workflow,omitempty" json:"workflow,omitempty"`
	Concurrency int     `bcl:"concurrency,omitempty" json:"concurrency,omitempty"`
	Enabled     *bool   `bcl:"enabled,omitempty" json:"enabled,omitempty"`
	Mode        RunMode `bcl:"mode,ident,omitempty" json:"mode,omitempty"`
}

type QueueInfo struct {
	ID          string `json:"id"`
	Capacity    int    `json:"capacity"`
	Depth       int    `json:"depth"`
	Published   int64  `json:"published"`
	Acked       int64  `json:"acked"`
	Nacked      int64  `json:"nacked"`
	Completed   int64  `json:"completed"`
	Consumers   int    `json:"consumers"`
	MaxAttempts int    `json:"max_attempts,omitempty"`
	DLQ         string `json:"dlq,omitempty"`
}

type QueueConsumerInfo struct {
	ID          string         `json:"id"`
	Queue       string         `json:"queue"`
	Workflow    string         `json:"workflow,omitempty"`
	Concurrency int            `json:"concurrency"`
	Status      ConsumerStatus `json:"status"`
	StartedAt   time.Time      `json:"started_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
	Processed   int64          `json:"processed"`
	Failed      int64          `json:"failed"`
}

type memoryQueue struct {
	cfg       QueueConfig
	jobs      chan Job
	published int64
	acked     int64
	nacked    int64
	completed int64
}

type memoryConsumer struct {
	info   QueueConsumerInfo
	cancel context.CancelFunc
}

type MemoryBroker struct {
	mu        sync.RWMutex
	queues    map[string]*memoryQueue
	waiters   map[string][]chan JobResult
	results   map[string]JobResult
	consumers map[string]*memoryConsumer
	jobQueues map[string]string
}

func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{queues: map[string]*memoryQueue{}, waiters: map[string][]chan JobResult{}, results: map[string]JobResult{}, consumers: map[string]*memoryConsumer{}, jobQueues: map[string]string{}}
}

func (b *MemoryBroker) EnsureQueue(cfg QueueConfig) error {
	if cfg.ID == "" {
		return errors.New("queue id is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if q := b.queues[cfg.ID]; q != nil {
		if cfg.Capacity > 0 && cfg.Capacity != cap(q.jobs) && len(q.jobs) == 0 {
			q.jobs = make(chan Job, cfg.Capacity)
		}
		if cfg.MaxAttempts > 0 {
			q.cfg.MaxAttempts = cfg.MaxAttempts
		}
		if cfg.DLQ != "" {
			q.cfg.DLQ = cfg.DLQ
		}
		return nil
	}
	capv := cfg.Capacity
	if capv <= 0 {
		capv = 4096
	}
	b.queues[cfg.ID] = &memoryQueue{cfg: QueueConfig{ID: cfg.ID, Capacity: capv, MaxAttempts: cfg.MaxAttempts, VisibilityTimeout: cfg.VisibilityTimeout, DLQ: cfg.DLQ}, jobs: make(chan Job, capv)}
	return nil
}

func (b *MemoryBroker) getQueue(name string) *memoryQueue {
	if name == "" {
		name = "default"
	}
	b.mu.RLock()
	q := b.queues[name]
	b.mu.RUnlock()
	if q != nil {
		return q
	}
	_ = b.EnsureQueue(QueueConfig{ID: name})
	b.mu.RLock()
	q = b.queues[name]
	b.mu.RUnlock()
	return q
}

func (b *MemoryBroker) Publish(ctx context.Context, j Job) error {
	queue := j.Queue
	if queue == "" {
		queue = j.WorkflowID
	}
	return b.PublishToQueue(ctx, queue, j)
}

func (b *MemoryBroker) PublishToQueue(ctx context.Context, queue string, j Job) error {
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
	q := b.getQueue(queue)
	select {
	case q.jobs <- j:
		b.mu.Lock()
		q.published++
		b.jobQueues[j.ID] = queue
		b.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *MemoryBroker) Subscribe(ctx context.Context, workflowID string) (<-chan Job, error) {
	return b.SubscribeQueue(ctx, workflowID)
}

func (b *MemoryBroker) SubscribeQueue(ctx context.Context, queue string) (<-chan Job, error) {
	return b.getQueue(queue).jobs, nil
}

func (b *MemoryBroker) Ack(ctx context.Context, jobID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if queue := b.jobQueues[jobID]; queue != "" {
		if q := b.queues[queue]; q != nil {
			q.acked++
		}
	}
	return nil
}

func (b *MemoryBroker) Nack(ctx context.Context, jobID string, err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if queue := b.jobQueues[jobID]; queue != "" {
		if q := b.queues[queue]; q != nil {
			q.nacked++
		}
	}
	return nil
}

func (b *MemoryBroker) Complete(ctx context.Context, r JobResult) error {
	var waiters []chan JobResult
	b.mu.Lock()
	b.results[r.JobID] = r
	waiters = b.waiters[r.JobID]
	delete(b.waiters, r.JobID)
	if r.Queue != "" {
		if q := b.queues[r.Queue]; q != nil {
			q.completed++
		}
	}
	b.mu.Unlock()
	for _, ch := range waiters {
		select {
		case ch <- r:
		default:
		}
		close(ch)
	}
	return nil
}

func (b *MemoryBroker) WaitResult(ctx context.Context, jobID string) (JobResult, error) {
	var existing JobResult
	var found bool
	ch := make(chan JobResult, 1)
	b.mu.Lock()
	existing, found = b.results[jobID]
	if !found {
		b.waiters[jobID] = append(b.waiters[jobID], ch)
	}
	b.mu.Unlock()
	if found {
		return existing, nil
	}
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return JobResult{}, ctx.Err()
	}
}

func (b *MemoryBroker) StartConsumer(ctx context.Context, cfg QueueConsumerConfig, handler QueueConsumerHandler) error {
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
	_ = b.EnsureQueue(QueueConfig{ID: cfg.Queue})
	queue := b.getQueue(cfg.Queue)
	b.mu.Lock()
	if old := b.consumers[cfg.ID]; old != nil && old.info.Status != ConsumerStopped {
		b.mu.Unlock()
		return fmt.Errorf("consumer %s already running", cfg.ID)
	}
	cctx, cancel := context.WithCancel(ctx)
	mc := &memoryConsumer{info: QueueConsumerInfo{ID: cfg.ID, Queue: cfg.Queue, Workflow: cfg.Workflow, Concurrency: cfg.Concurrency, Status: ConsumerRunning, StartedAt: time.Now(), UpdatedAt: time.Now()}, cancel: cancel}
	b.consumers[cfg.ID] = mc
	jobs := queue.jobs
	b.mu.Unlock()
	for i := 0; i < cfg.Concurrency; i++ {
		go b.consumerLoop(cctx, cfg.ID, jobs, handler)
	}
	return nil
}

func (b *MemoryBroker) consumerLoop(ctx context.Context, id string, jobs <-chan Job, handler QueueConsumerHandler) {
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
		case job := <-jobs:
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

func (b *MemoryBroker) bumpConsumer(id string, ok bool) {
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

func (b *MemoryBroker) PauseConsumer(id string) error {
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

func (b *MemoryBroker) ResumeConsumer(id string) error {
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

func (b *MemoryBroker) StopConsumer(id string) error {
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

func (b *MemoryBroker) ListConsumers() []QueueConsumerInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]QueueConsumerInfo, 0, len(b.consumers))
	for _, c := range b.consumers {
		out = append(out, c.info)
	}
	return out
}

func (b *MemoryBroker) ListQueues() []QueueInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]QueueInfo, 0, len(b.queues))
	for _, q := range b.queues {
		consumers := 0
		for _, c := range b.consumers {
			if c.info.Queue == q.cfg.ID && c.info.Status != ConsumerStopped {
				consumers++
			}
		}
		out = append(out, QueueInfo{ID: q.cfg.ID, Capacity: cap(q.jobs), Depth: len(q.jobs), Published: q.published, Acked: q.acked, Nacked: q.nacked, Completed: q.completed, Consumers: consumers, MaxAttempts: q.cfg.MaxAttempts, DLQ: q.cfg.DLQ})
	}
	return out
}

type localRequest struct {
	job    Job
	result chan JobResult
}

type LocalQueue struct {
	engine  *Engine
	workers int
	jobs    chan localRequest
}

func NewLocalQueue(e *Engine, workers int) *LocalQueue {
	if workers <= 0 {
		workers = 1
	}
	return &LocalQueue{engine: e, workers: workers, jobs: make(chan localRequest, 1024)}
}
func (q *LocalQueue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		go func() {
			for {
				select {
				case req := <-q.jobs:
					jr := q.engine.executeJob(ctx, req.job)
					if req.result != nil {
						req.result <- jr
						close(req.result)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}
func (q *LocalQueue) SubmitAndWait(ctx context.Context, job Job) (JobResult, error) {
	ch := make(chan JobResult, 1)
	select {
	case q.jobs <- localRequest{job: job, result: ch}:
	case <-ctx.Done():
		return JobResult{}, ctx.Err()
	}
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return JobResult{}, ctx.Err()
	}
}
func (q *LocalQueue) SubmitFireAndForget(ctx context.Context, job Job) error {
	select {
	case q.jobs <- localRequest{job: job}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) newJob(task *Task, node *Node, input any, attempt int) Job {
	queue := node.Pool
	if queue == "" {
		queue = task.WorkflowID
	}
	return Job{ID: newID("job"), Kind: JobKindNode, Queue: queue, TaskID: task.ID, WorkflowID: task.WorkflowID, NodeID: node.ID, Handler: node.Handler, Type: node.Type, Params: node.Params, Input: input, Attempt: attempt, MaxAttempts: node.RetryPolicy.MaxAttempts, Priority: node.Priority, CreatedAt: time.Now()}
}

func (e *Engine) executeJob(ctx context.Context, job Job) JobResult {
	if job.Kind == JobKindWorkflow || job.NodeID == QueueWorkflowNode {
		return e.executeWorkflowQueueJob(ctx, job)
	}
	started := time.Now()
	leaseID := ""
	if s, ok := e.store.(ExtendedStore); ok {
		leaseID = newID("lease")
		_ = s.CreateLease(WorkerLease{ID: leaseID, TaskID: job.TaskID, NodeID: job.NodeID, JobID: job.ID, WorkerID: "worker-" + job.Queue, ExpiresAt: time.Now().Add(30 * time.Second), BeatAt: time.Now()})
		done := make(chan struct{})
		defer close(done)
		go func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					_ = s.HeartbeatLease(leaseID, 30*time.Second)
				case <-done:
					return
				}
			}
		}()
	}
	wf := &Workflow{ID: job.WorkflowID}
	task := &Task{ID: job.TaskID, WorkflowID: job.WorkflowID, NodeResults: map[string]any{}}
	node := &Node{ID: job.NodeID, Type: job.Type, Handler: job.Handler, Params: job.Params, Mode: ModeInline, Await: true}
	res, err := e.executeHandler(ctx, wf, task, node, job.Input, job.Attempt)
	jr := JobResult{JobID: job.ID, Queue: job.Queue, TaskID: job.TaskID, WorkflowID: job.WorkflowID, NodeID: job.NodeID, Result: res, StartedAt: started, FinishedAt: time.Now()}
	if err != nil {
		jr.Error = err.Error()
	}
	if s, ok := e.store.(ExtendedStore); ok && leaseID != "" {
		_ = s.DeleteLease(leaseID)
	}
	return jr
}

func (e *Engine) executeWorkflowQueueJob(ctx context.Context, job Job) JobResult {
	started := time.Now()
	jr := JobResult{JobID: job.ID, Queue: job.Queue, TaskID: job.TaskID, WorkflowID: job.WorkflowID, NodeID: QueueWorkflowNode, StartedAt: started}
	task, err := e.store.Get(job.TaskID)
	if err != nil {
		jr.Error = err.Error()
		jr.FinishedAt = time.Now()
		return jr
	}
	wf, err := e.workflow(job.WorkflowID)
	if err != nil {
		e.finishTask(task, err)
		jr.Error = err.Error()
		jr.FinishedAt = time.Now()
		return jr
	}
	task.Status = TaskRunning
	task.UpdatedAt = time.Now()
	e.audit(task, "queue.task.started", "queued workflow task started", map[string]any{"queue": job.Queue, "job_id": job.ID})
	_ = e.store.Save(task)
	err = e.executeTask(ctx, wf, task, []RunItem{{NodeID: wf.First, Input: task.Input}})
	if err == nil && task.Status != TaskWaiting && task.Status != TaskPaused && task.Status != TaskCancelled {
		task.Result, err = e.applyWorkflowOutput(ctx, wf, task, task.Result)
	}
	e.finishTask(task, err)
	if err != nil {
		jr.Error = err.Error()
	}
	jr.Result = task.Result
	jr.FinishedAt = time.Now()
	return jr
}

func (e *Engine) StartDistributedWorker(ctx context.Context, workflowID string, concurrency int) {
	if concurrency <= 0 {
		concurrency = 1
	}
	if mb, ok := e.broker.(ManagedBroker); ok {
		id := "node-worker:" + workflowID
		_ = mb.StartConsumer(ctx, QueueConsumerConfig{ID: id, Queue: workflowID, Workflow: workflowID, Concurrency: concurrency}, e.executeJob)
		return
	}
	jobs, err := e.broker.Subscribe(ctx, workflowID)
	if err != nil {
		return
	}
	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case job := <-jobs:
					jr := e.executeJob(ctx, job)
					if jr.Error != "" {
						_ = e.broker.Nack(ctx, job.ID, errors.New(jr.Error))
					} else {
						_ = e.broker.Ack(ctx, job.ID)
					}
					_ = e.broker.Complete(ctx, jr)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}
