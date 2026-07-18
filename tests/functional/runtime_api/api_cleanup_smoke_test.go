package runtime_api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "cleanup smoke"},
	})
	assertTerminalDispatchForTrace(t, stream, traceID)

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work"))
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	completed := work.Results[0]
	if support.StringPointerValue(completed.TraceId) != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", support.StringPointerValue(completed.TraceId), traceID)
	}
	if generatedWorkStateName(completed.State) != "complete" || generatedWorkStateType(completed.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", completed.State)
	}

	statusRead, err := host.REST().GetStatus(context.Background())
	if err != nil {
		t.Fatalf("generated GetStatus() error = %v", err)
	}
	if statusRead.StatusCode() != http.StatusOK || statusRead.JSON200 == nil {
		t.Fatalf("generated GetStatus() response = %#v, want typed 200", statusRead)
	}
	if statusRead.JSON200.TotalTokens != 1 {
		t.Fatalf("generated GET /status total_tokens = %d, want 1", statusRead.JSON200.TotalTokens)
	}
	if statusRead.JSON200.Categories.Terminal != 1 {
		t.Fatalf("generated GET /status terminal count = %d, want 1", statusRead.JSON200.Categories.Terminal)
	}
	assertCleanupSmokeDashboardShell(t, host.Endpoint())
}

func assertTerminalDispatchForTrace(t *testing.T, stream *factoryEventHTTPStream, traceID string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if event.Context.TraceIds == nil {
			continue
		}
		for _, eventTraceID := range *event.Context.TraceIds {
			if eventTraceID == traceID {
				return
			}
		}
	}
	t.Fatalf("canonical session event stream did not expose terminal DISPATCH_RESPONSE for trace %q", traceID)
}

func assertCleanupSmokeDashboardShell(t *testing.T, baseURL string) {
	t.Helper()

	resp, err := http.Get(baseURL + "/dashboard/ui")
	if err != nil {
		t.Fatalf("GET /dashboard/ui: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /dashboard/ui: %v", err)
	}
	shell := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard/ui status = %d, want 200: %s", resp.StatusCode, shell)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /dashboard/ui content type = %q, want html shell", resp.Header.Get("Content-Type"))
	}
	if strings.TrimSpace(shell) == "" {
		t.Fatal("GET /dashboard/ui returned an empty shell")
	}

	routeResp, err := http.Get(baseURL + "/dashboard/ui/work/" + url.PathEscape("work-from-cleanup-smoke"))
	if err != nil {
		t.Fatalf("GET /dashboard/ui/work/...: %v", err)
	}
	defer routeResp.Body.Close()
	routeBody, err := io.ReadAll(routeResp.Body)
	if err != nil {
		t.Fatalf("read dashboard client route: %v", err)
	}
	if routeResp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard client route status = %d, want 200", routeResp.StatusCode)
	}
	if string(routeBody) != shell {
		t.Fatal("dashboard client route should fall back to the embedded app shell")
	}
}
