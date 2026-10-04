package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

var errRenderedImportUnavailable = errors.New("rendered site import unavailable")

const maxRenderedTransferBytes int64 = 32 << 20

func chromiumExecutable() string {
	if configured := strings.TrimSpace(os.Getenv("DARKPHISH_CHROMIUM_PATH")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

type boundedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len()+len(p) > b.limit {
		return 0, errRenderedImportUnavailable
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type renderTransferBudget struct {
	mu        sync.Mutex
	remaining int64
}

func newRenderTransferBudget(limit int64) *renderTransferBudget {
	return &renderTransferBudget{remaining: limit}
}

func (b *renderTransferBudget) consume(n int) error {
	if n <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if int64(n) > b.remaining {
		b.remaining = 0
		return errRenderedImportUnavailable
	}
	b.remaining -= int64(n)
	return nil
}

type budgetReader struct {
	reader io.Reader
	budget *renderTransferBudget
}

func (r *budgetReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		if budgetErr := r.budget.consume(n); budgetErr != nil {
			return 0, budgetErr
		}
	}
	return n, err
}

func startRenderedImportProxy(ctx context.Context, budget *renderTransferBudget) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       15 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleRenderedConnect(ctx, budget, w, r)
				return
			}
			handleRenderedHTTP(ctx, budget, w, r)
		}),
	}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	go func() { _ = server.Serve(listener) }()
	closeFn := func() { _ = server.Close() }
	return "http://" + listener.Addr().String(), closeFn, nil
}

func renderedTarget(raw string) (string, error) {
	parsed, err := parseImportURL(raw)
	if err != nil {
		return "", err
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(parsed.Hostname(), port), nil
}

func publicRenderedDial(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errRenderedImportUnavailable
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errRenderedImportUnavailable
	}
	var candidates []netip.Addr
	if literal, parseErr := netip.ParseAddr(strings.Trim(host, "[]")); parseErr == nil {
		candidates = []netip.Addr{literal.Unmap()}
	} else {
		resolved, resolveErr := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if resolveErr != nil || len(resolved) == 0 {
			return nil, errRenderedImportUnavailable
		}
		for _, ip := range resolved {
			candidates = append(candidates, ip.Unmap())
		}
	}
	for _, ip := range candidates {
		if !dialer.IsPublicAddress(ip) {
			return nil, errRenderedImportUnavailable
		}
	}
	var lastErr error
	for _, ip := range candidates {
		dialNetwork := "tcp6"
		if ip.Is4() {
			dialNetwork = "tcp4"
		}
		conn, dialErr := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}).DialContext(
			ctx, dialNetwork, net.JoinHostPort(ip.String(), port),
		)
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errRenderedImportUnavailable
}

func handleRenderedConnect(ctx context.Context, budget *renderTransferBudget, w http.ResponseWriter, r *http.Request) {
	target, err := renderedTarget("https://" + r.Host)
	if err != nil {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	upstream, err := publicRenderedDial(ctx, "tcp", target)
	if err != nil {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "proxy unavailable", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go func() {
		defer client.Close()
		defer upstream.Close()
		_, _ = io.Copy(upstream, &budgetReader{reader: client, budget: budget})
	}()
	go func() {
		defer client.Close()
		defer upstream.Close()
		_, _ = io.Copy(client, &budgetReader{reader: upstream, budget: budget})
	}()
}

func handleRenderedHTTP(ctx context.Context, budget *renderTransferBudget, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method denied", http.StatusMethodNotAllowed)
		return
	}
	if _, err := parseImportURL(r.URL.String()); err != nil {
		http.Error(w, "destination denied", http.StatusForbidden)
		return
	}
	request := r.Clone(ctx)
	request.RequestURI = ""
	request.Header.Del("Proxy-Connection")
	request.Header.Del("Proxy-Authorization")
	transport := &http.Transport{
		DialContext:           publicRenderedDial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	response, err := transport.RoundTrip(request)
	if err != nil {
		http.Error(w, "destination denied", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, &budgetReader{reader: response.Body, budget: budget})
}

func renderImportPage(ctx context.Context, raw string) ([]byte, *url.URL, error) {
	chrome := chromiumExecutable()
	if chrome == "" {
		return nil, nil, errRenderedImportUnavailable
	}
	sourceURL, err := parseImportURL(raw)
	if err != nil {
		return nil, nil, err
	}
	renderCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	proxyURL, closeProxy, err := startRenderedImportProxy(renderCtx, newRenderTransferBudget(maxRenderedTransferBytes))
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	defer closeProxy()
	profileDir, err := os.MkdirTemp("", "darkphish-render-*")
	if err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	defer os.RemoveAll(profileDir)

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-background-networking",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-sync",
		"--no-first-run",
		"--no-default-browser-check",
		"--mute-audio",
		"--user-data-dir=" + profileDir,
		"--proxy-server=" + proxyURL,
		"--proxy-bypass-list=<-loopback>",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"about:blank",
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DARKPHISH_CHROMIUM_NO_SANDBOX")), "true") {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.CommandContext(renderCtx, chrome, args...)
	configureRenderedCommand(cmd)
	var stderr boundedBuffer
	stderr.limit = 1 << 20
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, errRenderedImportUnavailable
	}
	defer func() {
		if cmd.Cancel != nil {
			_ = cmd.Cancel()
		}
		_ = cmd.Wait()
	}()

	rendered, finalURL, err := renderImportPageCDP(renderCtx, profileDir, sourceURL.String())
	if err != nil || len(rendered) == 0 || len(rendered) > maxImportedPageBytes || finalURL == nil {
		return nil, nil, errRenderedImportUnavailable
	}
	return rendered, finalURL, nil
}

func renderedImportWarning(err error) string {
	if err == nil {
		return ""
	}
	return "Rendered snapshot was unavailable; imported the static HTML response instead."
}
