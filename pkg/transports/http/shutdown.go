package http

import (
	"net"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ShutdownOperation is the narrow invocation-local cancellation capability
// exposed to the administrative HTTP route by runtime composition.
type ShutdownOperation func()

var _ interface {
	ShutdownServer(http.ResponseWriter, *http.Request)
} = (*Server)(nil)

// ShutdownServer acknowledges a loopback administrative request before
// invoking the cancellation authority. Forwarded headers are deliberately not
// consulted; authorization uses the actual peer and listener addresses.
func (s *Server) ShutdownServer(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackShutdownRequest(r) {
		s.writeError(w, http.StatusForbidden, "shutdown control requires a loopback peer", "SHUTDOWN_CONTROL_REJECTED")
		return
	}
	if s == nil || s.shutdown == nil {
		s.writeError(w, http.StatusServiceUnavailable, "shutdown control is unavailable", "SHUTDOWN_CONTROL_UNAVAILABLE")
		return
	}

	s.writeJSON(w, http.StatusAccepted, factoryapi.ShutdownAcceptedResponse{
		Status:  factoryapi.Accepted,
		Message: "graceful shutdown accepted",
	})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	s.shutdownOnce.Do(s.shutdown)
}

func isLoopbackShutdownRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	remoteIP := ipFromNetworkAddress(r.RemoteAddr)
	if remoteIP == nil || !remoteIP.IsLoopback() {
		return false
	}
	localAddr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || localAddr == nil {
		return false
	}
	localIP := ipFromNetworkAddress(localAddr.String())
	return localIP != nil && localIP.IsLoopback()
}

func ipFromNetworkAddress(address string) net.IP {
	trimmed := strings.TrimSpace(address)
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	trimmed = strings.Trim(strings.TrimSpace(trimmed), "[]")
	if zone := strings.LastIndex(trimmed, "%"); zone >= 0 {
		trimmed = trimmed[:zone]
	}
	return net.ParseIP(trimmed)
}
