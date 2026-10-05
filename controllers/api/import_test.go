package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
	"github.com/darkarmy-cyber/darkphish/models"
)

func makeImportRequest(ctx *testContext, allowedHosts []string, url string) *httptest.ResponseRecorder {
	orig := dialer.DefaultDialer.AllowedHosts()
	dialer.SetAllowedHosts(allowedHosts)
	req := httptest.NewRequest(http.MethodPost, "/api/import/site",
		bytes.NewBuffer([]byte(fmt.Sprintf(`
			{
				"url" : "%s"
			}
		`, url))))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	ctx.apiServer.ImportSite(response, req)
	dialer.SetAllowedHosts(orig)
	return response
}

func TestDefaultDeniedImport(t *testing.T) {
	ctx := setupTest(t)
	metadataURL := "http://169.254.169.254/latest/meta-data/"
	response := makeImportRequest(ctx, []string{}, metadataURL)
	expectedCode := http.StatusBadRequest
	if response.Code != expectedCode {
		t.Fatalf("incorrect status code received. expected %d got %d", expectedCode, response.Code)
	}
	got := &models.Response{}
	err := json.NewDecoder(response.Body).Decode(got)
	if err != nil {
		t.Fatalf("error decoding body: %v", err)
	}
	if !strings.Contains(got.Message, "site import failed") {
		t.Fatalf("incorrect response error provided: %s", got.Message)
	}
}

func TestDefaultAllowedImport(t *testing.T) {
	ctx := setupTest(t)
	h := "<html><head></head><body><img src=\"/test.png\"/></body></html>"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, h)
	}))
	defer ts.Close()
	response := makeImportRequest(ctx, []string{"127.0.0.1/32", "::1/128"}, ts.URL)
	expectedCode := http.StatusOK
	if response.Code != expectedCode {
		t.Fatalf("incorrect status code received. expected %d got %d", expectedCode, response.Code)
	}
}

func TestCustomDeniedImport(t *testing.T) {
	ctx := setupTest(t)
	h := "<html><head></head><body><img src=\"/test.png\"/></body></html>"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, h)
	}))
	defer ts.Close()
	response := makeImportRequest(ctx, []string{"192.168.1.1"}, ts.URL)
	expectedCode := http.StatusBadRequest
	if response.Code != expectedCode {
		t.Fatalf("incorrect status code received. expected %d got %d", expectedCode, response.Code)
	}
	got := &models.Response{}
	err := json.NewDecoder(response.Body).Decode(got)
	if err != nil {
		t.Fatalf("error decoding body: %v", err)
	}
	if !strings.Contains(got.Message, "site import failed") {
		t.Fatalf("incorrect response error provided: %s", got.Message)
	}
}

func TestImportSiteSharesRequestDeadlineWithRenderer(t *testing.T) {
	if importSiteWorkTimeout >= 30*time.Second {
		t.Fatalf("site import work timeout %s must remain below the admin server write timeout", importSiteWorkTimeout)
	}

	ctx := setupTest(t)
	h := "<html><head></head><body><p>static fallback</p></body></html>"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, h)
	}))
	defer ts.Close()

	originalRender := renderImportPageForImport
	t.Cleanup(func() { renderImportPageForImport = originalRender })
	renderImportPageForImport = func(renderCtx context.Context, rawURL string) ([]byte, *url.URL, error) {
		if _, ok := renderCtx.Deadline(); !ok {
			t.Fatal("renderer context has no deadline")
		}
		<-renderCtx.Done()
		return nil, nil, renderCtx.Err()
	}

	originalAllowed := dialer.DefaultDialer.AllowedHosts()
	dialer.SetAllowedHosts([]string{"127.0.0.1/32", "::1/128"})
	t.Cleanup(func() { dialer.SetAllowedHosts(originalAllowed) })

	requestCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/import/site",
		bytes.NewBufferString(fmt.Sprintf(`{"url":%q}`, ts.URL))).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	start := time.Now()
	ctx.apiServer.ImportSite(response, req)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("site import exceeded bounded request deadline: %s", elapsed)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected static fallback response, got status %d: %s", response.Code, response.Body.String())
	}
	var got cloneResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "static" || !strings.Contains(got.HTML, "static fallback") || len(got.Warnings) == 0 {
		t.Fatalf("unexpected fallback response: %#v", got)
	}
}

func TestImportSiteDeadlineCoversBodyDecode(t *testing.T) {
	ctx := setupTest(t)
	requestCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	reader, writer := io.Pipe()
	defer writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/import/site", reader).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	start := time.Now()
	ctx.apiServer.ImportSite(response, req)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("body decoding outlived the shared import deadline: %s", elapsed)
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected timed-out body decode to fail with status %d, got %d", http.StatusBadRequest, response.Code)
	}
}
