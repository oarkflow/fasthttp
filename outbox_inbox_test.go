package fh

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryOutboxDispatchAndInboxDedupe(t *testing.T) {
	store := NewMemoryOutboxInboxStore()
	called := 0
	outbox := NewStoredOutbox(OutboxConfig{Store: store, Dispatcher: func(ctx context.Context, msg *OutboxMessage) error {
		called++
		if msg.Topic != "email.send" {
			t.Fatalf("bad topic %s", msg.Topic)
		}
		return nil
	}})
	id, err := outbox.Publish(context.Background(), "email.send", map[string]any{"to": "a@example.com"}, map[string]string{"trace": "t1"})
	if err != nil || id == "" {
		t.Fatalf("publish id=%q err=%v", id, err)
	}
	n, err := outbox.DispatchOnce(context.Background(), 10)
	if err != nil || n != 1 || called != 1 {
		t.Fatalf("dispatch n=%d called=%d err=%v", n, called, err)
	}
	msgs, err := outbox.List(context.Background(), "done", 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("done messages=%d err=%v", len(msgs), err)
	}
	ok, err := store.BeginInbox(context.Background(), InboxMessage{ID: "evt-1"})
	if err != nil || !ok {
		t.Fatalf("inbox first ok=%v err=%v", ok, err)
	}
	ok, err = store.BeginInbox(context.Background(), InboxMessage{ID: "evt-1"})
	if !errors.Is(err, ErrDuplicateInboxMessage) || ok {
		t.Fatalf("expected duplicate, ok=%v err=%v", ok, err)
	}
}

func TestOutboxRetryThenFail(t *testing.T) {
	store := NewMemoryOutboxInboxStore()
	outbox := NewStoredOutbox(OutboxConfig{Store: store, MaxAttempts: 1, Dispatcher: func(context.Context, *OutboxMessage) error { return errors.New("boom") }})
	if _, err := outbox.Publish(context.Background(), "x", []byte(`{}`), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.DispatchOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	msgs, err := outbox.List(context.Background(), "failed", 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("failed messages=%d err=%v", len(msgs), err)
	}
}
