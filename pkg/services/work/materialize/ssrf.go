package materialize

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateRemoteTarget(ctx context.Context, rawURL string, parsed *url.URL, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return ssrfError(rawURL)
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return ssrfError(rawURL)
		}
		return nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return inaccessibleError(rawURL, "dns lookup failed")
	}
	if len(ips) == 0 {
		return inaccessibleError(rawURL, "dns lookup returned no addresses")
	}
	for _, addr := range ips {
		if isBlockedIP(addr.IP) {
			return ssrfError(rawURL)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP metadata and similar link-local service endpoints.
	if v4 := ip.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 {
		return true
	}
	return false
}

func ssrfError(rawURL string) error {
	return fmt.Errorf("media url not allowed: %s (ssrf)", rawURL)
}

func inaccessibleError(rawURL, reason string) error {
	return fmt.Errorf("media url inaccessible: %s (%s)", rawURL, reason)
}
