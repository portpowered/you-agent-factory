package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestCLIHTTPProfilesPreserveCommandTimeouts(t *testing.T) {
	t.Parallel()
	standard, err := provideStandardCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	extended, err := provideExtendedCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	if standard.Protocol == nil || standard.timeout != 10*time.Second {
		t.Fatalf("standard profile = %#v", standard)
	}
	if extended.Protocol == nil || extended.timeout != 15*time.Second {
		t.Fatalf("extended profile = %#v", extended)
	}
}

func TestModelsPullCLIHTTPProfileHasNoFixedClientTimeout(t *testing.T) {
	t.Parallel()
	pull, err := provideModelsPullCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	if pull.Protocol == nil {
		t.Fatal("Models pull protocol is nil")
	}
	if pull.timeout != 0 {
		t.Fatalf("Models pull timeout = %s, want no fixed client timeout", pull.timeout)
	}
}

func TestModelAssetHTTPClientAllowsBodyPastFormerWholeTransferBudget(t *testing.T) {
	t.Parallel()

	const formerWholeTransferBudget = 20 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		// This delay is intentionally longer than the former whole-transfer
		// budget while remaining short enough for a deterministic unit test.
		time.Sleep(2 * formerWholeTransferBudget)
		_, _ = io.WriteString(writer, "model-bytes")
	}))
	defer server.Close()

	client := newModelAssetHTTPClient()
	if client.Timeout != 0 {
		t.Fatalf("asset client timeout = %s, want no whole-transfer deadline", client.Timeout)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("asset request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read streamed asset: %v", err)
	}
	if string(body) != "model-bytes" {
		t.Fatalf("streamed asset = %q, want model-bytes", body)
	}
}

func TestMetricsCLICompletesReportAfterStandardCLITimeout(t *testing.T) {
	t.Parallel()
	assertMetricsCLIHTTPTimeoutPolicies(t)
	t.Run("success crosses local HTTP and renders report", testMetricsCLISuccess)
	t.Run("transport failure preserves cause and output", testMetricsCLITransportFailure)
}

func assertMetricsCLIHTTPTimeoutPolicies(t *testing.T) {
	t.Helper()
	standard, err := provideStandardCLIHTTPProtocol()
	if err != nil {
		t.Fatalf("provideStandardCLIHTTPProtocol(): %v", err)
	}
	if standard.timeout != 10*time.Second {
		t.Fatalf("standard CLI timeout = %s, want 10s", standard.timeout)
	}
	if metricsClient := newMetricsCLIHTTPClient(nil); metricsClient.Timeout != 5*time.Minute {
		t.Fatalf("metrics CLI timeout = %s, want 5m", metricsClient.Timeout)
	}
}

func testMetricsCLISuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/metrics" {
			t.Errorf("request path = %q, want /metrics", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(generatedclient.MetricsReport{
			Cost:      generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Providers: []generatedclient.MetricsBreakdown{{Key: "provider-a"}},
		}); err != nil {
			t.Errorf("encode metrics report: %v", err)
		}
	}))
	defer server.Close()

	transport := &metricsCLIForwardingTransport{next: http.DefaultTransport}
	var output bytes.Buffer
	err := provideMetricsCLIWithHTTPTransport(transport)(context.Background(), visualizationcli.MetricsConfig{
		Server:  server.URL,
		GroupBy: "provider",
		Output:  &output,
	})
	if err != nil {
		t.Fatalf("metrics operation: %v", err)
	}
	if transport.requests != 1 || transport.requestPath != "/metrics" {
		t.Fatalf("forwarded requests = %d at %q, want one /metrics request", transport.requests, transport.requestPath)
	}
	if transport.effectiveDeadline <= standardCLIHTTPTimeout {
		t.Fatalf("effective metrics deadline = %s, want greater than standard timeout %s", transport.effectiveDeadline, standardCLIHTTPTimeout)
	}
	for _, want := range []string{"Breakdown by provider: 1 rows", "provider-a:"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, output.String())
		}
	}
}

func testMetricsCLITransportFailure(t *testing.T) {
	wantCause := errors.New("injected metrics transport failure")
	transport := &metricsCLIForwardingTransport{
		next: metricsRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, wantCause
		}),
	}
	var output bytes.Buffer
	err := provideMetricsCLIWithHTTPTransport(transport)(context.Background(), visualizationcli.MetricsConfig{
		Server:  "http://metrics.test",
		GroupBy: "provider",
		Output:  &output,
	})
	if err == nil {
		t.Fatal("metrics operation error = nil, want transport failure")
	}
	var metricsErr *visualizationcli.MetricsError
	if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != visualizationcli.MetricsQueryFailedCode {
		t.Fatalf("error = %#v, want typed metrics query failure", err)
	}
	if !errors.Is(err, wantCause) {
		t.Fatalf("error = %v, want injected cause preserved", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed metrics request wrote partial output %q", output.String())
	}
}

type metricsCLIForwardingTransport struct {
	next              http.RoundTripper
	requests          int
	requestPath       string
	effectiveDeadline time.Duration
}

func (transport *metricsCLIForwardingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	transport.requestPath = request.URL.Path
	if deadline, ok := request.Context().Deadline(); ok {
		transport.effectiveDeadline = time.Until(deadline)
	}
	return transport.next.RoundTrip(request)
}

type metricsRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip metricsRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
