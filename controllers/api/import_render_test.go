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
	if got := budget.reserve(4); got != 4 {
		t.Fatalf("reserved %d bytes, want 4", got)
	}
	if got := budget.reserve(1); got != 0 {
		t.Fatalf("reserved %d bytes past the aggregate limit", got)
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

func TestRenderTransferBudgetCapsReadsBeforeNetworkConsumption(t *testing.T) {
	budget := newRenderTransferBudget(3)
	if got := budget.reserve(32 * 1024); got != 3 {
		t.Fatalf("reservation = %d, want 3", got)
	}
	if got := budget.reserve(32 * 1024); got != 0 {
		t.Fatalf("exhausted reservation = %d, want 0", got)
	}
	if !budget.exhausted() {
		t.Fatal("budget should report exhaustion")
	}
	budget.refund(2)
	if got := budget.reserve(32 * 1024); got != 2 {
		t.Fatalf("refunded reservation = %d, want 2", got)
	}
}

func TestRenderedImportSlotsFailFastWhenFull(t *testing.T) {
	for {
		select {
		case <-renderedImportSlots:
		default:
			goto drained
		}
	}
drained:
	for i := 0; i < maxConcurrentRenderedImports; i++ {
		if !acquireRenderedImportSlot() {
			t.Fatal("expected renderer slot")
		}
	}
	if acquireRenderedImportSlot() {
		t.Fatal("renderer admitted work past concurrency limit")
	}
	for i := 0; i < maxConcurrentRenderedImports; i++ {
		releaseRenderedImportSlot()
	}
}

func TestRenderObservationTracksLatestMainDocumentNavigation(t *testing.T) {
	observation := &renderObservation{frameID: "frame", loaderID: "initial"}
	observation.observe(map[string]any{
		"method": "Network.responseReceived",
		"params": map[string]any{
			"type":     "Document",
			"frameId":  "frame",
			"loaderId": "initial",
			"response": map[string]any{"status": float64(200), "url": "https://example.test/start"},
		},
	})
	observation.loadSeen = true
	observation.networkIdle = true
	observation.observe(map[string]any{
		"method": "Network.responseReceived",
		"params": map[string]any{
			"type":     "Document",
			"frameId":  "frame",
			"loaderId": "replacement",
			"response": map[string]any{"status": float64(500), "url": "https://example.test/error"},
		},
	})
	if observation.loaderID != "replacement" || observation.status != 500 || observation.responseURL != "https://example.test/error" {
		t.Fatalf("latest navigation was not tracked: %#v", observation)
	}
	if observation.loadSeen || observation.networkIdle {
		t.Fatal("navigation state was not reset for the replacement document")
	}
}
