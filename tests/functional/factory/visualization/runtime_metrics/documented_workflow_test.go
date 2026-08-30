package runtime_metrics_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestMetricsDocumentedWorkflowThroughRootProcess follows the packaged guide
// through a server assembled by the canonical root/wire graph. The CLI and
// direct HTTP assertions share that customer-facing server, so the test does
// not construct a second metrics adapter or transport graph.
func TestMetricsDocumentedWorkflowThroughRootProcess(t *testing.T) {
	t.Parallel()
	serverURL, env, workingDirectory, scopeClock, home := startDocumentedMetricsServer(t)
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	publicID := discoverDocumentedLiveSession(t, process, serverURL, env, workingDirectory)
	writeDocumentedMetrics(t, home, publicID)
	unscoped := runDocumentedMetrics(t, process, serverURL, env, workingDirectory)
	scoped := runDocumentedMetrics(t, process, serverURL, env, workingDirectory, "--session", publicID)
	assertDocumentedScopeSubset(t, unscoped, scoped, publicID)
	assertDocumentedGroupings(t, process, serverURL, env, workingDirectory, publicID)
	assertDocumentedProviderArithmetic(t, process, serverURL, env, workingDirectory, publicID)
	assertDocumentedHTTPParity(t, serverURL, publicID, scoped)
	assertDocumentedScopeFailures(t, process, serverURL, env, workingDirectory, scopeClock)
	functionalevidence.Covers(t, "rest/getMetrics")
}

func startDocumentedMetricsServer(
	t *testing.T,
) (string, []string, string, *documentedMetricsScopeClock, string) {
	t.Helper()
	home := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDirectory := support.ScaffoldSingleStepFactory(t, "metrics-documentation-workflow")
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	server := support.NewProcessAPIServer()
	scopeClock := &documentedMetricsScopeClock{}
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: server.Start, Clock: scopeClock,
	})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDirectory, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workingDirectory
	support.StartProcessCommand(t, process, inputs.Input)
	serverURL := server.WaitForURL(t)
	return serverURL, environment, workingDirectory, scopeClock, home
}

type documentedMetricsScopeClock struct {
	mu         sync.RWMutex
	returnZero bool
}

func (clock *documentedMetricsScopeClock) EnableUnavailable() {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.returnZero = true
}

func (clock *documentedMetricsScopeClock) Now() time.Time {
	clock.mu.RLock()
	returnZero := clock.returnZero
	clock.mu.RUnlock()
	if returnZero {
		return time.Time{}
	}
	return time.Now()
}

func writeDocumentedMetrics(t *testing.T, home, sessionID string) {
	t.Helper()
	root := platformmetrics.RuntimeMetricsRoot(home)
	common := func(sessionID, dispatchID, metric string, value float64) map[string]any {
		return map[string]any{
			"metric_name": metric, "value": value, "unit": "tokens",
			"session_id": sessionID, "dispatch_id": dispatchID,
			"workstation": "workstation-a", "worker_type": "worker-a",
		}
	}
	records := []map[string]any{
		common(sessionID, "dispatch-codex", runtimeDispatchComplete, 1),
		common(sessionID, "dispatch-codex", runtimeProviderInputTokens, 4),
		common(sessionID, "dispatch-unknown", runtimeDispatchComplete, 1),
		common(sessionID, "dispatch-unknown", runtimeProviderInputTokens, 3),
		common("other-session", "dispatch-other", runtimeDispatchComplete, 1),
		common("other-session", "dispatch-other", runtimeProviderInputTokens, 100),
	}
	records[1]["provider"] = "codex"
	records[3]["provider"] = "${workerProvider}"
	writeRuntimeMetricsArtifact(t, filepath.Join(root, "120000.000000000-runtime-metrics-documented.log"), false, records)
}

const (
	runtimeDispatchComplete     = "dispatch.completed"
	runtimeDispatchDuration     = "dispatch.duration"
	runtimeProviderDuration     = "provider.duration"
	runtimeProviderFailed       = "provider.failed"
	runtimeProviderInputTokens  = "provider.input_tokens"
	runtimeProviderOutputTokens = "provider.output_tokens"
)

func writeRuntimeMetricsArtifact(
	t *testing.T,
	path string,
	compressed bool,
	records []map[string]any,
	tail ...string,
) {
	t.Helper()
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode runtime metric: %v", err)
		}
	}
	if len(tail) > 0 {
		if _, err := io.WriteString(&content, tail[0]); err != nil {
			t.Fatalf("write torn runtime metric: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if !compressed {
		if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
		return
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q): %v", path, err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(content.Bytes()); err != nil {
		_ = writer.Close()
		_ = file.Close()
		t.Fatalf("compress %q: %v", path, err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close gzip %q: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %q: %v", path, err)
	}
}

func discoverDocumentedLiveSession(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory string,
) string {
	t.Helper()
	result := runDocumentedCommand(t, process, environment, workingDirectory,
		"you", "--json", "--server", serverURL, "session", "list", "--scope", "live")
	if result.err != nil {
		t.Fatalf("live session list: %v\nstderr:\n%s", result.err, result.inputs.Stderr())
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(result.inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode live session list: %v\n%s", err, result.inputs.Stdout())
	}
	if len(response.Sessions) == 0 || strings.TrimSpace(response.Sessions[0].Id) == "" {
		t.Fatalf("live sessions = %#v, want a public Factory Session ID", response.Sessions)
	}
	return response.Sessions[0].Id
}

func runDocumentedMetrics(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory string,
	args ...string,
) documentedMetricsCommandResult {
	t.Helper()
	return runDocumentedCommand(t, process, environment, workingDirectory,
		append([]string{"you", "--json", "--server", serverURL, "metrics"}, args...)...)
}

func assertDocumentedScopeSubset(
	t *testing.T,
	unscoped, scoped documentedMetricsCommandResult,
	publicID string,
) {
	t.Helper()
	var allReport, scopedReport factoryapi.MetricsReport
	decodeDocumentedMetricsReport(t, unscoped.inputs.Stdout(), &allReport)
	decodeDocumentedMetricsReport(t, scoped.inputs.Stdout(), &scopedReport)
	if scopedReport.Scope.FactorySessionId == nil || *scopedReport.Scope.FactorySessionId != publicID {
		t.Fatalf("scoped report scope = %#v, want %q", scopedReport.Scope, publicID)
	}
	if scopedReport.Totals.CompletedDispatches <= 0 || scopedReport.Totals.CompletedDispatches >= allReport.Totals.CompletedDispatches {
		t.Fatalf("scoped completed dispatches = %v, unscoped = %v, want a non-zero proper subset", scopedReport.Totals.CompletedDispatches, allReport.Totals.CompletedDispatches)
	}
	if scopedReport.Totals.InputTokens > allReport.Totals.InputTokens || scopedReport.Totals.OutputTokens > allReport.Totals.OutputTokens {
		t.Fatalf("scoped token totals = (%v, %v), unscoped = (%v, %v), want component-wise subset", scopedReport.Totals.InputTokens, scopedReport.Totals.OutputTokens, allReport.Totals.InputTokens, allReport.Totals.OutputTokens)
	}
}

func assertDocumentedGroupings(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory, publicID string,
) {
	t.Helper()
	for _, grouping := range []string{"workstation", "worker", "provider"} {
		result := runDocumentedCommand(t, process, environment, workingDirectory,
			"you", "--server", serverURL, "metrics", "--group-by", grouping, "--session", publicID)
		if result.err != nil || !strings.Contains(result.inputs.Stdout(), "Breakdown by "+grouping) || result.inputs.Stderr() != "" {
			t.Fatalf("human metrics group=%s: error=%v stdout=%q stderr=%q", grouping, result.err, result.inputs.Stdout(), result.inputs.Stderr())
		}
	}
}

func assertDocumentedProviderArithmetic(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory, publicID string,
) {
	t.Helper()
	providerJSON := runDocumentedCommand(t, process, environment, workingDirectory,
		"you", "--json", "--server", serverURL, "metrics", "--group-by", "provider", "--session", publicID)
	if providerJSON.err != nil || providerJSON.inputs.Stderr() != "" {
		t.Fatalf("provider JSON metrics: error=%v stderr=%q", providerJSON.err, providerJSON.inputs.Stderr())
	}
	var report struct {
		Totals factoryapi.MetricsAggregate   `json:"totals"`
		Groups []factoryapi.MetricsBreakdown `json:"groups"`
	}
	if err := json.Unmarshal([]byte(providerJSON.inputs.Stdout()), &report); err != nil {
		t.Fatalf("decode provider JSON metrics: %v\n%s", err, providerJSON.inputs.Stdout())
	}
	completed := 0.0
	for _, group := range report.Groups {
		if strings.Contains(group.Key, "${") {
			t.Fatalf("provider report exposed template key %q", group.Key)
		}
		completed += group.Aggregate.CompletedDispatches
	}
	if completed != report.Totals.CompletedDispatches || len(report.Groups) == 0 {
		t.Fatalf("provider completed dispatches = %v, total = %v, groups = %#v", completed, report.Totals.CompletedDispatches, report.Groups)
	}
}

func assertDocumentedHTTPParity(
	t *testing.T,
	serverURL, publicID string,
	scoped documentedMetricsCommandResult,
) {
	t.Helper()
	response, err := http.Get(serverURL + "/metrics?session_id=" + publicID)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", response.StatusCode)
	}
	var routeReport, cliReport factoryapi.MetricsReport
	if err := json.NewDecoder(response.Body).Decode(&routeReport); err != nil {
		t.Fatalf("decode GET /metrics: %v", err)
	}
	decodeDocumentedMetricsReport(t, scoped.inputs.Stdout(), &cliReport)
	if routeReport.Totals.CompletedDispatches != cliReport.Totals.CompletedDispatches {
		t.Fatalf("HTTP completed dispatches = %v, CLI = %v, want parity", routeReport.Totals.CompletedDispatches, cliReport.Totals.CompletedDispatches)
	}
}

func assertDocumentedScopeFailures(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory string,
	scopeClock *documentedMetricsScopeClock,
) {
	t.Helper()
	result := runDocumentedCommand(t, process, environment, workingDirectory,
		"you", "--json", "--server", serverURL, "metrics", "--session", "missing-live-id")
	if result.err == nil || result.inputs.Stdout() != "" || !strings.Contains(result.inputs.Stderr(), "METRICS_SESSION_NOT_FOUND") || !strings.Contains(result.inputs.Stderr(), "you session list --scope live") {
		t.Fatalf("unknown session result: error=%v stdout=%q stderr=%q", result.err, result.inputs.Stdout(), result.inputs.Stderr())
	}
	response, err := http.Get(serverURL + "/metrics?session_id=missing-live-id")
	if err != nil {
		t.Fatalf("GET unknown metrics session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown metrics session status = %d, want 404", response.StatusCode)
	}

	unmappableFactory := support.ScaffoldSingleStepFactory(t, "metrics-unmappable-session")
	opened := support.OpenFactorySessionAt(t, serverURL, unmappableFactory)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" || strings.TrimSpace(opened.Session.FactoryDir) == "" {
		t.Fatalf("opened unmappable Factory Session = %#v, want public identity", opened)
	}
	actualFactoryDirectory := opened.Session.FactoryDir
	// A live session remains discoverable through the control-plane fallback
	// when its projection cannot be built. Make the canonical projection clock
	// unavailable only after opening this real session so the list route retains
	// its public identity while detail resolution returns scope unavailable.
	scopeClock.EnableUnavailable()
	unmappableID := discoverDocumentedLiveSessionForFactory(t, process, serverURL, environment, workingDirectory, actualFactoryDirectory)
	assertDocumentedUnmappableFailure(t, process, serverURL, environment, workingDirectory, unmappableID)
}

func discoverDocumentedLiveSessionForFactory(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory, factoryDirectory string,
) string {
	t.Helper()
	result := runDocumentedCommand(t, process, environment, workingDirectory,
		"you", "--json", "--server", serverURL, "session", "list", "--scope", "live")
	if result.err != nil {
		t.Fatalf("live session list for unmappable scope: %v\nstderr:\n%s", result.err, result.inputs.Stderr())
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(result.inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode live session list for unmappable scope: %v\n%s", err, result.inputs.Stdout())
	}
	for _, session := range response.Sessions {
		if filepath.Clean(session.FactoryDir) == filepath.Clean(factoryDirectory) {
			return session.Id
		}
	}
	t.Fatalf("live session list = %#v, want session for %q", response.Sessions, factoryDirectory)
	return ""
}

func assertDocumentedUnmappableFailure(
	t *testing.T,
	process support.Process,
	serverURL string,
	environment []string,
	workingDirectory, sessionID string,
) {
	t.Helper()
	result := runDocumentedCommand(t, process, environment, workingDirectory,
		"you", "--json", "--server", serverURL, "metrics", "--session", sessionID)
	if result.err == nil || result.inputs.Stdout() != "" ||
		!strings.Contains(result.inputs.Stderr(), "METRICS_SESSION_SCOPE_UNAVAILABLE") ||
		!strings.Contains(result.inputs.Stderr(), "you session list --scope live") {
		t.Fatalf("unmappable session result: error=%v stdout=%q stderr=%q", result.err, result.inputs.Stdout(), result.inputs.Stderr())
	}
	response, err := http.Get(serverURL + "/metrics?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("GET unmappable metrics session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unmappable metrics session status = %d, want 503", response.StatusCode)
	}
	var body factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode unmappable metrics error: %v", err)
	}
	if body.Code != factoryapi.ErrorResponseCode("METRICS_SESSION_SCOPE_UNAVAILABLE") ||
		!strings.Contains(body.Message, "you session list --scope live") {
		t.Fatalf("unmappable metrics response = %#v, want coded guidance", body)
	}
}

type documentedMetricsCommandResult struct {
	inputs *support.CapturedInputs
	err    error
}

func runDocumentedCommand(
	t *testing.T,
	process support.Process,
	environment []string,
	workingDirectory string,
	args ...string,
) documentedMetricsCommandResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = workingDirectory
	return documentedMetricsCommandResult{inputs: inputs, err: process.Execute(inputs.Input)}
}

func decodeDocumentedMetricsReport(t *testing.T, output string, report *factoryapi.MetricsReport) {
	t.Helper()
	if err := json.Unmarshal([]byte(output), report); err != nil {
		t.Fatalf("decode metrics report: %v\n%s", err, output)
	}
}
