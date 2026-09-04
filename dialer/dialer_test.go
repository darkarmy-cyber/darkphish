package dialer

import (
	"fmt"
	"net/netip"
	"strings"
	"syscall"
	"testing"
)

func TestDefaultDeny(t *testing.T) {
	control := restrictedControl(nil)
	host := "169.254.169.254"
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}
}

func TestDefaultAllow(t *testing.T) {
	control := restrictedControl(nil)
	host := "1.1.1.1"
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
}

func TestCustomAllow(t *testing.T) {
	host := "127.0.0.1"
	allowed := []netip.Prefix{netip.MustParsePrefix(host + "/32")}
	control := restrictedControl(allowed)
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
}

func TestCustomDeny(t *testing.T) {
	host := "127.0.0.1"
	allowed := []netip.Prefix{netip.MustParsePrefix(host + "/32")}
	control := restrictedControl(allowed)
	conn := new(syscall.RawConn)
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	got := control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}
}

func TestAllowedHostsDoesNotBlockExternal(t *testing.T) {
	d := &RestrictedDialer{}
	if err := d.SetAllowedHosts([]string{"203.0.113.1/32"}); err != nil {
		t.Fatal(err)
	}
	control := d.Dialer().Control
	conn := new(syscall.RawConn)
	allowed := []struct {
		network string
		address string
	}{
		{network: "tcp4", address: "142.251.127.108:587"},
		{network: "tcp6", address: "[2607:f8b0:4002:c06::1b]:443"},
		{network: "tcp4", address: "203.0.113.1:443"},
	}
	for _, destination := range allowed {
		if err := control(destination.network, destination.address, *conn); err != nil {
			t.Fatalf("expected %s to be allowed: %v", destination.address, err)
		}
	}
	if err := control("tcp4", "127.0.0.1:80", *conn); err == nil || !strings.Contains(err.Error(), "upstream connection denied") {
		t.Fatalf("expected loopback to be denied, got %v", err)
	}
}

func TestSetAllowedHostsReplacesPolicy(t *testing.T) {
	d := &RestrictedDialer{}
	if err := d.SetAllowedHosts([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetAllowedHosts([]string{"10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	got := d.AllowedHosts()
	if len(got) != 1 || got[0] != "10.0.0.1/32" {
		t.Fatalf("allowlist was appended instead of replaced: %#v", got)
	}
}

func TestSingleIP(t *testing.T) {
	orig := DefaultDialer.AllowedHosts()
	host := "127.0.0.1"
	DefaultDialer.SetAllowedHosts([]string{host})
	control := DefaultDialer.Dialer().Control
	conn := new(syscall.RawConn)
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	got := control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}

	host = "::1"
	DefaultDialer.SetAllowedHosts([]string{host})
	control = DefaultDialer.Dialer().Control
	conn = new(syscall.RawConn)
	expected = fmt.Errorf("upstream connection denied to internal host at %s", host)
	got = control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}

	// Test an allowed connection
	got = control("tcp4", fmt.Sprintf("[%s]:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
	DefaultDialer.SetAllowedHosts(orig)
}
