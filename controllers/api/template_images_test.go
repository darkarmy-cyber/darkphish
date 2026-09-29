package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
	"github.com/darkarmy-cyber/darkphish/models"
)

type previewTransport func(*http.Request) (*http.Response, error)

func (f previewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImagePreviewURLPolicy(t *testing.T) {
	target, err := parseImagePreviewURL("https://example.test/image.png#not-sent")
	if err != nil || target.String() != "https://example.test/image.png" {
		t.Fatal("request must use the validated, fragment-free URL")
	}
	for _, raw := range []string{"http://example.test/a.png", "//example.test/a", "file:///private", "https://u:p@example.test/a", "https://example.test:8443/a", "https://example.test\\a", "https://example.test/\nsecret", "", "https://example.test/" + strings.Repeat("a", 4096)} {
		if validImagePreviewURL(raw) {
			t.Errorf("accepted %q", raw)
		}
	}
	if !validImagePreviewURL("https://ssl.gstatic.com/social/photosui/images/email/header/logo_photos_cs_color_165x32dp.png") {
		t.Fatal("normal HTTPS image rejected")
	}
	client := newPinnedImageClient("example.test", []netip.Addr{netip.MustParseAddr("8.8.8.8")})
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.ServerName != "example.test" || client.Jar != nil || client.Timeout == 0 {
		t.Fatal("unsafe preview client")
	}
	first := httptest.NewRequest("GET", "https://example.test/a", nil)
	for _, raw := range []string{"http://example.test/a", "https://u:p@example.test/a", "https://example.test:8443/a"} {
		if client.CheckRedirect(httptest.NewRequest("GET", raw, nil), []*http.Request{first}) == nil {
			t.Fatal("unsafe redirect accepted")
		}
	}
	if client.CheckRedirect(first, []*http.Request{first, first, first, first, first}) == nil {
		t.Fatal("unbounded redirects")
	}
}

func TestImagePreviewNumericHTTPSPorts(t *testing.T) {
	for _, host := range []string{"example.test", "[2606:4700:4700::1111]"} {
		for _, port := range []string{"443", "0443", "000443"} {
			target, err := parseImagePreviewURL("https://" + host + ":" + port + "/image.png")
			if err != nil || target.String() != "https://"+host+"/image.png" {
				t.Fatalf("HTTPS port not canonicalized: %s:%s", host, port)
			}
		}
	}
	for _, port := range []string{"0444", "0080", "0", "65536", "+443", "443x"} {
		if validImagePreviewURL("https://example.test:" + port + "/image.png") {
			t.Fatalf("non-HTTPS port accepted: %s", port)
		}
	}
}

func TestImagePreviewDeniesInternalEvenWhenImportAllowsIt(t *testing.T) {
	original := dialer.DefaultDialer.AllowedHosts()
	if err := dialer.SetAllowedHosts([]string{"127.0.0.1/32"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dialer.SetAllowedHosts(original) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("internal endpoint contacted") }))
	defer server.Close()
	client := newPinnedImageClient("localhost", []netip.Addr{netip.MustParseAddr("127.0.0.1")})
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	for _, address := range []string{"127.0.0.1:443", "localhost:443", server.Listener.Addr().String()} {
		conn, err := transport.DialContext(context.Background(), "tcp", address)
		if err == nil {
			conn.Close()
			t.Fatal("internal connection allowed")
		}
	}
}

func TestRasterPreviewValidatesAndReencodes(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	data, err := rasterPreview(append(input.Bytes(), []byte("discarded metadata")...))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if err != nil || bytes.Contains(decoded, []byte("discarded metadata")) {
		t.Fatal("not a clean raster")
	}
	for _, bad := range [][]byte{[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), []byte(`<html>not an image</html>`), input.Bytes()[:24], make([]byte, maxPreviewImageBytes+1)} {
		if _, err := rasterPreview(bad); err == nil {
			t.Fatal("invalid raster accepted")
		}
	}
	var oversized bytes.Buffer
	_ = png.Encode(&oversized, image.NewNRGBA(image.Rect(0, 0, 1001, 1000)))
	if _, err := rasterPreview(oversized.Bytes()); err == nil {
		t.Fatal("pixel limit bypassed")
	}
	var limited previewBuffer
	if _, err := limited.Write(make([]byte, maxPreviewImageBytes+1)); err == nil {
		t.Fatal("output limit bypassed")
	}
}

func TestImagePreviewFetchIsBoundedAndCredentialFree(t *testing.T) {
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	for _, scenario := range []string{"ok", "status", "large", "unknown-size", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			client := &http.Client{Transport: previewTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://8.8.8.8:443/image?token=private" || r.Host != "example.test" || r.URL.Fragment != "" {
					t.Fatal("request did not use the pinned address and original virtual host")
				}
				if len(r.Header) != 0 {
					t.Fatal("forwarded request headers")
				}
				body := picture.Bytes()
				status, length := http.StatusOK, int64(len(body))
				switch scenario {
				case "status":
					status = http.StatusForbidden
				case "large":
					length = maxPreviewImageBytes + 1
				case "unknown-size":
					length = -1
					body = make([]byte, maxPreviewImageBytes+1)
				case "invalid":
					body = []byte("private diagnostic")
				}
				return &http.Response{StatusCode: status, ContentLength: length, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
			})}
			lookup := func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
			}
			data, err := fetchPinnedImagePreview(context.Background(), "https://example.test/image?token=private#not-sent", lookup, func(string, []netip.Addr) *http.Client { return client })
			if scenario == "ok" && (err != nil || !strings.HasPrefix(data, "data:image/png;base64,")) {
				t.Fatal("valid preview failed")
			}
			if scenario != "ok" && (err != errImagePreview || data != "") {
				t.Fatal("invalid preview or diagnostic exposed")
			}
		})
	}
}

func TestImagePreviewRequestValidationAndAuthentication(t *testing.T) {
	for _, body := range []string{`{}`, `{"urls":[]}`, `{"urls":["http://example.test/a"]}`, `{"urls":["https://example.test/a"],"extra":true}`, `{"urls":["https://example.test/a"]}{}`, strings.Repeat("x", 65<<10)} {
		r := httptest.NewRequest("POST", "/api/import/email/images", strings.NewReader(body))
		w := httptest.NewRecorder()
		(&Server{}).PreviewEmailImages(w, r)
		if w.Code != http.StatusBadRequest || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("invalid input status %d", w.Code)
		}
	}
	tooMany, _ := json.Marshal(map[string][]string{"urls": make([]string, maxPreviewImages+1)})
	w := httptest.NewRecorder()
	(&Server{}).PreviewEmailImages(w, httptest.NewRequest("POST", "/api/import/email/images", bytes.NewReader(tooMany)))
	if w.Code != http.StatusBadRequest {
		t.Fatal("unbounded image count")
	}
	w = httptest.NewRecorder()
	(&Server{}).PreviewEmailImages(w, httptest.NewRequest("GET", "/api/import/email/images", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal("GET allowed")
	}
	ctx := setupTest(t)
	w = httptest.NewRecorder()
	ctx.apiServer.ServeHTTP(w, httptest.NewRequest("POST", "/api/import/email/images", strings.NewReader(`{"urls":["https://example.test/a"]}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated preview returned %d", w.Code)
	}
	for _, scope := range []string{"templates:read", "templates:write"} {
		_, token, err := models.CreatePersonalAccessToken(ctx.admin.Id, "preview scope test", []string{scope}, time.Now().UTC().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/api/import/email/images", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		ctx.apiServer.ServeHTTP(w, r)
		want := http.StatusForbidden
		if scope == "templates:write" {
			want = http.StatusBadRequest
		}
		if w.Code != want {
			t.Fatalf("scope %s returned %d, want %d", scope, w.Code, want)
		}
	}
}

func TestImagePreviewBatchLimitsAndDeniedResults(t *testing.T) {
	imagePreviewSlots <- struct{}{}
	imagePreviewSlots <- struct{}{}
	w := httptest.NewRecorder()
	(&Server{}).PreviewEmailImages(w, httptest.NewRequest("POST", "/api/import/email/images", strings.NewReader(`{"urls":["https://127.0.0.1/a"]}`)))
	<-imagePreviewSlots
	<-imagePreviewSlots
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("busy batch was not rejected")
	}
	w = httptest.NewRecorder()
	(&Server{}).PreviewEmailImages(w, httptest.NewRequest("POST", "/api/import/email/images", strings.NewReader(`{"urls":["https://127.0.0.1/a","https://127.0.0.1/a"]}`)))
	var results []imagePreviewResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(results) != 1 || results[0].Data != "" || results[0].Error != errImagePreview.Error() {
		t.Fatal("denied result or deduplication failed")
	}
	if len(imagePreviewSlots) != 0 {
		t.Fatal("batch slot leaked")
	}
}

func TestMalformedEmailImportReturnsError(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{}).ImportEmail(w, httptest.NewRequest("POST", "/api/import/email", strings.NewReader(`{"content":"not a raw email","convert_links":true}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatal("malformed email not rejected")
	}
}
