package signature

import (
	"testing"
	"time"
)

func TestParseTimestampSupportsUnixAndRFC3339(t *testing.T) {
	want := time.Unix(1_700_000_000, 0).UTC()
	for _, value := range []string{"1700000000", want.Format(time.RFC3339)} {
		got, err := parseTimestamp(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if !got.Equal(want) {
			t.Fatalf("parse %q = %v, want %v", value, got, want)
		}
	}
	if _, err := parseTimestamp("not-a-time"); err == nil {
		t.Fatal("invalid timestamp was accepted")
	}
}
