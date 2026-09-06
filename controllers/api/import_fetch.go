package api

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

const maxImportedPageBytes = 8 << 20

var errImportFailed = errors.New("site import failed: destination denied, unavailable, or untrusted")

// URL checks complement, never replace, the RestrictedDialer's connect-time
// address policy. That policy is also applied after DNS resolution on every hop.
func parseImportURL(raw string) (*url.URL, error) {
	invalid := errors.New("site import requires an absolute HTTP(S) URL without credentials")
	if len(raw) == 0 || len(raw) > 4096 || strings.ContainsAny(raw, "\\\r\n\t") || strings.TrimSpace(raw) != raw {
		return nil, invalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Hostname() == "" || strings.Contains(parsed.Hostname(), "%") {
		return nil, invalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, invalid
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, invalid
		}
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed, nil
}

func newImportClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// No environment proxy: all connections must use the restricted dialer.
			DialContext:            dialer.Dialer().DialContext,
			TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  10 * time.Second,
			MaxResponseHeaderBytes: 1 << 20,
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errImportFailed
			}
			if _, err := parseImportURL(request.URL.String()); err != nil {
				return errImportFailed
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && request.URL.Scheme != "https" {
				return errImportFailed
			}
			return nil
		},
	}
}

func fetchImportPage(ctx context.Context, raw string) ([]byte, *url.URL, error) {
	target, err := parseImportURL(raw)
	if err != nil {
		return nil, nil, err
	}
	client := newImportClient()
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, errImportFailed
	}
	response, err := client.Do(request)
	if err != nil {
		// Do not expose URL userinfo, query tokens, DNS answers or TLS diagnostics.
		return nil, nil, errImportFailed
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 || response.ContentLength > maxImportedPageBytes {
		return nil, nil, errImportFailed
	}
	// The transport decompresses gzip before this limit is applied.
	content, err := io.ReadAll(io.LimitReader(response.Body, maxImportedPageBytes+1))
	if err != nil || len(content) > maxImportedPageBytes {
		return nil, nil, errImportFailed
	}
	return content, response.Request.URL, nil
}
