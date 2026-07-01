package fh

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

type bannerAddr string

func (a bannerAddr) Network() string { return "tcp" }
func (a bannerAddr) String() string  { return string(a) }

type bannerListener struct{ addr net.Addr }

func (l bannerListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l bannerListener) Close() error              { return nil }
func (l bannerListener) Addr() net.Addr            { return l.addr }

func TestRenderStartupBanner(t *testing.T) {
	out := RenderStartupBanner(StartupBannerConfig{ASCIIArt: "-"}, StartupBannerData{
		Name:      "fh",
		Version:   "v1.0.0",
		URL:       "http://127.0.0.1:3000",
		Routes:    3,
		GoVersion: "go1.test",
		PID:       123,
		HTTP2:     true,
		Mode:      ModeProduction,
	})
	for _, want := range []string{"+", "Name", "fh v1.0.0", "URL", "Routes", "3", "HTTP/2", "enabled"} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestStartupBannerWritesToConfiguredWriter(t *testing.T) {
	var buf bytes.Buffer
	app := New(WithStartupBanner(StartupBannerConfig{Writer: &buf, ASCIIArt: "-", Name: "demo", Version: "v1"}))
	app.Get("/", func(c Ctx) error { return c.SendString("ok") })
	app.printStartupBanner(bannerListener{addr: bannerAddr(":3000")})
	out := buf.String()
	if !strings.Contains(out, "demo v1") || !strings.Contains(out, "http://127.0.0.1:3000") || !strings.Contains(out, "Routes") {
		t.Fatalf("unexpected banner output:\n%s", out)
	}
}

func TestStartupBannerDisabled(t *testing.T) {
	var buf bytes.Buffer
	app := New(WithStartupBanner(StartupBannerConfig{Writer: &buf, Disabled: true}))
	app.printStartupBanner(bannerListener{addr: bannerAddr(":3000")})
	if buf.Len() != 0 {
		t.Fatalf("expected disabled banner to write nothing, got %q", buf.String())
	}
}

func TestStartupURLNormalizesWildcardAddresses(t *testing.T) {
	cases := map[string]string{
		":8080":        "http://127.0.0.1:8080",
		"0.0.0.0:9000": "http://127.0.0.1:9000",
		"[::]:7000":    "http://127.0.0.1:7000",
	}
	for in, want := range cases {
		if got := startupURL("http", in); got != want {
			t.Fatalf("startupURL(%q)=%q want %q", in, got, want)
		}
	}
}
