package ipwhitelist

import "testing"

func TestNormalizeAllowsBlockOnlyConfiguration(t *testing.T) {
	cfg, err := normalize(Config{Blocked: []string{"192.0.2.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Store != nil {
		t.Fatal("block-only configuration unexpectedly created an allowlist")
	}
	if cfg.BlockStore == nil {
		t.Fatal("block-only configuration did not create a blocklist")
	}
}
