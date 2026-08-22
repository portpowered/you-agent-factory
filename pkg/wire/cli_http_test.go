package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestMetricsCLICompletesReportAfterStandardCLITimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		// This delay intentionally crosses the ordinary 10-second CLI timeout;
		// metrics uses a separate policy for the documented retained-history
		// workload.
		time.Sleep(standardCLIHTTPTimeout + 100*time.Millisecond)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(generatedclient.MetricsReport{
			Cost:      generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Providers: []generatedclient.MetricsBreakdown{{Key: "provider-a"}},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	operation := provideMetricsCLI()
	err := operation(context.Background(), visualizationcli.MetricsConfig{
		Server:  server.URL,
		GroupBy: "provider",
		Output:  &output,
	})
	if err != nil {
		t.Fatalf("metrics operation after standard timeout: %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("metrics operation wrote no report after delayed response")
	}
}
