package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
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

func TestRenderedHeaderBytesChargesRepeatedNamesPerValue(t *testing.T) {
	header := http.Header{"Set-Cookie": {"a=1", "b=2", "c=3"}}
	want := 2 + 3*(len("Set-Cookie")+len("a=1")+4)
	if got := renderedHeaderBytes(header); got != want {
		t.Fatalf("header bytes = %d, want %d", got, want)
	}
}

func TestRenderedRequestHeadersAreCappedAndCharged(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://example.test/page", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("X-Test", "one")
	request.Header.Add("X-Test", "two")
	headerBytes := renderedRequestHeaderBytes(request)
	budget := newRenderTransferBudget(int64(headerBytes))
	if got := budget.reserve(headerBytes); got != headerBytes || !budget.exhausted() {
		t.Fatalf("request headers were not charged: reserved=%d exhausted=%v", got, budget.exhausted())
	}
	request.Header.Set("X-Large", strings.Repeat("x", maxRenderedRequestHeaderBytes))
	if got := renderedRequestHeaderBytes(request); got <= maxRenderedRequestHeaderBytes {
		t.Fatalf("oversized request headers counted as %d bytes", got)
	}
}

func TestRenderedProxyRejectsOversizedRequestHeadersBeforeForwarding(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/page", nil)
	request.Header.Set("X-Large", strings.Repeat("x", maxRenderedRequestHeaderBytes))
	recorder := httptest.NewRecorder()
	handleRenderedProxyRequest(context.Background(), newRenderTransferBudget(maxRenderedTransferBytes), recorder, request)
	if recorder.Code != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("oversized request headers returned %d", recorder.Code)
	}
}

func TestRenderedHTTPRejectsWebSocketUpgradeAndStripsHopHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/socket", nil)
	request.Header.Set("Connection", "keep-alive, X-Remove")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("X-Remove", "secret")
	recorder := httptest.NewRecorder()
	handleRenderedHTTP(context.Background(), newRenderTransferBudget(maxRenderedTransferBytes), recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("WebSocket upgrade returned %d", recorder.Code)
	}
	removeRenderedHopByHopHeaders(request.Header)
	for _, key := range []string{"Connection", "Upgrade", "X-Remove"} {
		if request.Header.Get(key) != "" {
			t.Fatalf("hop-by-hop header %s was retained", key)
		}
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

func TestPausedRequestCommandEnforcesHTTPSAndSafeMethods(t *testing.T) {
	event := func(method, rawURL string) map[string]any {
		return map[string]any{
			"params": map[string]any{
				"requestId": "request-1",
				"request": map[string]any{
					"method": method,
					"url":    rawURL,
				},
			},
		}
	}
	method, _, err := pausedRequestCommand(event(http.MethodGet, "https://example.test/page"), true)
	if err != nil || method != "Fetch.continueRequest" {
		t.Fatalf("HTTPS GET should continue: method=%s err=%v", method, err)
	}
	method, _, err = pausedRequestCommand(event(http.MethodGet, "http://example.test/page"), true)
	if err != nil || method != "Fetch.failRequest" {
		t.Fatalf("HTTP downgrade should be blocked: method=%s err=%v", method, err)
	}
	method, _, err = pausedRequestCommand(event(http.MethodPost, "https://example.test/submit"), true)
	if err != nil || method != "Fetch.failRequest" {
		t.Fatalf("state-changing method should be blocked: method=%s err=%v", method, err)
	}
}

func TestPausedRequestCommandRejectsWebSockets(t *testing.T) {
	event := func(resourceType string, headers map[string]any) map[string]any {
		return map[string]any{
			"params": map[string]any{
				"requestId":    "request-1",
				"resourceType": resourceType,
				"request": map[string]any{
					"method":  http.MethodGet,
					"url":     "https://example.test/socket",
					"headers": headers,
				},
			},
		}
	}
	for _, candidate := range []map[string]any{
		event("WebSocket", nil),
		event("Other", map[string]any{"upgrade": "WebSocket"}),
	} {
		method, _, err := pausedRequestCommand(candidate, true)
		if err != nil || method != "Fetch.failRequest" {
			t.Fatalf("WebSocket request was not blocked: method=%s err=%v", method, err)
		}
	}
}

func TestChildTargetsEnableInterceptionBeforeResume(t *testing.T) {
	commands := childTargetCommands()
	if len(commands) != 3 || commands[0].method != "Target.setAutoAttach" || commands[1].method != "Fetch.enable" || commands[2].method != "Runtime.runIfWaitingForDebugger" {
		t.Fatalf("unexpected child-target policy commands: %#v", commands)
	}
	autoAttach := commands[0].params
	if autoAttach["autoAttach"] != true || autoAttach["waitForDebuggerOnStart"] != true || autoAttach["flatten"] != true {
		t.Fatalf("child targets are not attached paused with flat sessions: %#v", autoAttach)
	}
}

func TestChildTargetFetchFailureDoesNotResume(t *testing.T) {
	methods := make(chan []string, 1)
	server := httptest.NewServer(websocket.Handler(func(connection *websocket.Conn) {
		seen := make([]string, 0, 2)
		defer func() { methods <- seen }()
		for len(seen) < 2 {
			var request map[string]any
			if err := websocket.JSON.Receive(connection, &request); err != nil {
				return
			}
			method, _ := request["method"].(string)
			seen = append(seen, method)
			response := map[string]any{"id": request["id"], "sessionId": request["sessionId"]}
			if method == "Fetch.enable" {
				response["error"] = map[string]any{"message": "unsupported"}
			} else {
				response["result"] = map[string]any{}
			}
			_ = websocket.JSON.Send(connection, response)
		}
	}))
	defer server.Close()
	connection, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", "http://127.0.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	client := &cdpClient{ws: connection, ignored: make(map[int64]struct{})}
	err = client.handleAttachedTarget(context.Background(), map[string]any{
		"method": "Target.attachedToTarget",
		"params": map[string]any{"sessionId": "child-session"},
	})
	_ = connection.Close()
	if err == nil {
		t.Fatal("child target resumed after Fetch.enable failed")
	}
	seen := <-methods
	if len(seen) != 2 || seen[0] != "Target.setAutoAttach" || seen[1] != "Fetch.enable" {
		t.Fatalf("unexpected child commands after interception failure: %#v", seen)
	}
}

func TestPendingReplacementNavigationIsDrainedBeforeSettlement(t *testing.T) {
	observation := &renderObservation{
		frameID:     "frame",
		loaderID:    "initial",
		status:      http.StatusOK,
		loadSeen:    true,
		loadAt:      time.Now().Add(-2 * time.Second),
		networkIdle: true,
	}
	client := &cdpClient{pending: []map[string]any{{
		"method": "Network.responseReceived",
		"params": map[string]any{
			"type":     "Document",
			"frameId":  "frame",
			"loaderId": "replacement",
			"response": map[string]any{"status": float64(500), "url": "https://example.test/error"},
		},
	}}}
	if err := drainPendingRenderEvents(context.Background(), client, observation); err != nil {
		t.Fatal(err)
	}
	if observation.loaderID != "replacement" || observation.status != 500 || observation.loadSeen {
		t.Fatalf("queued replacement navigation was not applied: %#v", observation)
	}
}

func TestRuntimeCallQueuesSocketNavigationForPostSnapshotValidation(t *testing.T) {
	server := httptest.NewServer(websocket.Handler(func(connection *websocket.Conn) {
		var request map[string]any
		if err := websocket.JSON.Receive(connection, &request); err != nil {
			return
		}
		_ = websocket.JSON.Send(connection, map[string]any{
			"id":     request["id"],
			"result": map[string]any{},
		})
		_ = websocket.JSON.Send(connection, map[string]any{
			"method": "Network.responseReceived",
			"params": map[string]any{
				"type":     "Document",
				"frameId":  "frame",
				"loaderId": "replacement",
				"response": map[string]any{"status": float64(500), "url": "https://example.test/error"},
			},
		})
		<-time.After(time.Second)
	}))
	defer server.Close()
	connection, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", "http://127.0.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := &cdpClient{ws: connection, ignored: make(map[int64]struct{})}
	if _, err := client.call(context.Background(), "Runtime.evaluate", nil); err != nil {
		t.Fatal(err)
	}
	observation := &renderObservation{frameID: "frame", loaderID: "initial", status: http.StatusOK, loadSeen: true}
	if err := waitForRenderQuiescence(context.Background(), client, observation, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if observation.loaderID != "replacement" || observation.status != 500 {
		t.Fatalf("socket-queued navigation was not revalidated: %#v", observation)
	}
}

func TestRenderedSettleReadsSocketBeforeBoundaryAcceptance(t *testing.T) {
	server := httptest.NewServer(websocket.Handler(func(connection *websocket.Conn) {
		_ = websocket.JSON.Send(connection, map[string]any{
			"method": "Network.responseReceived",
			"params": map[string]any{
				"type":     "Document",
				"frameId":  "frame",
				"loaderId": "replacement",
				"response": map[string]any{"status": float64(500), "url": "https://example.test/error"},
			},
		})
		_ = websocket.JSON.Send(connection, map[string]any{"method": "Page.loadEventFired", "params": map[string]any{}})
		<-time.After(3 * time.Second)
	}))
	defer server.Close()
	connection, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", "http://127.0.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := &cdpClient{ws: connection, ignored: make(map[int64]struct{})}
	observation := &renderObservation{
		frameID: "frame", loaderID: "initial", status: http.StatusOK,
		loadSeen: true, loadAt: time.Now().Add(-2 * time.Second),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForRenderedSettle(ctx, client, observation); err != nil {
		t.Fatal(err)
	}
	if observation.loaderID != "replacement" || observation.status != 500 {
		t.Fatalf("socket-ready replacement navigation was skipped: %#v", observation)
	}
}

func TestRenderedSettleQuietTimeoutDoesNotExpireSubsequentWrites(t *testing.T) {
	server := httptest.NewServer(websocket.Handler(func(connection *websocket.Conn) {
		_ = websocket.JSON.Send(connection, map[string]any{"method": "Page.loadEventFired", "params": map[string]any{}})
		var request map[string]any
		if err := websocket.JSON.Receive(connection, &request); err != nil {
			return
		}
		_ = websocket.JSON.Send(connection, map[string]any{
			"id": request["id"], "result": map[string]any{"value": "ok"},
		})
	}))
	defer server.Close()
	connection, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", "http://127.0.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := &cdpClient{ws: connection, ignored: make(map[int64]struct{})}
	observation := &renderObservation{
		frameID: "frame", loaderID: "loader", status: http.StatusOK,
		loadSeen: true, loadAt: time.Now().Add(-2 * time.Second),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForRenderedSettle(ctx, client, observation); err != nil {
		t.Fatal(err)
	}
	result, err := client.call(ctx, "Runtime.evaluate", nil)
	if err != nil {
		t.Fatalf("CDP call after settle quiet timeout failed: %v", err)
	}
	if result["value"] != "ok" {
		t.Fatalf("unexpected post-settle response: %#v", result)
	}
}

func TestRendererSecurityCapsAndSetupCommands(t *testing.T) {
	if maxRenderedCDPPayloadBytes < maxImportedPageBytes*7 {
		t.Fatalf("CDP cap %d does not cover worst-case double escaping", maxRenderedCDPPayloadBytes)
	}
	if maxRenderedMemoryBytes != 512<<20 {
		t.Fatalf("unexpected per-render memory cap: %d", maxRenderedMemoryBytes)
	}
	commands := renderSetupCommands()
	for _, command := range commands {
		if command.method == "Page.navigate" {
			t.Fatal("navigation must run only after renderer security setup")
		}
	}
	browserCommands := browserSetupCommands()
	if len(browserCommands) != 1 || browserCommands[0].method != "Browser.setDownloadBehavior" || browserCommands[0].params["behavior"] != "deny" {
		t.Fatal("download denial is missing from pre-navigation setup")
	}
}

func TestBrowserWebSocketURLUsesProfileEndpoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte("9222\n/devtools/browser/test-id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := browserWebSocketURL(dir, 9222)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/test-id" {
		t.Fatalf("browser websocket URL = %q", got)
	}
}

func TestRenderObservationFlagsIntermediateHTTPSDowngrade(t *testing.T) {
	observation := &renderObservation{frameID: "frame", loaderID: "loader", originalHTTPS: true}
	observation.observe(map[string]any{
		"method": "Network.requestWillBeSent",
		"params": map[string]any{
			"type":    "Document",
			"frameId": "frame",
			"request": map[string]any{
				"url": "http://example.test/intermediate",
			},
		},
	})
	if !observation.insecureDowngrade {
		t.Fatal("intermediate HTTP navigation was not flagged")
	}
}
