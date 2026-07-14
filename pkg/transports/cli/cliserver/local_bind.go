package cliserver

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// LocalBindTarget is the host and TCP port derived from a local --server URI for you run.
type LocalBindTarget struct {
	Host string
	Port int
}

// LocalBindTargetFromServer resolves server into a validated local bind host and TCP port.
// The server host must be a local loopback name (localhost, 127.0.0.1, or ::1).
func LocalBindTargetFromServer(server string) (LocalBindTarget, error) {
	base, err := ResolveBase(server)
	if err != nil {
		return LocalBindTarget{}, err
	}
	return LocalBindTargetFromBase(base)
}

// LocalBindTargetFromBase derives a local bind host and TCP port from a validated base URI.
func LocalBindTargetFromBase(base Base) (LocalBindTarget, error) {
	host, err := localBindHostname(base.URL.Hostname())
	if err != nil {
		return LocalBindTarget{}, err
	}
	port, err := tcpPortFromURL(base.URL)
	if err != nil {
		return LocalBindTarget{}, err
	}
	return LocalBindTarget{Host: host, Port: port}, nil
}

func localBindHostname(hostname string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(hostname))
	switch normalized {
	case "localhost", "127.0.0.1", "::1":
		return normalized, nil
	default:
		return "", fmt.Errorf(
			"server host %q is not a local bind target; you run only supports local hosts such as localhost or 127.0.0.1 (use --server http://localhost:7437)",
			hostname,
		)
	}
}

func tcpPortFromURL(u url.URL) (int, error) {
	portText := u.Port()
	if portText == "" {
		switch u.Scheme {
		case "http":
			portText = "80"
		case "https":
			portText = "443"
		default:
			return 0, fmt.Errorf("unsupported scheme %q for local bind port", u.Scheme)
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, fmt.Errorf("invalid server port %q: %w", portText, err)
	}
	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("invalid server port %d: must be between 0 and 65535", port)
	}
	return port, nil
}
