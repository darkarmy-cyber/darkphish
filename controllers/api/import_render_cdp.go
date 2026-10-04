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
	ws      *websocket.Conn
	nextID  int64
	pending []map[string]any
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

func dialCDP(ctx context.Context, raw string) (*cdpClient, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "ws" || parsed.Hostname() != "127.0.0.1" {
		return nil, errRenderedImportUnavailable
	}
	ws, err := websocket.Dial(raw, "", "http://127.0.0.1/")
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = ws.SetDeadline(deadline)
	}
	return &cdpClient{ws: ws}, nil
}

func (c *cdpClient) close() {
	if c != nil && c.ws != nil {
		_ = c.ws.Close()
	}
}

func (c *cdpClient) receive(ctx context.Context) (map[string]any, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.ws.SetDeadline(deadline)
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

func (c *cdpClient) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	c.nextID++
	id := c.nextID
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
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
			c.pending = append(c.pending, message)
			continue
		}
		messageID, ok := message["id"].(float64)
		if !ok || int64(messageID) != id {
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
	}
}

type renderObservation struct {
	frameID     string
	loaderID    string
	status      int
	responseURL string
	loadSeen    bool
	loadAt      time.Time
	networkIdle bool
}

func (o *renderObservation) observe(event map[string]any) {
	method, _ := event["method"].(string)
	params, _ := event["params"].(map[string]any)
	switch method {
	case "Network.responseReceived":
		eventType, _ := params["type"].(string)
		frameID, _ := params["frameId"].(string)
		loaderID, _ := params["loaderId"].(string)
		if eventType != "Document" || frameID != o.frameID || (o.loaderID != "" && loaderID != o.loaderID) {
			return
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
		if observation.networkIdle {
			return nil
		}
		if observation.loadSeen && time.Since(observation.loadAt) >= 1500*time.Millisecond {
			return nil
		}
		if time.Now().After(deadline) {
			if observation.loadSeen {
				return nil
			}
			return errRenderedImportUnavailable
		}
		readCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		event, err := client.nextEvent(readCtx)
		cancel()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		observation.observe(event)
	}
}

func renderImportPageCDP(ctx context.Context, profileDir, target string) ([]byte, *url.URL, error) {
	portCtx, cancelPort := context.WithTimeout(ctx, 5*time.Second)
	port, err := readDevToolsPort(portCtx, profileDir)
	cancelPort()
	if err != nil {
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
	defer client.close()

	for _, command := range []struct {
		method string
		params map[string]any
	}{
		{"Page.enable", nil},
		{"Network.enable", nil},
		{"Page.setLifecycleEventsEnabled", map[string]any{"enabled": true}},
	} {
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
	observation := &renderObservation{frameID: frameID, loaderID: loaderID}
	for len(client.pending) > 0 {
		event := client.pending[0]
		client.pending = client.pending[1:]
		observation.observe(event)
	}
	if err := waitForRenderedSettle(ctx, client, observation); err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	if observation.status < 200 || observation.status >= 400 {
		return nil, nil, errRenderedImportUnavailable
	}

	evaluated, err := client.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": "JSON.stringify({html:document.documentElement?document.documentElement.outerHTML:\"\",url:location.href,ready:document.readyState})",
		"returnByValue": true,
	})
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	remote, _ := evaluated["result"].(map[string]any)
	value, _ := remote["value"].(string)
	if value == "" {
		return nil, nil, errRenderedImportUnavailable
	}
	var snapshot struct {
		HTML  string `json:"html"`
		URL   string `json:"url"`
		Ready string `json:"ready"`
	}
	if err := json.Unmarshal([]byte(value), &snapshot); err != nil || snapshot.HTML == "" || snapshot.URL == "" || snapshot.Ready == "loading" {
		return nil, nil, errRenderedImportUnavailable
	}
	finalURL, err := url.Parse(snapshot.URL)
	if err != nil || finalURL.User != nil || (finalURL.Scheme != "http" && finalURL.Scheme != "https") || finalURL.Hostname() == "" {
		return nil, nil, errRenderedImportUnavailable
	}
	return []byte(snapshot.HTML), finalURL, nil
}
