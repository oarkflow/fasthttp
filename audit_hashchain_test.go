package fh

import (
	"context"
	"testing"
)

type captureAuditSink struct{ events []AuditEvent }

func (s *captureAuditSink) WriteAudit(ctx context.Context, e AuditEvent) error {
	s.events = append(s.events, e)
	return nil
}

func TestHashChainAuditSinkAddsHashes(t *testing.T) {
	cap := &captureAuditSink{}
	sink := NewHashChainAuditSink(cap)
	if err := sink.WriteAudit(context.Background(), AuditEvent{Action: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteAudit(context.Background(), AuditEvent{Action: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(cap.events) != 2 {
		t.Fatalf("events=%d", len(cap.events))
	}
	first := cap.events[0].Metadata["audit_hash"]
	if first == "" {
		t.Fatal("missing first hash")
	}
	if cap.events[1].Metadata["audit_prev_hash"] != first {
		t.Fatalf("hash chain mismatch")
	}
}
