package fh

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdempotencyStoreReplayAndConflict(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenIdempotencyStore(dir+"/idem.jsonl", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	decision, _, err := store.Begin("key-1", "hash-a", "POST", "/orders")
	if err != nil {
		t.Fatal(err)
	}
	if decision != idemNew {
		t.Fatalf("expected new, got %v", decision)
	}
	if err := store.Complete("key-1", "hash-a", 201, "application/json", map[string][]string{"X-Test": {"ok"}}, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}

	decision, rec, err := store.Begin("key-1", "hash-a", "POST", "/orders")
	if err != nil {
		t.Fatal(err)
	}
	if decision != idemReplay {
		t.Fatalf("expected replay, got %v", decision)
	}
	if rec.StatusCode != 201 || string(rec.Response) != `{"ok":true}` {
		t.Fatalf("bad replay record: %#v", rec)
	}

	decision, _, err = store.Begin("key-1", "hash-b", "POST", "/orders")
	if err != nil {
		t.Fatal(err)
	}
	if decision != idemConflict {
		t.Fatalf("expected conflict, got %v", decision)
	}
}

func TestDurableQueueProcessesJob(t *testing.T) {
	q, err := OpenDurableQueue(DurableQueueConfig{Dir: t.TempDir(), Workers: 1, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	var processed atomic.Int32
	q.Register("email", func(ctx context.Context, job *QueueJob) error {
		var payload map[string]string
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		if payload["to"] != "user@example.com" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		processed.Add(1)
		return nil
	})
	if err := q.Start(); err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if _, err := q.Enqueue("email", map[string]string{"to": "user@example.com"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if processed.Load() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	st, _ := q.Stats()
	t.Fatalf("job was not processed; stats=%+v", st)
}

func TestRequestJournalAppend(t *testing.T) {
	path := t.TempDir() + "/journal.jsonl"
	j, err := OpenRequestJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(RequestJournalEntry{RequestID: "req_test", Event: "received"}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty journal")
	}
}

func TestDurableQueueEventLog(t *testing.T) {
	dir := t.TempDir()
	q, err := OpenDurableQueue(DurableQueueConfig{Dir: dir, Workers: 1, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	q.Register("event_test", func(ctx context.Context, job *QueueJob) error { return nil })
	if _, err := q.Enqueue("event_test", map[string]string{"hello": "world"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
		if strings.Contains(string(b), "enqueued") && strings.Contains(string(b), "completed") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	t.Fatalf("queue event log was not updated: %s", b)
}
