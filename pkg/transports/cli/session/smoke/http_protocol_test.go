package session_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type smokeHTTPClock struct{}

func (smokeHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func testSessionHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, smokeHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}
