package api

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/netip"
	"reflect"
	"testing"
)

func TestImageAddressResolution(t *testing.T) {
	public := netip.MustParseAddr("8.8.8.8")
	v6 := netip.MustParseAddr("2606:4700:4700::1111")
	for _, tc := range []struct {
		name    string
		answers []netip.Addr
		wantOK  bool
	}{
		{"public", []netip.Addr{v6, public}, true},
		{"mapped public", []netip.Addr{netip.MustParseAddr("::ffff:8.8.8.8")}, true},
		{"empty", nil, false},
		{"invalid", []netip.Addr{{}}, false},
		{"mixed", []netip.Addr{public, netip.MustParseAddr("127.0.0.1")}, false},
		{"metadata", []netip.Addr{netip.MustParseAddr("169.254.169.254")}, false},
		{"mapped internal", []netip.Addr{netip.MustParseAddr("::ffff:10.0.0.1")}, false},
		{"zone", []netip.Addr{v6.WithZone("eth0")}, false},
		{"too many", make([]netip.Addr, 65), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]netip.Addr(nil), tc.answers...)
			calls := 0
			got, err := resolveImageAddresses(context.Background(), "arbitrary-user-selected.test", func(ctx context.Context, network, hostname string) ([]netip.Addr, error) {
				calls++
				if network != "ip" || hostname != "arbitrary-user-selected.test" {
					t.Fatal("hostname restricted or altered")
				}
				return tc.answers, nil
			})
			if (err == nil) != tc.wantOK || calls != 1 {
				t.Fatal("incorrect resolution policy")
			}
			if tc.wantOK && (len(got) == 0 || !got[0].Is4()) {
				t.Fatal("IPv4 not preferred or mapped address not normalized")
			}
			if !reflect.DeepEqual(before, tc.answers) {
				t.Fatal("resolver answers modified")
			}
		})
	}
	_, err := resolveImageAddresses(context.Background(), "private-query.test", func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, errors.New("private DNS details")
	})
	if err != errImagePreview {
		t.Fatal("DNS diagnostics exposed")
	}
}

func TestPinnedImageClientRefusesUnpinnedTargets(t *testing.T) {
	addresses := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	client := newPinnedImageClient("arbitrary.test", addresses)
	defer client.CloseIdleConnections()
	addresses[0] = netip.MustParseAddr("127.0.0.1")
	transport := client.Transport.(*http.Transport)
	for _, address := range []string{"arbitrary.test:443", "1.1.1.1:443", "8.8.8.8:80", "127.0.0.1:443"} {
		conn, err := transport.DialContext(context.Background(), "tcp", address)
		if err == nil {
			conn.Close()
			t.Fatal("unpinned destination accepted")
		}
	}
	if client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("automatic redirect enabled")
	}
	if conn, err := transport.DialContext(context.Background(), "udp", "8.8.8.8:443"); err == nil {
		conn.Close()
		t.Fatal("non-TCP network accepted")
	}
}

func TestImageRedirectsResolveOriginalOriginAndRevalidate(t *testing.T) {
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	for _, tc := range []struct {
		name, location string
		wantOK         bool
		wantHost       string
		wantCalls      int
	}{
		{"relative", "../next%2Fimage.png?x=1#ignored", true, "arbitrary.test", 2},
		{"other public host", "https://another-user-host.test/next.png", true, "another-user-host.test", 2},
		{"numeric HTTPS port", "https://another-user-host.test:0443/next.png", true, "another-user-host.test", 2},
		{"internal", "https://internal.test/a", false, "", 1},
		{"rebind", "/again", false, "", 1},
		{"downgrade", "http://arbitrary.test/a", false, "", 1},
		{"credentials", "https://user:secret@arbitrary.test/a", false, "", 1},
		{"port", "https://arbitrary.test:8443/a", false, "", 1},
		{"loop", "/again", false, "", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookups, calls := 0, 0
			lookup := func(ctx context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("DNS lookup not deadline bounded")
				}
				if host == "internal.test" || (tc.name == "rebind" && lookups > 1) {
					return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
				}
				return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
			}
			factory := func(host string, addresses []netip.Addr) *http.Client {
				return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: previewTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host != "8.8.8.8:443" || r.Host != host {
						t.Fatal("authority not pinned")
					}
					if len(r.Header) != 0 {
						t.Fatal("headers forwarded")
					}
					if calls == 1 || tc.name == "loop" {
						return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{tc.location}}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
					}
					if host != tc.wantHost {
						t.Fatal("relative redirect resolved against numeric IP")
					}
					if tc.name == "relative" && r.URL.RequestURI() != "/next%2Fimage.png?x=1" {
						t.Fatal("escaped path/query not preserved")
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(picture.Bytes()))}, nil
				})}
			}
			data, err := fetchPinnedImagePreview(context.Background(), "https://arbitrary.test/folder/start.png", lookup, factory)
			if (err == nil) != tc.wantOK || calls != tc.wantCalls {
				t.Fatalf("result=%v requests=%d wantOK=%v requests=%d", err, calls, tc.wantOK, tc.wantCalls)
			}
			if !tc.wantOK && (err != errImagePreview || data != "") {
				t.Fatal("unsafe error result")
			}
			if tc.wantOK && lookups != calls {
				t.Fatal("unexpected repeat lookup within a hop")
			}
		})
	}
}

func TestImagePreviewCancelledBeforeLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchPinnedImagePreview(ctx, "https://arbitrary.test/a", func(context.Context, string, string) ([]netip.Addr, error) {
		t.Fatal("lookup after cancellation")
		return nil, nil
	}, nil)
	if err != errImagePreview {
		t.Fatal("cancelled request accepted")
	}
}
