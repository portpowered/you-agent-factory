package restclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
)

func TestAdapterUsesCallerBaseURLAndHTTPClient(t *testing.T) {
	const clientMarker = "caller-owned-client"
	var requestCount atomic.Int32

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/factory-sessions/missing-session/response-events" {
			t.Errorf("request path = %q, want generated response-events path", r.URL.Path)
		}
		if got := r.Header.Get("X-Functional-Client"); got != clientMarker {
			t.Errorf("X-Functional-Client = %q, want %q", got, clientMarker)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"family":"NOT_FOUND","code":"RESPONSE_EVENT_SESSION_NOT_FOUND","message":"session not found"}`))
	}))
	t.Cleanup(host.Close)

	httpClient := &http.Client{Transport: markerTransport{
		marker: clientMarker,
		base:   http.DefaultTransport,
	}}
	adapter, err := restclient.New(host.URL, httpClient)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response, err := adapter.GetFactoryResponseEventsBySessionID(context.Background(), "missing-session", nil)
	if err != nil {
		t.Fatalf("GetFactoryResponseEventsBySessionID() error = %v", err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requestCount.Load())
	}
	if response.StatusCode() != http.StatusNotFound || response.JSON404 == nil {
		t.Fatalf("response = %#v, want generated typed 404 response", response)
	}
	if response.JSON404.Family != generatedclient.ErrorFamilyNotFound || response.JSON404.Code != generatedclient.RESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf("typed error = %#v, want NOT_FOUND/RESPONSE_EVENT_SESSION_NOT_FOUND", response.JSON404)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		httpClient *http.Client
	}{
		{name: "missing base URL", httpClient: &http.Client{}},
		{name: "relative base URL", baseURL: "/api", httpClient: &http.Client{}},
		{name: "unsupported scheme", baseURL: "ftp://example.test", httpClient: &http.Client{}},
		{name: "base URL query", baseURL: "https://example.test?scope=one", httpClient: &http.Client{}},
		{name: "missing HTTP client", baseURL: "https://example.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := restclient.New(tt.baseURL, tt.httpClient)
			if err == nil {
				t.Fatalf("New() = %#v, want configuration error", adapter)
			}
		})
	}
}

type markerTransport struct {
	marker string
	base   http.RoundTripper
}

func (t markerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errors.New("marker transport received nil request")
	}
	request.Header.Set("X-Functional-Client", t.marker)
	return t.base.RoundTrip(request)
}
