package resume_from_recording_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	metricsLineageInputTokens  int64 = 1_000_000
	metricsLineageOutputTokens int64 = 2_000_000
)

// TestSingleDispatchMetricsStayScopedAcrossRecordingResume proves the
// customer-visible live and resumed paths with one priced dispatch. The
// source is queried while its server is still live, then its finalized
// recording is resumed in a replacement server without a provider call.
func TestSingleDispatchMetricsStayScopedAcrossRecordingResume(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldSingleStepFactory(t, "single-dispatch-metrics-resume")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	home := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	sourceRecording := filepath.Join(home, "source.recording.jsonl")
	successorRecording := filepath.Join(home, "successor.recording.jsonl")
	sourceRunner := newMetricsLineageCommandRunner()
	source := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--record", sourceRecording},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: sourceRunner},
	})

	submitted := support.SubmitDefaultSessionWork(t, source.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPointer("single-dispatch-metrics-resume"),
		Payload:      map[string]any{"subject": "one-lineage"},
		WorkTypeName: "task",
	})
	if submitted.Accepted != true || submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("source Work submission = %#v, want one accepted Work identity", submitted)
	}
	sourceRunner.waitForStart(t)
	sourceRunner.release(t)
	support.WaitForSessionTerminalStatus(t, source.URL(), factorysessions.DefaultSessionID, 15*time.Second)

	sourceSession := support.GetDefaultSession(t, source.URL())
	if sourceSession.Id == "" || sourceSession.Id == factorysessions.DefaultSessionID {
		t.Fatalf("source Factory Session ID = %q, want generated canonical identity", sourceSession.Id)
	}
	sourceSnapshot := queryMetricsLineageSnapshot(t, source.URL(), sourceSession.Id)
	assertMetricsLineageSnapshot(t, sourceSnapshot, sourceSession.Id, sourceSession.Id)

	defaultSnapshot := queryMetricsLineageSnapshot(t, source.URL(), factorysessions.DefaultSessionID)
	assertMetricsLineageSnapshot(t, defaultSnapshot, factorysessions.DefaultSessionID, sourceSession.Id)
	assertMetricsLineageFactsEqual(t, "live ~default", sourceSnapshot, defaultSnapshot)
	if got := sourceRunner.CallCount(); got != 1 {
		t.Fatalf("source provider command calls = %d, want exactly one", got)
	}

	source.Close(t)
	if _, err := os.Stat(sourceRecording); err != nil {
		t.Fatalf("source recording after close: %v", err)
	}

	successorRunner := testutil.NewProviderCommandRunner()
	successor := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--resume", sourceRecording, "--record", successorRecording},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: successorRunner},
	})
	support.WaitForSessionTerminalStatus(t, successor.URL(), factorysessions.DefaultSessionID, 15*time.Second)

	successorSession := support.GetDefaultSession(t, successor.URL())
	if successorSession.Id == "" || successorSession.Id == factorysessions.DefaultSessionID {
		t.Fatalf("successor Factory Session ID = %q, want generated canonical identity", successorSession.Id)
	}
	if successorSession.Id == sourceSession.Id {
		t.Fatalf("successor Factory Session ID = %q, want distinct source identity %q", successorSession.Id, sourceSession.Id)
	}
	listed := support.ListDefaultSessionWork(t, successor.URL())
	if len(listed.Results) != 1 || support.WorkItemCustomerLocation(listed.Results[0]) != support.WorkCustomerLocation("task", "complete") {
		t.Fatalf("successor Work projection = %#v, want one terminal Work", listed.Results)
	}

	successorSnapshot := queryMetricsLineageSnapshot(t, successor.URL(), successorSession.Id)
	assertMetricsLineageSnapshot(t, successorSnapshot, successorSession.Id, sourceSession.Id)
	assertMetricsLineageFactsEqual(t, "resumed successor", sourceSnapshot, successorSnapshot)
	repeatedSuccessorSnapshot := queryMetricsLineageSnapshot(t, successor.URL(), successorSession.Id)
	assertMetricsLineageFactsEqual(t, "repeated successor", successorSnapshot, repeatedSuccessorSnapshot)
	if got := successorRunner.CallCount(); got != 0 {
		t.Fatalf("successor provider command calls = %d, want zero redispatches", got)
	}
}

type metricsLineageSnapshot struct {
	metrics factoryapi.MetricsReport
	costs   generatedclient.CostsReport
}

func queryMetricsLineageSnapshot(
	t testing.TB,
	baseURL, sessionID string,
) metricsLineageSnapshot {
	t.Helper()
	selector := url.QueryEscape(sessionID)
	return metricsLineageSnapshot{
		metrics: support.GetJSON[factoryapi.MetricsReport](
			t,
			baseURL+"/metrics?session_id="+selector,
		),
		costs: support.GetJSON[generatedclient.CostsReport](
			t,
			baseURL+"/metrics/costs?session_id="+selector,
		),
	}
}

func assertMetricsLineageSnapshot(
	t testing.TB,
	snapshot metricsLineageSnapshot,
	requestedSessionID, lineageSessionID string,
) {
	t.Helper()
	if snapshot.metrics.Scope.FactorySessionId == nil || *snapshot.metrics.Scope.FactorySessionId != requestedSessionID {
		t.Fatalf("metrics scope = %#v, want requested session %q", snapshot.metrics.Scope, requestedSessionID)
	}
	if snapshot.metrics.Totals.InputTokens != float64(metricsLineageInputTokens) ||
		snapshot.metrics.Totals.OutputTokens != float64(metricsLineageOutputTokens) ||
		snapshot.metrics.Totals.CompletedDispatches != 1 ||
		snapshot.metrics.Totals.ProviderLatency.Samples != 1 ||
		snapshot.metrics.Totals.ProviderLatency.P50 == nil ||
		*snapshot.metrics.Totals.ProviderLatency.P50 <= 0 {
		t.Fatalf("metrics totals = %#v, want one dispatch, exact tokens, and positive provider latency", snapshot.metrics.Totals)
	}
	if len(snapshot.metrics.UsageRows) != 1 {
		t.Fatalf("metrics usage rows = %#v, want exactly one correlated row", snapshot.metrics.UsageRows)
	}
	usage := snapshot.metrics.UsageRows[0]
	if usage.FactorySessionId == nil || *usage.FactorySessionId != lineageSessionID ||
		usage.InputTokens == nil || *usage.InputTokens != metricsLineageInputTokens ||
		usage.OutputTokens == nil || *usage.OutputTokens != metricsLineageOutputTokens ||
		usage.DispatchId == nil || *usage.DispatchId == "" ||
		usage.WorkId == nil || *usage.WorkId == "" ||
		usage.WorkerSessionId == nil || *usage.WorkerSessionId == "" {
		t.Fatalf("metrics usage row = %#v, want one source-correlated lineage row", usage)
	}

	if snapshot.costs.Scope.FactorySessionId == nil || *snapshot.costs.Scope.FactorySessionId != requestedSessionID ||
		snapshot.costs.Status != generatedclient.CostsReportStatus("PRICED") ||
		snapshot.costs.KnownCost == nil || *snapshot.costs.KnownCost != "21.25" ||
		snapshot.costs.PricedSubtotal == nil || *snapshot.costs.PricedSubtotal != "21.25" ||
		snapshot.costs.Coverage.EncounteredRows != 1 || snapshot.costs.Coverage.PricedRows != 1 ||
		snapshot.costs.Coverage.UnpricedRows != 0 || len(snapshot.costs.LineItems) != 1 {
		t.Fatalf("costs report = %#v, want one fully priced row at 21.25", snapshot.costs)
	}
	item := snapshot.costs.LineItems[0]
	if item.FactorySessionId == nil || *item.FactorySessionId != lineageSessionID ||
		item.Status != generatedclient.CostsLineItemStatus("PRICED") ||
		item.PricedAmount == nil || *item.PricedAmount != "21.25" ||
		item.InputTokens == nil || *item.InputTokens != metricsLineageInputTokens ||
		item.OutputTokens == nil || *item.OutputTokens != metricsLineageOutputTokens ||
		item.Provider == nil || *item.Provider != "CODEX" ||
		item.Model == nil || *item.Model != "gpt-5-codex" ||
		item.DispatchId == nil || *item.DispatchId == "" ||
		item.WorkId == nil || *item.WorkId == "" ||
		item.WorkerSessionId == nil || *item.WorkerSessionId == "" {
		t.Fatalf("costs line item = %#v, want one source-correlated priced row", item)
	}
}

func assertMetricsLineageFactsEqual(
	t testing.TB,
	phase string,
	left, right metricsLineageSnapshot,
) {
	t.Helper()
	if !reflect.DeepEqual(left.metrics.Totals, right.metrics.Totals) ||
		!reflect.DeepEqual(left.metrics.Providers, right.metrics.Providers) ||
		!reflect.DeepEqual(left.metrics.WorkerTypes, right.metrics.WorkerTypes) ||
		!reflect.DeepEqual(left.metrics.Workstations, right.metrics.Workstations) ||
		!reflect.DeepEqual(left.metrics.UsageRows, right.metrics.UsageRows) ||
		!reflect.DeepEqual(left.costs.LineItems, right.costs.LineItems) ||
		!reflect.DeepEqual(left.costs.ProviderModels, right.costs.ProviderModels) ||
		!reflect.DeepEqual(left.costs.WorkerSessions, right.costs.WorkerSessions) ||
		!reflect.DeepEqual(left.costs.WorkItems, right.costs.WorkItems) ||
		!reflect.DeepEqual(left.costs.FactorySessions, right.costs.FactorySessions) ||
		left.costs.Status != right.costs.Status ||
		left.costs.KnownCost == nil || right.costs.KnownCost == nil ||
		*left.costs.KnownCost != *right.costs.KnownCost {
		t.Fatalf("%s lineage facts differ:\nleft metrics=%#v\nright metrics=%#v\nleft costs=%#v\nright costs=%#v", phase, left.metrics, right.metrics, left.costs, right.costs)
	}
}

type metricsLineageCommandRunner struct {
	delegate    *testutil.ProviderCommandRunner
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newMetricsLineageCommandRunner() *metricsLineageCommandRunner {
	return &metricsLineageCommandRunner{
		delegate: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdoutWithUsage(
				"single dispatch COMPLETE",
				metricsLineageInputTokens,
				metricsLineageOutputTokens,
			),
		}),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (runner *metricsLineageCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.startOnce.Do(func() { close(runner.started) })
	select {
	case <-runner.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return runner.delegate.Run(ctx, request)
}

func (runner *metricsLineageCommandRunner) waitForStart(t testing.TB) {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-runner.started:
	case <-timer.C:
		t.Fatal("source provider command did not start")
	}
}

func (runner *metricsLineageCommandRunner) release(t testing.TB) {
	t.Helper()
	runner.releaseOnce.Do(func() { close(runner.release) })
}

func (runner *metricsLineageCommandRunner) CallCount() int {
	return runner.delegate.CallCount()
}

var _ platformprocess.CommandRunner = (*metricsLineageCommandRunner)(nil)
