package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

type cdpClient struct {
	ws           *websocket.Conn
	nextID       int64
	pending      []map[string]any
	ignored      map[int64]struct{}
	requireHTTPS bool
}

const maxRenderedCDPPayloadBytes = maxImportedPageBytes*8 + (1 << 20)

type cdpCommand struct {
	method string
	params map[string]any
}

func childTargetCommands() []cdpCommand {
	return []cdpCommand{
		{"Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}},
		{"Fetch.enable", map[string]any{"patterns": []map[string]any{{"urlPattern": "*", "requestStage": "Request"}}}},
		{"Runtime.runIfWaitingForDebugger", nil},
	}
}

func renderSetupCommands() []cdpCommand {
	return []cdpCommand{
		{"Page.enable", nil},
		{"Network.enable", nil},
		{"Fetch.enable", map[string]any{"patterns": []map[string]any{{"urlPattern": "*", "requestStage": "Request"}}}},
		{"Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}},
		{"Page.setLifecycleEventsEnabled", map[string]any{"enabled": true}},
	}
}

func browserSetupCommands() []cdpCommand {
	return []cdpCommand{{"Browser.setDownloadBehavior", map[string]any{"behavior": "deny"}}}
}

func readDevToolsPort(ctx context.Context, profileDir string) (int, error) {
	path := filepath.Join(profileDir, "DevToolsActivePort")
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 1 {
				port, parseErr := strconv.Atoi(strings.TrimSpace(lines[0]))
				if parseErr == nil && port > 0 && port <= 65535 {
					return port, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

func pageWebSocketURL(ctx context.Context, port int) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errRenderedImportUnavailable
	}
	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", err
	}
	for _, target := range targets {
		if target.Type == "page" && strings.HasPrefix(target.WebSocketDebuggerURL, "ws://127.0.0.1:") {
			return target.WebSocketDebuggerURL, nil
		}
	}
	return "", errRenderedImportUnavailable
}

func browserWebSocketURL(profileDir string, port int) (string, error) {
	data, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort"))
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return "", errRenderedImportUnavailable
	}
	path := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(path, "/devtools/browser/") || strings.ContainsAny(path, "?#") {
		return "", errRenderedImportUnavailable
	}
	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, path), nil
}

func denyBrowserDownloads(ctx context.Context, profileDir string, port int) error {
	wsURL, err := browserWebSocketURL(profileDir, port)
	if err != nil {
		return err
	}
	client, err := dialCDP(ctx, wsURL)
	if err != nil {
		return err
	}
	defer client.close()
	for _, command := range browserSetupCommands() {
		if _, err := client.call(ctx, command.method, command.params); err != nil {
			return err
		}
	}
	return nil
}

func dialCDP(ctx context.Context, raw string) (*cdpClient, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "ws" || parsed.Hostname() != "127.0.0.1" {
		return nil, errRenderedImportUnavailable
	}
	ws, err := websocket.Dial(raw, "", "http://127.0.0.1/")
	if err != nil {
		return nil, err
	}
	ws.MaxPayloadBytes = maxRenderedCDPPayloadBytes
	if deadline, ok := ctx.Deadline(); ok {
		_ = ws.SetDeadline(deadline)
	}
	return &cdpClient{ws: ws, ignored: make(map[int64]struct{})}, nil
}

func (c *cdpClient) close() {
	if c != nil && c.ws != nil {
		_ = c.ws.Close()
	}
}

func (c *cdpClient) receive(ctx context.Context) (map[string]any, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.ws.SetReadDeadline(deadline)
	} else {
		_ = c.ws.SetReadDeadline(time.Time{})
	}
	var raw string
	if err := websocket.Message.Receive(c.ws, &raw); err != nil {
		return nil, err
	}
	var message map[string]any
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return nil, err
	}
	return message, nil
}

func (c *cdpClient) sendNoWaitSession(method string, params map[string]any, sessionID string) error {
	c.nextID++
	id := c.nextID
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	if err := websocket.JSON.Send(c.ws, request); err != nil {
		return err
	}
	c.ignored[id] = struct{}{}
	return nil
}

func pausedRequestCommand(event map[string]any, requireHTTPS bool) (string, map[string]any, error) {
	params, _ := event["params"].(map[string]any)
	requestID, _ := params["requestId"].(string)
	request, _ := params["request"].(map[string]any)
	method, _ := request["method"].(string)
	rawURL, _ := request["url"].(string)
	resourceType, _ := params["resourceType"].(string)
	if requestID == "" {
		return "", nil, errRenderedImportUnavailable
	}
	parsed, err := url.Parse(rawURL)
	allowedScheme := parsed.Scheme == "http" || parsed.Scheme == "https"
	if requireHTTPS {
		allowedScheme = parsed.Scheme == "https"
	}
	if err != nil || parsed.Hostname() == "" || !allowedScheme || isWebSocketRequest(resourceType, request) ||
		(method != http.MethodGet && method != http.MethodHead) {
		return "Fetch.failRequest", map[string]any{"requestId": requestID, "errorReason": "BlockedByClient"}, nil
	}
	return "Fetch.continueRequest", map[string]any{"requestId": requestID}, nil
}

func isWebSocketRequest(resourceType string, request map[string]any) bool {
	if strings.EqualFold(resourceType, "WebSocket") {
		return true
	}
	headers, _ := request["headers"].(map[string]any)
	for key, value := range headers {
		if strings.EqualFold(key, "Upgrade") && strings.EqualFold(fmt.Sprint(value), "websocket") {
			return true
		}
	}
	return false
}

func (c *cdpClient) handlePausedRequest(event map[string]any) error {
	method, params, err := pausedRequestCommand(event, c.requireHTTPS)
	if err != nil {
		return err
	}
	sessionID, _ := event["sessionId"].(string)
	return c.sendNoWaitSession(method, params, sessionID)
}

func (c *cdpClient) handleAttachedTarget(ctx context.Context, event map[string]any) error {
	params, _ := event["params"].(map[string]any)
	sessionID, _ := params["sessionId"].(string)
	if sessionID == "" {
		return errRenderedImportUnavailable
	}
	for _, command := range childTargetCommands() {
		if _, err := c.callSession(ctx, command.method, command.params, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func drainPendingRenderEvents(ctx context.Context, client *cdpClient, observation *renderObservation) error {
	for len(client.pending) > 0 {
		event := client.pending[0]
		client.pending = client.pending[1:]
		handled, err := client.handleImmediateEvent(ctx, event)
		if err != nil {
			return err
		}
		if !handled {
			observation.observe(event)
		}
	}
	return nil
}

func (c *cdpClient) handleImmediateEvent(ctx context.Context, event map[string]any) (bool, error) {
	method, _ := event["method"].(string)
	switch method {
	case "Fetch.requestPaused":
		return true, c.handlePausedRequest(event)
	case "Target.attachedToTarget":
		return true, c.handleAttachedTarget(ctx, event)
	default:
		return false, nil
	}
}

func (c *cdpClient) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	return c.callSession(ctx, method, params, "")
}

func (c *cdpClient) callSession(ctx context.Context, method string, params map[string]any, sessionID string) (map[string]any, error) {
	c.nextID++
	id := c.nextID
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	if err := websocket.JSON.Send(c.ws, request); err != nil {
		return nil, err
	}
	for {
		message, err := c.receive(ctx)
		if err != nil {
			return nil, err
		}
		if eventMethod, ok := message["method"].(string); ok && eventMethod != "" {
			if eventMethod == "Fetch.requestPaused" {
				if err := c.handlePausedRequest(message); err != nil {
					return nil, err
				}
			} else {
				// Attached targets start paused. Queue their setup rather than nesting
				// calls, which would make response correlation ambiguous.
				c.pending = append(c.pending, message)
			}
			continue
		}
		messageID, ok := message["id"].(float64)
		if !ok {
			continue
		}
		responseID := int64(messageID)
		if _, ignored := c.ignored[responseID]; ignored {
			delete(c.ignored, responseID)
			continue
		}
		if responseID != id {
			continue
		}
		if protocolErr, ok := message["error"].(map[string]any); ok {
			return nil, fmt.Errorf("cdp %s failed: %v", method, protocolErr["message"])
		}
		result, _ := message["result"].(map[string]any)
		return result, nil
	}
}

func (c *cdpClient) nextEvent(ctx context.Context) (map[string]any, error) {
	if len(c.pending) > 0 {
		event := c.pending[0]
		c.pending = c.pending[1:]
		return event, nil
	}
	for {
		message, err := c.receive(ctx)
		if err != nil {
			return nil, err
		}
		if _, ok := message["method"].(string); ok {
			return message, nil
		}
		if messageID, ok := message["id"].(float64); ok {
			responseID := int64(messageID)
			delete(c.ignored, responseID)
		}
	}
}

type renderObservation struct {
	frameID           string
	loaderID          string
	status            int
	responseURL       string
	loadSeen          bool
	loadAt            time.Time
	networkIdle       bool
	originalHTTPS     bool
	insecureDowngrade bool
}

func (o *renderObservation) observe(event map[string]any) {
	method, _ := event["method"].(string)
	params, _ := event["params"].(map[string]any)
	switch method {
	case "Network.requestWillBeSent":
		eventType, _ := params["type"].(string)
		frameID, _ := params["frameId"].(string)
		if eventType == "Document" && frameID == o.frameID && o.originalHTTPS {
			request, _ := params["request"].(map[string]any)
			rawURL, _ := request["url"].(string)
			if parsed, err := url.Parse(rawURL); err == nil && strings.EqualFold(parsed.Scheme, "http") {
				o.insecureDowngrade = true
			}
			if redirect, ok := params["redirectResponse"].(map[string]any); ok {
				redirectURL, _ := redirect["url"].(string)
				if parsed, err := url.Parse(redirectURL); err == nil && strings.EqualFold(parsed.Scheme, "http") {
					o.insecureDowngrade = true
				}
			}
		}
	case "Network.responseReceived":
		eventType, _ := params["type"].(string)
		frameID, _ := params["frameId"].(string)
		loaderID, _ := params["loaderId"].(string)
		if eventType != "Document" || frameID != o.frameID {
			return
		}
		if loaderID != "" && loaderID != o.loaderID {
			o.loaderID = loaderID
			o.status = 0
			o.responseURL = ""
			o.loadSeen = false
			o.networkIdle = false
		}
		response, _ := params["response"].(map[string]any)
		if status, ok := response["status"].(float64); ok {
			o.status = int(status)
		}
		o.responseURL, _ = response["url"].(string)
	case "Page.loadEventFired":
		o.loadSeen = true
		o.loadAt = time.Now()
	case "Page.lifecycleEvent":
		frameID, _ := params["frameId"].(string)
		name, _ := params["name"].(string)
		if frameID == o.frameID && name == "networkIdle" {
			o.networkIdle = true
		}
	}
}

func waitForRenderedSettle(ctx context.Context, client *cdpClient, observation *renderObservation) error {
	deadline := time.Now().Add(6 * time.Second)
	for {
		if err := drainPendingRenderEvents(ctx, client, observation); err != nil {
			return errRenderedImportUnavailable
		}
		settled := observation.loadSeen && time.Since(observation.loadAt) >= 1500*time.Millisecond
		expired := time.Now().After(deadline)
		if expired && !observation.loadSeen {
			return errRenderedImportUnavailable
		}
		readWindow := 300 * time.Millisecond
		if settled || expired {
			readWindow = 50 * time.Millisecond
		}
		readCtx, cancel := context.WithTimeout(ctx, readWindow)
		event, err := client.nextEvent(readCtx)
		cancel()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if settled || expired {
					return nil
				}
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errRenderedImportUnavailable
		}
		handled, err := client.handleImmediateEvent(ctx, event)
		if err != nil {
			return errRenderedImportUnavailable
		}
		if !handled {
			observation.observe(event)
		}
	}
}

func waitForRenderQuiescence(ctx context.Context, client *cdpClient, observation *renderObservation, window time.Duration) error {
	for {
		if err := drainPendingRenderEvents(ctx, client, observation); err != nil {
			return err
		}
		readCtx, cancel := context.WithTimeout(ctx, window)
		event, err := client.nextEvent(readCtx)
		cancel()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return nil
			}
			return err
		}
		handled, err := client.handleImmediateEvent(ctx, event)
		if err != nil {
			return err
		}
		if !handled {
			observation.observe(event)
		}
	}
}

func renderImportPageCDP(ctx context.Context, profileDir, target string) ([]byte, *url.URL, error) {
	portCtx, cancelPort := context.WithTimeout(ctx, 5*time.Second)
	port, err := readDevToolsPort(portCtx, profileDir)
	cancelPort()
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	if err := denyBrowserDownloads(ctx, profileDir, port); err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	wsCtx, cancelWS := context.WithTimeout(ctx, 5*time.Second)
	wsURL, err := pageWebSocketURL(wsCtx, port)
	cancelWS()
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	client, err := dialCDP(ctx, wsURL)
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	originalURLForObservation, _ := url.Parse(target)
	client.requireHTTPS = originalURLForObservation != nil && strings.EqualFold(originalURLForObservation.Scheme, "https")
	defer client.close()

	for _, command := range renderSetupCommands() {
		if _, err := client.call(ctx, command.method, command.params); err != nil {
			return nil, nil, errRenderedImportUnavailable
		}
	}

	navigation, err := client.call(ctx, "Page.navigate", map[string]any{"url": target})
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	if errorText, _ := navigation["errorText"].(string); errorText != "" {
		return nil, nil, errRenderedImportUnavailable
	}
	frameID, _ := navigation["frameId"].(string)
	loaderID, _ := navigation["loaderId"].(string)
	if frameID == "" {
		return nil, nil, errRenderedImportUnavailable
	}
	observation := &renderObservation{frameID: frameID, loaderID: loaderID, originalHTTPS: client.requireHTTPS}
	if err := drainPendingRenderEvents(ctx, client, observation); err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	if err := waitForRenderedSettle(ctx, client, observation); err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	if observation.insecureDowngrade || observation.status < 200 || observation.status >= 300 {
		return nil, nil, errRenderedImportUnavailable
	}

	evaluated, err := client.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    "(()=>{const html=document.documentElement?document.documentElement.outerHTML:\"\";const tooLarge=html.length>8388608;return {html:tooLarge?\"\":html,url:location.href,ready:document.readyState,tooLarge:tooLarge}})()",
		"returnByValue": true,
	})
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	// Runtime.evaluate's response is ordered after navigation events already emitted
	// by Chromium. Re-apply those events before accepting the captured document.
	if err := waitForRenderQuiescence(ctx, client, observation, 50*time.Millisecond); err != nil ||
		observation.insecureDowngrade || observation.status < 200 || observation.status >= 300 {
		return nil, nil, errRenderedImportUnavailable
	}
	remote, _ := evaluated["result"].(map[string]any)
	value, _ := remote["value"].(map[string]any)
	htmlValue, _ := value["html"].(string)
	urlValue, _ := value["url"].(string)
	readyValue, _ := value["ready"].(string)
	tooLarge, _ := value["tooLarge"].(bool)
	if value == nil || tooLarge || htmlValue == "" || urlValue == "" || readyValue == "loading" {
		return nil, nil, errRenderedImportUnavailable
	}
	finalURL, err := url.Parse(urlValue)
	originalURL, originalErr := url.Parse(target)
	if err != nil || originalErr != nil || finalURL.User != nil || (finalURL.Scheme != "http" && finalURL.Scheme != "https") || finalURL.Hostname() == "" {
		return nil, nil, errRenderedImportUnavailable
	}
	if strings.EqualFold(originalURL.Scheme, "https") && !strings.EqualFold(finalURL.Scheme, "https") {
		return nil, nil, errRenderedImportUnavailable
	}
	return []byte(htmlValue), finalURL, nil
}
