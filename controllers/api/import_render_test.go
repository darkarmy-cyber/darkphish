package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderedTargetUsesExpectedPorts(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{"https://example.test/login", "example.test:443"},
		{"http://example.test/login", "example.test:80"},
		{"https://example.test:8443/login", "example.test:8443"},
	} {
		got, err := renderedTarget(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestRenderedTargetRejectsInvalidURL(t *testing.T) {
	if _, err := renderedTarget("file:///tmp/page"); err == nil {
		t.Fatal("non-http import target accepted")
	}
}

func TestBoundedBufferRejectsOverflow(t *testing.T) {
	var b boundedBuffer
	b.limit = 4
	if _, err := b.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("5")); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestChromiumExecutableHonorsConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "browser")
	if err := os.WriteFile(path, []byte("placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKPHISH_CHROMIUM_PATH", path)
	if got := chromiumExecutable(); got != path {
		t.Fatalf("got %q want %q", got, path)
	}
}

func TestRenderedImportWarningDoesNotExposeDiagnostics(t *testing.T) {
	message := renderedImportWarning(errRenderedImportUnavailable)
	if message == "" || strings.Contains(message, "/") {
		t.Fatal("fallback warning is missing or exposes implementation details")
	}
}

func TestRenderTransferBudgetFailsClosed(t *testing.T) {
	budget := newRenderTransferBudget(4)
	if err := budget.consume(4); err != nil {
		t.Fatal(err)
	}
	if err := budget.consume(1); err == nil {
		t.Fatal("render transfer budget accepted bytes past the aggregate limit")
	}
}

func TestPublicRenderedDialRejectsNonPublicDestinations(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:443",
		"10.0.0.1:443",
		"169.254.169.254:80",
		"100.64.0.1:443",
		"[::1]:443",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, err := publicRenderedDial(ctx, "tcp", address)
		cancel()
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil {
			t.Fatalf("non-public rendered destination %s was allowed", address)
		}
	}
}

func TestRenderObservationTracksMainDocumentAndIdle(t *testing.T) {
	observation := &renderObservation{frameID: "frame", loaderID: "loader"}
	observation.observe(map[string]any{
		"method": "Network.responseReceived",
		"params": map[string]any{
			"type":     "Document",
			"frameId":  "frame",
			"loaderId": "loader",
			"response": map[string]any{
				"status": float64(200),
				"url":    "https://example.test/app/",
			},
		},
	})
	observation.observe(map[string]any{
		"method": "Page.lifecycleEvent",
		"params": map[string]any{
			"frameId": "frame",
			"name":    "networkIdle",
		},
	})
	if observation.status != 200 || observation.responseURL != "https://example.test/app/" || !observation.networkIdle {
		t.Fatalf("unexpected render observation: %#v", observation)
	}
}
