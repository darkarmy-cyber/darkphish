package dialer

import (
	"net/netip"
	"testing"
)

func TestPublicAddressBoundary(t *testing.T) {
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if !IsPublicAddress(netip.MustParseAddr(value)) {
			t.Errorf("public address rejected: %s", value)
		}
	}
	for _, value := range []string{"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.0.1", "192.0.0.1", "192.0.2.1", "192.88.99.1", "192.168.0.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "fc00::1", "fd00:ec2::254", "fe80::1", "ff02::1", "2606:4700:4700::1111%eth0"} {
		if IsPublicAddress(netip.MustParseAddr(value)) {
			t.Errorf("non-public address accepted: %s", value)
		}
	}
	if IsPublicAddress(netip.Addr{}) {
		t.Fatal("invalid address accepted")
	}
}
