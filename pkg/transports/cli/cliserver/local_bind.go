package cliserver

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// LocalBindTarget is the host and TCP port selected for a local listener.
type LocalBindTarget struct {
	Host string
	Port int
}

// LocalBindError reports an invalid local server endpoint selected for a
// run/server host.
type LocalBindError struct {
	Cause error
}

func (err *LocalBindError) Error() string {
	if err == nil || err.Cause == nil {
		return "invalid local server endpoint"
	}
	return err.Cause.Error()
}

func (err *LocalBindError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// IsLocalBindError reports whether err contains local server endpoint failure.
func IsLocalBindError(err error) bool {
	var bindErr *LocalBindError
	return errors.As(err, &bindErr)
}

// LocalBindTargetFromServer resolves server into a validated local bind host and TCP port.
// The server host must be a local loopback name (localhost, 127.0.0.1, or ::1).
func LocalBindTargetFromServer(server string) (LocalBindTarget, error) {
	base, err := ResolveBase(server)
	if err != nil {
		return LocalBindTarget{}, &LocalBindError{Cause: err}
	}
	return LocalBindTargetFromBase(base)
}

// LocalBindTargetFromListen resolves the explicit host:port syntax accepted by
// listener-owning CLI commands. Unlike the legacy --server compatibility path,
// --listen requires a non-zero port because it is an exact bind request.
func LocalBindTargetFromListen(listen string) (LocalBindTarget, error) {
	trimmed := strings.TrimSpace(listen)
	if trimmed == "" {
		return LocalBindTarget{}, &LocalBindError{Cause: fmt.Errorf(
			"--listen address is required (use --listen 127.0.0.1:7437)",
		)}
	}
	hostname, portText, err := net.SplitHostPort(trimmed)
	if err != nil {
		return LocalBindTarget{}, &LocalBindError{Cause: fmt.Errorf(
			"invalid --listen address %q: expected a local host:port such as 127.0.0.1:7437: %w",
			listen, err,
		)}
	}
	host, err := localBindHostnameFor("--listen", hostname)
	if err != nil {
		return LocalBindTarget{}, &LocalBindError{Cause: err}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return LocalBindTarget{}, &LocalBindError{Cause: fmt.Errorf(
			"invalid --listen port %q: a non-zero TCP port between 1 and 65535 is required",
			portText,
		)}
	}
	return LocalBindTarget{Host: host, Port: port}, nil
}

// LocalBindTargetFromBase derives a local bind host and TCP port from a validated base URI.
func LocalBindTargetFromBase(base Base) (LocalBindTarget, error) {
	host, err := localBindHostname(base.URL.Hostname())
	if err != nil {
		return LocalBindTarget{}, &LocalBindError{Cause: err}
	}
	port, err := tcpPortFromURL(base.URL)
	if err != nil {
		return LocalBindTarget{}, &LocalBindError{Cause: err}
	}
	return LocalBindTarget{Host: host, Port: port}, nil
}

func localBindHostname(hostname string) (string, error) {
	return localBindHostnameFor("server", hostname)
}

func localBindHostnameFor(source, hostname string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(hostname))
	switch normalized {
	case "localhost", "127.0.0.1", "::1":
		return normalized, nil
	default:
		return "", fmt.Errorf(
			"%s host %q is not a local bind target; use a loopback host such as localhost or 127.0.0.1",
			source, hostname,
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
