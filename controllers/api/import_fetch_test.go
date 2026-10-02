package api

import (
	"compress/gzip"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkarmy-cyber/darkphish/dialer"
)

func TestImportTrustedTLSAndRedirectPolicy(t *testing.T) {
	original := dialer.DefaultDialer.AllowedHosts()
	if err := dialer.SetAllowedHosts([]string{"127.0.0.1/32"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dialer.SetAllowedHosts(original) })
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("trusted synthetic page"))
	}))
	defer server.Close()
	client := newImportClient()
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || transport.TLSClientConfig.InsecureSkipVerify || client.Timeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.MaxResponseHeaderBytes <= 0 {
		t.Fatal("import client lost its security/resource policy")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig.RootCAs = roots
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal("a properly trusted certificate must work", err)
	}
	_ = response.Body.Close()
	first := httptest.NewRequest(http.MethodGet, "https://source.example.test", nil)
	for _, target := range []string{"http://source.example.test", "https://user:synthetic@example.test", "file:///private"} {
		next := httptest.NewRequest(http.MethodGet, target, nil)
		if client.CheckRedirect(next, []*http.Request{first}) == nil {
			t.Fatalf("unsafe redirect accepted: %q", target)
		}
	}
	next := httptest.NewRequest(http.MethodGet, "https://destination.example.test", nil)
	if err := client.CheckRedirect(next, []*http.Request{first}); err != nil {
		t.Fatal("safe HTTPS redirect rejected")
	}
	if client.CheckRedirect(next, []*http.Request{first, first, first, first, first}) == nil {
		t.Fatal("unbounded redirect chain")
	}
}

func TestImportBoundsDecompressedResponse(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var writer io.Writer = w
			if compressed {
				w.Header().Set("Content-Encoding", "gzip")
				stream := gzip.NewWriter(w)
				defer stream.Close()
				writer = stream
			}
			chunk := strings.Repeat("A", 1<<20)
			for range 9 {
				if _, err := io.WriteString(writer, chunk); err != nil {
					return
				}
			}
		}))
		response := importSyntheticSite(t, server.URL, []string{"127.0.0.1/32"})
		server.Close()
		if response.Code != http.StatusBadRequest {
			t.Fatalf("oversized response accepted (gzip=%t)", compressed)
		}
	}
}

func TestImportCancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := fetchImportPage(ctx, "http://127.0.0.1:1"); err == nil {
		t.Fatal("cancelled request should not fetch")
	}
}

func TestImportSanitizesHTMLAndKeepsForms(t *testing.T) {
	page := `<html><head><base href="https://evil.example.test"></head><body><script>window.bad=true</script><form action="https://collect.example.test" onsubmit="bad()"><input name="field"><textarea name="note">text</textarea><button formaction="https://collect.example.test">Submit</button></form></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, page) }))
	defer server.Close()
	response := importSyntheticSite(t, server.URL, []string{"127.0.0.1/32"})
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var imported cloneResponse
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(imported.HTML))
	if err != nil {
		t.Fatal(err)
	}
	if document.Find("form").Length() != 1 || document.Find("input[name=field]").Length() != 1 || document.Find("textarea[name=note]").Length() != 1 || document.Find("button").Length() != 1 {
		t.Fatal("form controls were not preserved")
	}
	form := document.Find("form").First()
	if action, _ := form.Attr("action"); action != "" {
		t.Fatal("remote form action survived")
	}
	if method, _ := form.Attr("method"); method != "post" {
		t.Fatal("imported form does not submit through the landing-page endpoint")
	}
	if document.Find("script,base,[onsubmit],[formaction]").Length() != 0 || strings.Contains(imported.HTML, "data-darkphish-training") || strings.Contains(imported.HTML, "data-training-notice") {
		t.Fatal("unsafe or retired mode markup survived import")
	}
}
