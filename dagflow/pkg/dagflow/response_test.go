package dagflow

import "testing"

func TestNormalizeInspectObjectWithBracketScalarReturnsJSONObject(t *testing.T) {
	input := "{to: null, subject: [script] null, body: {to: a@b.com, subject: Hi, body: Hello}, enriched: true, app_env: null, tenant_config: null}"

	got := publicPayload(input)

	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T: %#v", got, got)
	}

	if _, wrapped := obj["data"]; wrapped {
		t.Fatalf("must not wrap parsed inspect object inside data: %#v", obj)
	}

	if obj["to"] != nil {
		t.Fatalf("expected to=null, got %#v", obj["to"])
	}

	if obj["subject"] != "[script] null" {
		t.Fatalf("expected subject scalar, got %#v", obj["subject"])
	}

	body, ok := obj["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body object, got %T: %#v", obj["body"], obj["body"])
	}

	if body["to"] != "a@b.com" {
		t.Fatalf("expected nested body.to, got %#v", body["to"])
	}

	if obj["enriched"] != true {
		t.Fatalf("expected enriched=true, got %#v", obj["enriched"])
	}
}

func TestParseInterpreterInspectMapAsPublicPayload(t *testing.T) {
	in := `{to: null, subject: [script] null, body: {to: a@b.com, subject: Hi, body: Hello}, enriched: true, app_env: null, tenant_config: null}`
	out, ok := publicPayload(in).(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", publicPayload(in))
	}
	if out["enriched"] != true {
		t.Fatalf("expected enriched true, got %#v", out["enriched"])
	}
	body, ok := out["body"].(map[string]any)
	if !ok || body["to"] != "a@b.com" {
		t.Fatalf("expected nested body object, got %#v", out["body"])
	}
}

func TestPublicPayloadWrapsScalars(t *testing.T) {
	if _, ok := publicPayload("hello").(map[string]any); !ok {
		t.Fatal("string response must be wrapped as object")
	}
	arr, ok := publicPayload([]any{"x", map[string]any{"ok": true}}).([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected array, got %#v", publicPayload([]any{"x"}))
	}
	if _, ok := arr[0].(map[string]any); !ok {
		t.Fatalf("scalar array element must be wrapped as object, got %#v", arr[0])
	}
}
