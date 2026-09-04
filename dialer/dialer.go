package dialer

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// RestrictedDialer creates net.Dialers that reject connections to local,
// reserved, metadata, and multicast addresses unless an administrator has
// explicitly allowlisted the destination range.
type RestrictedDialer struct {
	allowedHosts []netip.Prefix
}

// DefaultDialer is the process-wide outbound dialer policy.
var DefaultDialer = &RestrictedDialer{}

// SetAllowedHosts configures explicit exceptions for the default dialer.
func SetAllowedHosts(allowed []string) error {
	return DefaultDialer.SetAllowedHosts(allowed)
}

// AllowedHosts returns a copy of the configured allowlist.
func (d *RestrictedDialer) AllowedHosts() []string {
	ranges := make([]string, 0, len(d.allowedHosts))
	for _, ipRange := range d.allowedHosts {
		ranges = append(ranges, ipRange.String())
	}
	return ranges
}

// SetAllowedHosts validates and atomically replaces the current allowlist.
func (d *RestrictedDialer) SetAllowedHosts(allowed []string) error {
	parsedRanges := make([]netip.Prefix, 0, len(allowed))
	for _, value := range allowed {
		value = strings.TrimSpace(value)
		if singleIP, err := netip.ParseAddr(value); err == nil {
			singleIP = singleIP.Unmap()
			value = netip.PrefixFrom(singleIP, singleIP.BitLen()).String()
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return fmt.Errorf("provided IP range is not valid CIDR notation: %w", err)
		}
		parsedRanges = append(parsedRanges, prefix.Masked())
	}
	d.allowedHosts = parsedRanges
	return nil
}

// Dialer returns a net.Dialer that applies the default outbound policy.
func Dialer() *net.Dialer {
	return DefaultDialer.Dialer()
}

// Dialer returns a net.Dialer that applies the configured outbound policy at
// connect time, after DNS resolution. This protects against DNS rebinding.
func (d *RestrictedDialer) Dialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   restrictedControl(d.allowedHosts),
	}
}

// deniedRanges deliberately does not contain ::/0 or ::ffff:0:0/96. Those
// prefixes match all IPv6 or IPv4-mapped destinations and caused the upstream
// allowed_internal_hosts regression fixed by gophish PR #9425.
var deniedRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type dialControl = func(network, address string, c syscall.RawConn) error

func restrictedControl(allowed []netip.Prefix) dialControl {
	return func(network string, address string, _ syscall.RawConn) error {
		if network != "tcp4" && network != "tcp6" {
			return fmt.Errorf("%s is not a safe network type", network)
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%s is not a valid host/port pair: %w", address, err)
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("%s is not a valid IP address", host)
		}
		ip = ip.WithZone("").Unmap()
		for _, ipRange := range allowed {
			if ipRange.Contains(ip) {
				return nil
			}
		}
		for _, ipRange := range deniedRanges {
			if ipRange.Contains(ip) {
				return fmt.Errorf("upstream connection denied to internal host at %s", host)
			}
		}
		return nil
	}
}
