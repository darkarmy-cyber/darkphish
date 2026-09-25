package middleware

import (
	"net"
	"net/http"
	"strings"
)

func trustedProxyNetworks(entries []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if ip.To4() != nil {
				ip = ip.To4()
				bits = 32
			}
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			networks = append(networks, network)
		}
	}
	return networks
}

func proxyIsTrusted(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func requestPeer(r *http.Request) (net.IP, string) {
	host, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr), ""
	}
	return net.ParseIP(host), port
}

func forwardedClientIP(r *http.Request, peer net.IP, networks []*net.IPNet) net.IP {
	if peer == nil || !proxyIsTrusted(peer, networks) {
		return peer
	}
	if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		var chain []net.IP
		for _, value := range forwarded {
			for _, item := range strings.Split(value, ",") {
				ip := net.ParseIP(strings.TrimSpace(item))
				if ip == nil {
					return peer
				}
				chain = append(chain, ip)
			}
		}
		current := peer
		for i := len(chain) - 1; i >= 0 && proxyIsTrusted(current, networks); i-- {
			current = chain[i]
		}
		return current
	}
	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP
	}
	return peer
}

// TrustedProxyHeaders applies client-address forwarding only when the immediate
// network peer is explicitly trusted. Untrusted and malformed headers are ignored.
func TrustedProxyHeaders(next http.Handler, trustedProxies []string) http.Handler {
	networks := trustedProxyNetworks(trustedProxies)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, port := requestPeer(r)
		client := forwardedClientIP(r, peer, networks)
		if client != nil && !client.Equal(peer) {
			if port == "" {
				r.RemoteAddr = client.String()
			} else {
				r.RemoteAddr = net.JoinHostPort(client.String(), port)
			}
		}
		next.ServeHTTP(w, r)
	})
}
