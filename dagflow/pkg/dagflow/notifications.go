package dagflow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type NotificationChannelType string

type NotificationDeliveryStatus string

const (
	NotifyLog      NotificationChannelType = "log"
	NotifyCallback NotificationChannelType = "callback"
	NotifyWebhook  NotificationChannelType = "webhook"
	NotifyEmail    NotificationChannelType = "email"
	NotifySMS      NotificationChannelType = "sms"

	NotifyPending   NotificationDeliveryStatus = "pending"
	NotifyDelivered NotificationDeliveryStatus = "delivered"
	NotifyFailed    NotificationDeliveryStatus = "failed"
	NotifySkipped   NotificationDeliveryStatus = "skipped"
)

type NotificationChannel struct {
	ID       string                  `json:"id" bcl:",id"`
	Type     NotificationChannelType `json:"type" bcl:"type,ident"`
	Enabled  *bool                   `json:"enabled,omitempty" bcl:"enabled,omitempty"`
	Endpoint string                  `json:"endpoint,omitempty" bcl:"endpoint,omitempty"`
	Method   string                  `json:"method,omitempty" bcl:"method,omitempty"`
	Secret   string                  `json:"-" bcl:"secret,omitempty"`
	Headers  map[string]string       `json:"headers,omitempty" bcl:"headers,omitempty"`
	Timeout  string                  `json:"timeout,omitempty" bcl:"timeout,omitempty"`
	Retries  int                     `json:"retries,omitempty" bcl:"retries,omitempty"`
	From     string                  `json:"from,omitempty" bcl:"from,omitempty"`
	To       []string                `json:"to,omitempty" bcl:"to,omitempty"`
	Subject  string                  `json:"subject,omitempty" bcl:"subject,omitempty"`
	SMTPHost string                  `json:"smtp_host,omitempty" bcl:"smtp_host,omitempty"`
	SMTPPort string                  `json:"smtp_port,omitempty" bcl:"smtp_port,omitempty"`
	Username string                  `json:"username,omitempty" bcl:"username,omitempty"`
	Password string                  `json:"-" bcl:"password,omitempty"`
	Params   map[string]any          `json:"params,omitempty" bcl:"params,omitempty"`
}

type NotificationRule struct {
	ID        string            `json:"id" bcl:",id"`
	Enabled   *bool             `json:"enabled,omitempty" bcl:"enabled,omitempty"`
	Events    []string          `json:"events,omitempty" bcl:"events,omitempty"`
	Channels  []string          `json:"channels,omitempty" bcl:"channels,omitempty"`
	When      string            `json:"when,omitempty" bcl:"when,omitempty"`
	Condition string            `json:"condition,omitempty" bcl:"condition,omitempty"`
	Title     string            `json:"title,omitempty" bcl:"title,omitempty"`
	Message   string            `json:"message,omitempty" bcl:"message,omitempty"`
	Severity  string            `json:"severity,omitempty" bcl:"severity,omitempty"`
	Data      DataSpec          `json:"data,omitempty" bcl:"data,block,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" bcl:"headers,omitempty"`
}

type NotificationMessage struct {
	ID         string            `json:"id"`
	RuleID     string            `json:"rule_id,omitempty"`
	ChannelID  string            `json:"channel_id,omitempty"`
	Event      string            `json:"event"`
	Title      string            `json:"title,omitempty"`
	Message    string            `json:"message,omitempty"`
	Severity   string            `json:"severity,omitempty"`
	TaskID     string            `json:"task_id,omitempty"`
	WorkflowID string            `json:"workflow_id,omitempty"`
	NodeID     string            `json:"node_id,omitempty"`
	Payload    any               `json:"payload,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

type NotificationDelivery struct {
	ID         string                     `json:"id"`
	MessageID  string                     `json:"message_id"`
	RuleID     string                     `json:"rule_id,omitempty"`
	ChannelID  string                     `json:"channel_id"`
	Channel    NotificationChannelType    `json:"channel"`
	TaskID     string                     `json:"task_id,omitempty"`
	WorkflowID string                     `json:"workflow_id,omitempty"`
	NodeID     string                     `json:"node_id,omitempty"`
	Event      string                     `json:"event"`
	Status     NotificationDeliveryStatus `json:"status"`
	Attempts   int                        `json:"attempts"`
	Error      string                     `json:"error,omitempty"`
	CreatedAt  time.Time                  `json:"created_at"`
	UpdatedAt  time.Time                  `json:"updated_at"`
}

type NotificationStore interface {
	SaveNotificationDelivery(NotificationDelivery) error
	ListNotificationDeliveries() []NotificationDelivery
}

type NotificationHandler interface {
	Deliver(context.Context, NotificationChannel, NotificationMessage) error
}

type NotificationHandlerFunc func(context.Context, NotificationChannel, NotificationMessage) error
type NotificationCallback func(context.Context, NotificationMessage) error

func (f NotificationHandlerFunc) Deliver(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	return f(ctx, ch, msg)
}

type NotificationDispatcher struct {
	mu        sync.RWMutex
	channels  map[string]NotificationChannel
	handlers  map[NotificationChannelType]NotificationHandler
	callbacks map[string]NotificationCallback
	client    *http.Client
}

func NewNotificationDispatcher() *NotificationDispatcher {
	d := &NotificationDispatcher{channels: map[string]NotificationChannel{}, handlers: map[NotificationChannelType]NotificationHandler{}, callbacks: map[string]NotificationCallback{}, client: &http.Client{Timeout: 10 * time.Second}}
	d.RegisterHandler(NotifyLog, NotificationHandlerFunc(d.deliverLog))
	d.RegisterHandler(NotifyCallback, NotificationHandlerFunc(d.deliverCallback))
	d.RegisterHandler(NotifyWebhook, NotificationHandlerFunc(d.deliverWebhook))
	d.RegisterHandler(NotifyEmail, NotificationHandlerFunc(d.deliverEmail))
	d.RegisterHandler(NotifySMS, NotificationHandlerFunc(d.deliverSMS))
	return d
}

func (d *NotificationDispatcher) RegisterChannel(ch NotificationChannel) error {
	if ch.ID == "" {
		return errors.New("notification channel id is required")
	}
	if ch.Type == "" {
		ch.Type = NotifyLog
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels[ch.ID] = ch
	return nil
}

func (d *NotificationDispatcher) RegisterCallback(channelID string, cb NotificationCallback) {
	if d == nil || channelID == "" || cb == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callbacks[channelID] = cb
}

func (d *NotificationDispatcher) RegisterHandler(t NotificationChannelType, h NotificationHandler) {
	if d == nil || h == nil || t == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[t] = h
}

func (d *NotificationDispatcher) Channel(id string) (NotificationChannel, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ch, ok := d.channels[id]
	return ch, ok
}

func (d *NotificationDispatcher) Deliver(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	if ch.Enabled != nil && !*ch.Enabled {
		return nil
	}
	d.mu.RLock()
	h := d.handlers[ch.Type]
	d.mu.RUnlock()
	if h == nil {
		return fmt.Errorf("notification channel type %q has no registered handler", ch.Type)
	}
	return h.Deliver(ctx, ch, msg)
}

func (d *NotificationDispatcher) deliverCallback(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	d.mu.RLock()
	cb := d.callbacks[ch.ID]
	d.mu.RUnlock()
	if cb == nil {
		return fmt.Errorf("callback channel %s has no registered callback", ch.ID)
	}
	return cb(ctx, msg)
}

func (d *NotificationDispatcher) deliverLog(_ context.Context, ch NotificationChannel, msg NotificationMessage) error {
	level := msg.Severity
	if level == "" {
		level = "info"
	}
	log.Printf("dagflow notification channel=%s level=%s event=%s task=%s workflow=%s node=%s title=%q message=%q payload=%s", ch.ID, level, msg.Event, msg.TaskID, msg.WorkflowID, msg.NodeID, msg.Title, msg.Message, compactJSON(msg.Payload))
	return nil
}

func (d *NotificationDispatcher) deliverWebhook(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	if ch.Endpoint == "" {
		return fmt.Errorf("webhook channel %s requires endpoint", ch.ID)
	}
	method := ch.Method
	if method == "" {
		method = http.MethodPost
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, ch.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dagflow-notifier/1.0")
	for k, v := range ch.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range msg.Headers {
		req.Header.Set(k, v)
	}
	if ch.Secret != "" {
		sig := signHMACSHA256(ch.Secret, body)
		req.Header.Set("X-Dagflow-Signature", sig)
		req.Header.Set("X-Dagflow-Event", msg.Event)
		req.Header.Set("X-Dagflow-Message-ID", msg.ID)
	}
	client := d.client
	if ch.Timeout != "" {
		if to, err := time.ParseDuration(ch.Timeout); err == nil && to > 0 {
			client = &http.Client{Timeout: to}
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func (d *NotificationDispatcher) deliverEmail(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	if ch.SMTPHost == "" || ch.From == "" || len(ch.To) == 0 {
		return fmt.Errorf("email channel %s requires smtp_host, from and to", ch.ID)
	}
	port := ch.SMTPPort
	if port == "" {
		port = "587"
	}
	addr := ch.SMTPHost + ":" + port
	subject := ch.Subject
	if msg.Title != "" {
		subject = msg.Title
	}
	if subject == "" {
		subject = "dagflow notification: " + msg.Event
	}
	body := msg.Message
	if body == "" {
		body = compactJSON(msg.Payload)
	}
	raw := "From: " + ch.From + "\r\nTo: " + strings.Join(ch.To, ",") + "\r\nSubject: " + sanitizeHeader(subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body + "\r\n"
	var auth smtp.Auth
	if ch.Username != "" || ch.Password != "" {
		auth = smtp.PlainAuth("", ch.Username, ch.Password, ch.SMTPHost)
	}
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, auth, ch.From, ch.To, []byte(raw)) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (d *NotificationDispatcher) deliverSMS(ctx context.Context, ch NotificationChannel, msg NotificationMessage) error {
	// SMS delivery is implemented as a generic signed HTTP provider adapter. This keeps the core independent from any vendor while remaining production usable with providers such as Twilio-compatible, AWS SNS via HTTPS proxy, or local gateways.
	if ch.Endpoint == "" {
		return fmt.Errorf("sms channel %s requires endpoint", ch.ID)
	}
	payload := map[string]any{"to": ch.To, "message": firstNonEmpty(msg.Message, msg.Title, compactJSON(msg.Payload)), "event": msg.Event, "task_id": msg.TaskID, "workflow_id": msg.WorkflowID, "node_id": msg.NodeID, "params": ch.Params}
	cp := msg
	cp.Payload = payload
	return d.deliverWebhook(ctx, ch, cp)
}

func signHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
func sanitizeHeader(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }
func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func (e *Engine) emitAuditNotifications(ctx context.Context, task *Task, ev AuditEvent) {
	if e == nil || task == nil || e.notifier == nil {
		return
	}
	wf, _ := e.workflow(task.WorkflowID)
	var node *Node
	if wf != nil && ev.NodeID != "" {
		node = wf.Nodes[ev.NodeID]
	}
	payload := map[string]any{"audit": ev, "task": taskActivitySummary(task)}
	for _, rule := range collectNotificationRules(wf, node) {
		e.emitConfiguredNotification(ctx, wf, task, node, ev.Event, rule, payload)
	}
}

func collectNotificationRules(wf *Workflow, node *Node) []NotificationRule {
	var out []NotificationRule
	if wf != nil {
		out = append(out, wf.Notifications...)
	}
	if node != nil {
		out = append(out, node.Notifications...)
	}
	return out
}

func (e *Engine) emitConfiguredNotification(ctx context.Context, wf *Workflow, task *Task, node *Node, event string, rule NotificationRule, payload any) {
	if e == nil || e.notifier == nil {
		return
	}
	if !ruleEnabled(rule.Enabled) || !matchesEvent(rule.Events, event) {
		return
	}
	facts := map[string]any{"event": event, "input": payload, "result": payload}
	if task != nil {
		facts = e.workflowFacts(task, node, payload, map[string]any{"event": event})
	}
	ok, err := e.evalNamedOrInline(rule.Condition, rule.When, facts)
	if err != nil || !ok {
		return
	}
	if !rule.Data.Empty() && wf != nil {
		if v, derr := e.applyData(ctx, rule.Data, &DataContext{Workflow: wf, Task: task, Node: node, Input: payload, Result: payload}, payload); derr == nil {
			payload = v
		}
	}
	msg := NotificationMessage{ID: newID("ntfmsg"), RuleID: rule.ID, Event: event, Title: renderTemplate(rule.Title, task, node, event), Message: renderTemplate(rule.Message, task, node, event), Severity: rule.Severity, Payload: Redact(payload), Headers: rule.Headers, CreatedAt: time.Now()}
	if task != nil {
		msg.TaskID = task.ID
		msg.WorkflowID = task.WorkflowID
	}
	if wf != nil && msg.WorkflowID == "" {
		msg.WorkflowID = wf.ID
	}
	if node != nil {
		msg.NodeID = node.ID
	}
	for _, channelID := range rule.Channels {
		ch, ok := e.notifier.Channel(channelID)
		if !ok {
			continue
		}
		cp := msg
		cp.ChannelID = channelID
		e.deliverNotification(ctx, ch, cp)
	}
}

func (e *Engine) deliverNotification(ctx context.Context, ch NotificationChannel, msg NotificationMessage) {
	delivery := NotificationDelivery{ID: newID("ntf"), MessageID: msg.ID, RuleID: msg.RuleID, ChannelID: ch.ID, Channel: ch.Type, TaskID: msg.TaskID, WorkflowID: msg.WorkflowID, NodeID: msg.NodeID, Event: msg.Event, Status: NotifyPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	attempts := ch.Retries + 1
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		delivery.Attempts++
		err = e.notifier.Deliver(ctx, ch, msg)
		if err == nil {
			delivery.Status = NotifyDelivered
			delivery.Error = ""
			break
		}
		delivery.Status = NotifyFailed
		delivery.Error = err.Error()
		if i+1 < attempts {
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}
	delivery.UpdatedAt = time.Now()
	if st, ok := e.store.(NotificationStore); ok {
		_ = st.SaveNotificationDelivery(delivery)
	}
	if err != nil && ch.Type != NotifyLog {
		log.Printf("dagflow notification failed channel=%s type=%s event=%s error=%v", ch.ID, ch.Type, msg.Event, err)
	}
}

func renderTemplate(s string, task *Task, node *Node, event string) string {
	if s == "" {
		return s
	}
	repl := map[string]string{"{{event}}": event}
	if task != nil {
		repl["{{task.id}}"] = task.ID
		repl["{{task.workflow_id}}"] = task.WorkflowID
		repl["{{task.status}}"] = string(task.Status)
	}
	if node != nil {
		repl["{{node.id}}"] = node.ID
		repl["{{node.type}}"] = string(node.Type)
	}
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}
