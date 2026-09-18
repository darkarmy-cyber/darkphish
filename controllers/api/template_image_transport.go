package api

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

type imageIPLookup func(context.Context, string, string) ([]netip.Addr, error)
type imageClientFactory func(string, []netip.Addr) *http.Client

// Resolve every hop once, reject mixed public/private DNS answers, then pin
// connections to the validated numeric addresses. No hostname allowlist exists.
func resolveImageAddresses(ctx context.Context, hostname string, lookup imageIPLookup) ([]netip.Addr, error) {
	answers, err := lookup(ctx, "ip", hostname)
	if err != nil || len(answers) == 0 || len(answers) > 64 {
		return nil, errImagePreview
	}
	addresses := make([]netip.Addr, 0, len(answers))
	for _, ip := range answers {
		if !dialer.IsPublicAddress(ip) {
			return nil, errImagePreview
		}
		addresses = append(addresses, ip.Unmap())
	}
	// Prefer IPv4 on hosts without working IPv6; failures still try the other
	// already-validated addresses under the same overall request deadline.
	sort.SliceStable(addresses, func(i, j int) bool { return addresses[i].Is4() && !addresses[j].Is4() })
	return addresses, nil
}

func newPinnedImageClient(hostname string, addresses []netip.Addr) *http.Client {
	addresses = append([]netip.Addr(nil), addresses...)
	transport := &http.Transport{
		// No environment proxy, cookie jar, shared connection pool or credentials.
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: hostname},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || len(addresses) == 0 || address != net.JoinHostPort(addresses[0].String(), "443") {
			return nil, errImagePreview
		}
		for _, ip := range addresses {
			if !dialer.IsPublicAddress(ip) {
				return nil, errImagePreview
			}
		}
		for _, ip := range addresses {
			// Numeric targets cannot trigger a second DNS lookup. The restricted
			// dialer also enforces its mandatory connect-time boundary.
			d := (&dialer.RestrictedDialer{}).Dialer()
			conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), "443"))
			if err == nil {
				return conn, nil
			}
			if ctx.Err() != nil {
				break
			}
		}
		return nil, errImagePreview
	}
	return &http.Client{
		Transport: transport, Timeout: 8 * time.Second,
		// Redirects must return to our resolver/validator before any new request.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func fetchImagePreview(ctx context.Context, raw string) (string, error) {
	return fetchPinnedImagePreview(ctx, raw, net.DefaultResolver.LookupNetIP, newPinnedImageClient)
}

func fetchPinnedImagePreview(ctx context.Context, raw string, lookup imageIPLookup, factory imageClientFactory) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	target, err := parseImagePreviewURL(raw)
	if err != nil {
		return "", errImagePreview
	}
	for hop := 0; hop < 5; hop++ {
		if ctx.Err() != nil {
			return "", errImagePreview
		}
		addresses, err := resolveImageAddresses(ctx, target.Hostname(), lookup)
		if err != nil {
			return "", errImagePreview
		}
		// The network authority comes only from validated numeric DNS answers.
		// The original URL contributes only its path/query, HTTP Host and TLS SNI.
		endpoint := &url.URL{Scheme: "https", Host: net.JoinHostPort(addresses[0].String(), "443"),
			Path: target.Path, RawPath: target.RawPath, RawQuery: target.RawQuery, ForceQuery: target.ForceQuery}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return "", errImagePreview
		}
		request.Host = target.Host
		client := factory(target.Hostname(), addresses)
		response, err := client.Do(request)
		if err != nil {
			client.CloseIdleConnections()
			return "", errImagePreview
		}
		if response.StatusCode == 301 || response.StatusCode == 302 || response.StatusCode == 303 || response.StatusCode == 307 || response.StatusCode == 308 {
			location := response.Header.Get("Location")
			response.Body.Close()
			client.CloseIdleConnections()
			if location == "" || len(location) > 4096 {
				return "", errImagePreview
			}
			reference, err := url.Parse(location)
			if err != nil {
				return "", errImagePreview
			}
			// Resolve relative redirects against the original origin, never the
			// numeric connection address or the server's own administrative URL.
			target, err = parseImagePreviewURL(target.ResolveReference(reference).String())
			if err != nil {
				return "", errImagePreview
			}
			continue
		}
		data, err := readImagePreview(response)
		response.Body.Close()
		client.CloseIdleConnections()
		return data, err
	}
	return "", errImagePreview
}
