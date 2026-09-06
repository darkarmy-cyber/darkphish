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

func TestImportMetadataAttributesPreserveSimulationHTML(t *testing.T) {
	page := `<html><head></head><body><script>window.simulationOnly=true</script><form action='/submit?x=" data-bad="'><input name="field"></form></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, page) }))
	defer server.Close()
	response := importSyntheticSite(t, server.URL+`/page?x="`, []string{"127.0.0.1/32"})
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
	metadata := document.Find(`input[name="__original_url"]`)
	if metadata.Length() != 1 || len(metadata.Nodes[0].Attr) != 3 || document.Find("[data-bad]").Length() != 0 {
		t.Fatal("import metadata escaped its attribute boundary")
	}
	if document.Find("script").Text() != "window.simulationOnly=true" {
		t.Fatal("simulation HTML was globally sanitized")
	}
}
