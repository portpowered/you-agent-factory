package runtime_api

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies is a pre-DI
// transport/client proof. Production-shaped graph equivalence belongs to the
// functional graph coverage introduced after Wire DI.
func TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	var requests atomic.Int32
	httpClient := &http.Client{Transport: countingRoundTripper{
		count: &requests,
		base:  http.DefaultTransport,
	}}
	adapter, err := restclient.New(host.URL(), httpClient)
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	response, err := adapter.GetFactoryResponseEventsBySessionID(context.Background(), "missing-session", nil)
	if err != nil {
		t.Fatalf("request response events through generated REST adapter: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("caller-owned HTTP client request count = %d, want 1", requests.Load())
	}
	if response.StatusCode() != http.StatusNotFound || response.JSON404 == nil {
		t.Fatalf("generated response = %#v, want typed 404 from functional HTTP host", response)
	}
	if response.JSON404.Code != generatedclient.RESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf("generated error code = %q, want %q", response.JSON404.Code, generatedclient.RESPONSEEVENTSESSIONNOTFOUND)
	}
}

type countingRoundTripper struct {
	count *atomic.Int32
	base  http.RoundTripper
}

func (t countingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	t.count.Add(1)
	return t.base.RoundTrip(request)
}
