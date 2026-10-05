package inference_test

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertLocalAIConfiguredServerLoopbackPort(t *testing.T, serverURL string) int {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse configured-server URL %q: %v", serverURL, err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("configured-server URL = %q, want an OS-assigned loopback listener", serverURL)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		t.Fatalf("configured-server port %q: %v", portText, err)
	}
	if port == 7437 {
		t.Fatalf("configured-server port = %d, want OS-assigned port rather than fixed 7437", port)
	}
	return port
}

func assertLocalAIConfiguredServerReleased(
	t *testing.T,
	server *support.FunctionalAPIServer,
	serverURL string,
) {
	t.Helper()
	select {
	case <-server.Done():
	//nolint:testsleep // The test waits for an observed event; this deadline only bounds failure or cleanup.
	case <-time.After(5 * time.Second):
		t.Fatal("configured-server process did not finish after Close")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(strings.TrimSuffix(serverURL, "/") + "/status")
	if err == nil {
		response.Body.Close()
		t.Fatalf("configured-server listener still accepted requests after Close")
	}
}
