package resume_from_recording_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// W6-S/F: CLI resume restores a completed Work and its selected metrics without
// redispatch. Rejected inputs leave the source and an active healthy peer intact.
// All parallel scenarios share one process; each owns its profile, listener,
// session and command-runner route. HTTP reads observe the CLI/API parity contract.
func TestRecordingResumePreservesWorkAndRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	var routes sync.Map
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: recordingResumeRoutes{runners: &routes},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			route := ctx.Value(recordingResumeServerKey{}).(recordingResumeServer)
			close(route.started)
			return route.api.Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	for _, kind := range []string{"success", "missing", "malformed", "unsupported version"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldSingleStepFactory(t, "selected-recording-resume")
			support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
			runner := support.NewRecordingCommandRunner("selected recording COMPLETE")
			routes.Store(filepath.Clean(dir), runner)
			t.Cleanup(func() { routes.Delete(filepath.Clean(dir)) })
			home := t.TempDir()
			sourcePath := filepath.Join(home, "source.json")
			metricsRoot := filepath.Join(home, "metrics")
			source := startRecordingResumeSession(t, process, dir, home, "--record", sourcePath, "--runtime-metrics-dir", metricsRoot)
			submit := recordingResumeInputs(t, dir, t.TempDir())
			submit.Args = []string{"you", "submit", "batch", `{"requestId":"selected-recording","type":"FACTORY_REQUEST_BATCH","works":[{"name":"recorded-work","workTypeName":"task","payload":{"subject":"selected-recorded-payload"}}]}`,
				"--server", source.url, "--session", source.id}
			if err := process.Execute(submit.Input); err != nil {
				t.Fatalf("submit recorded Work: %v; stderr=%s", err, submit.Stderr())
			}
			support.WaitForSessionTerminalStatus(t, source.url, source.id, resumeFromRecordingTimeout)
			before := selectedRecordingWork(t, source)
			metrics := selectedRecordingMetrics(t, process, source)
			if metrics.Totals.CompletedDispatches != 1 || metrics.Totals.InputTokens <= 0 || metrics.Totals.OutputTokens <= 0 {
				t.Fatalf("source metrics = %#v, want one completed provider dispatch", metrics.Totals)
			}
			source.command.Stop(t)
			original := readRecordingResumeFile(t, sourcePath)
			peerHome := t.TempDir()
			peer := startRecordingResumeSession(t, process, dir, peerHome, "--resume", sourcePath, "--record", filepath.Join(peerHome, "successor.json"), "--runtime-metrics-dir", metricsRoot)
			support.WaitForSessionTerminalStatus(t, peer.url, peer.id, resumeFromRecordingTimeout)
			after := selectedRecordingWork(t, peer)
			assertRecordingWorkEqual(t, before, after)
			assertRecordingResumeMetrics(t, process, peer, metrics)
			if kind != "success" {
				assertRecordingResumeRejected(t, process, dir, kind)
			}
			assertRecordingWorkEqual(t, after, selectedRecordingWork(t, peer))
			assertRecordingResumeMetrics(t, process, peer, metrics)
			if runner.CallCount() != 1 {
				t.Fatalf("provider calls = %d, want one source dispatch and no rejected/resumed dispatch", runner.CallCount())
			}
			peer.command.Stop(t)
			if kind == "success" {
				replay := startRecordingResumeSession(t, process, dir, t.TempDir(), "--replay", sourcePath, "--no-record", "--runtime-metrics-dir", metricsRoot)
				support.WaitForSessionTerminalStatus(t, replay.url, replay.id, resumeFromRecordingTimeout)
				assertRecordingWorkEqual(t, before, selectedRecordingWork(t, replay))
				replay.command.Stop(t)
				if runner.CallCount() != 1 {
					t.Fatal("replay dispatched the recorded Work again")
				}
			}
			if !bytes.Equal(original, readRecordingResumeFile(t, sourcePath)) {
				t.Fatal("resume changed the source recording")
			}
		})
	}
}

type recordingResumeServerKey struct{}
type recordingResumeServer struct {
	api     *support.ProcessAPIServer
	started chan struct{}
}
type recordingResumeRoutes struct{ runners *sync.Map }

func (routes recordingResumeRoutes) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner, ok := routes.runners.Load(filepath.Clean(request.WorkDir))
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("no recording scenario provider route")
	}
	return runner.(*support.RecordingCommandRunner).Run(ctx, request)
}

type recordingResumeSession struct {
	id, url, dir string
	command      *support.ProcessCommand
}

func recordingResumeInputs(t *testing.T, dir, home string, args ...string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "run", "--dir", dir,
		"--session", uuid.NewString(), "--quiet"}, args...))
	inputs.WorkingDirectory = dir
	inputs.Env = []string{"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	return inputs
}

func startRecordingResumeSession(t *testing.T, process support.ApplicationProcess, dir, home string, args ...string) recordingResumeSession {
	t.Helper()
	inputs := recordingResumeInputs(t, dir, home, append([]string{"--continuously", "--with-server", "--listen", "127.0.0.1:1"}, args...)...)
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Env, dir)
	server := support.NewProcessAPIServer()
	started := make(chan struct{})
	inputs.Context = context.WithValue(inputs.Context, recordingResumeServerKey{}, recordingResumeServer{api: server, started: started})
	command := support.StartProcessCommand(t, process, inputs.Input)
	select {
	case <-started:
	case <-command.Done():
		command.AcceptError()
		t.Fatalf("resume host exited before binding: %v; stderr=%s", command.Err(), inputs.Stderr())
	case <-time.After(resumeFromRecordingTimeout):
		t.Fatal("resume host did not reach the transport boundary")
	}
	session := recordingResumeSession{id: inputs.Args[5], url: server.WaitForURL(t), dir: dir, command: command}
	return session
}

func selectedRecordingWork(t *testing.T, session recordingResumeSession) factoryapi.Work {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, session.url+"/factory-sessions/"+session.id)
	selected, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode selected Factory Session: %v", err)
	}
	if selected.Id != session.id || filepath.Clean(selected.FactoryDir) != filepath.Clean(session.dir) {
		t.Fatalf("restored Factory Session = %#v, want selected identity/Factory", selected)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(session.url, session.id, "/work"))
	if len(listed.Results) != 1 || support.WorkItemCustomerLocation(listed.Results[0]) != support.WorkCustomerLocation("task", "complete") {
		t.Fatalf("selected restored Work = %#v, want one completed Work", listed.Results)
	}
	return listed.Results[0]
}

func assertRecordingWorkEqual(t *testing.T, before, after factoryapi.Work) {
	t.Helper()
	// Recording flush can promote UNCONFIRMED to CONFIRMED during a public read.
	// That durability update does not change the recovered Work identity/content.
	before.ConfirmationState, after.ConfirmationState = nil, nil
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("recorded Work changed: before=%#v after=%#v", before, after)
	}
}

func selectedRecordingMetrics(t *testing.T, process support.ApplicationProcess, session recordingResumeSession) factoryapi.MetricsReport {
	t.Helper()
	inputs := recordingResumeInputs(t, session.dir, t.TempDir())
	inputs.Args = []string{"you", "metrics", "--server", session.url, "--session", session.id, "--json"}
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("selected metrics read: %v; stderr=%s", err, inputs.Stderr())
	}
	var document struct {
		Scope struct {
			FactorySessionID string `json:"factory_session_id"`
		} `json:"scope"`
		Totals struct {
			InputTokens         float64 `json:"input_tokens"`
			OutputTokens        float64 `json:"output_tokens"`
			CompletedDispatches float64 `json:"completed_dispatches"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
		t.Fatalf("decode selected metrics: %v; stdout=%s", err, inputs.Stdout())
	}
	if document.Scope.FactorySessionID != session.id {
		t.Fatalf("CLI metrics scope = %s, want %s", document.Scope.FactorySessionID, session.id)
	}
	report := support.GetJSON[factoryapi.MetricsReport](t, session.url+"/metrics?session_id="+url.QueryEscape(session.id))
	if document.Totals.InputTokens != report.Totals.InputTokens || document.Totals.OutputTokens != report.Totals.OutputTokens ||
		document.Totals.CompletedDispatches != report.Totals.CompletedDispatches {
		t.Fatalf("CLI/API metrics differ: CLI=%#v API=%#v", document.Totals, report.Totals)
	}
	if report.Scope.FactorySessionId == nil || *report.Scope.FactorySessionId != session.id {
		t.Fatalf("metrics scope = %#v, want %s", report.Scope, session.id)
	}
	return report
}

func assertRecordingResumeMetrics(t *testing.T, process support.ApplicationProcess, session recordingResumeSession, source factoryapi.MetricsReport) {
	t.Helper()
	got := selectedRecordingMetrics(t, process, session)
	if got.Totals.InputTokens != source.Totals.InputTokens || got.Totals.OutputTokens != source.Totals.OutputTokens ||
		got.Totals.CompletedDispatches != source.Totals.CompletedDispatches || !reflect.DeepEqual(got.UsageRows, source.UsageRows) {
		t.Fatalf("resumed metrics lost source facts: source=%#v got=%#v", source, got)
	}
}

func assertRecordingResumeRejected(t *testing.T, process support.ApplicationProcess, dir, kind string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "rejected.json")
	var payload []byte
	switch kind {
	case "malformed":
		payload = []byte(`{"private":"PRIVATE-RECORDING-CONTENT",`)
	case "unsupported version":
		payload = []byte(`{"recordingKind":"you.factory-session.javascript.recording","schemaVersion":"2","replayCompatibilityVersion":"99"}`)
	}
	if payload != nil {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inputs := recordingResumeInputs(t, dir, home, "--resume", path, "--record", filepath.Join(home, "successor.json"))
	err := process.Execute(inputs.Input)
	if err == nil || inputs.Stdout() != "" {
		t.Fatalf("rejected resume err=%v stdout=%q", err, inputs.Stdout())
	}
	assertRecordingResumeDiagnostic(t, kind, err, inputs.Stderr())
	if kind == "missing" {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("missing source changed: %v", statErr)
		}
	} else if !bytes.Equal(payload, readRecordingResumeFile(t, path)) {
		t.Fatal("rejected resume changed source bytes")
	}
}

func assertRecordingResumeDiagnostic(t *testing.T, kind string, err error, stderr string) {
	t.Helper()
	if strings.Contains(stderr, "PRIVATE-RECORDING-CONTENT") || strings.Contains(err.Error(), "PRIVATE-RECORDING-CONTENT") {
		t.Fatal("rejected recording leaked private content")
	}
	var diagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.Split(strings.TrimSpace(stderr), "\n")[0]), &diagnostic); decodeErr != nil {
		t.Fatalf("decode rejected-resume diagnostic: %v; stderr=%s", decodeErr, stderr)
	}
	wantCode := "REPLAY_ARTIFACT_DEPENDENCY_FAILURE"
	if kind == "unsupported version" {
		wantCode = "UNSUPPORTED_REPLAY_COMPATIBILITY_VERSION"
	}
	if string(diagnostic.Code) != wantCode || diagnostic.Family != factoryapi.ErrorFamilyBadRequest || diagnostic.Message == "" {
		t.Fatalf("resume diagnostic = %#v, want %s/BAD_REQUEST with actionable message", diagnostic, wantCode)
	}
	if kind == "unsupported version" {
		var rejected *recordings.ReplayInputError
		if !errors.As(err, &rejected) || rejected.Diagnostic.Code != recordings.ReplayArtifactDiagnosticUnsupportedVersion {
			t.Fatalf("unsupported-version error = %v, want typed compatibility rejection", err)
		}
	}
	if kind == "missing" && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing input error = %v, want not-found cause", err)
	}
	if kind == "malformed" {
		var syntax *json.SyntaxError
		if !errors.As(err, &syntax) {
			t.Fatalf("malformed input error = %v, want JSON syntax cause", err)
		}
	}
}

func readRecordingResumeFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

const (
	metricsLineageInputTokens           int64 = 1_000_000
	metricsLineageOutputTokens          int64 = 2_000_000
	metricsLineageProviderLatencyMillis int64 = 1
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
	sourceProvider := newMetricsLineageProvider()
	source := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--record", sourceRecording},
		Edges:                     serviceedges.Edges{ProviderOverride: sourceProvider},
	})

	submitted := support.SubmitDefaultSessionWork(t, source.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPointer("single-dispatch-metrics-resume"),
		Payload:      map[string]any{"subject": "one-lineage"},
		WorkTypeName: "task",
	})
	if submitted.Accepted != true || submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("source Work submission = %#v, want one accepted Work identity", submitted)
	}
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
	if got := sourceProvider.CallCount(); got != 1 {
		t.Fatalf("source provider calls = %d, want exactly one", got)
	}

	source.Close(t)
	if _, err := os.Stat(sourceRecording); err != nil {
		t.Fatalf("source recording after close: %v", err)
	}

	successorProvider := testutil.NewNativeMockProvider()
	successor := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--resume", sourceRecording, "--record", successorRecording},
		Edges:                     serviceedges.Edges{ProviderOverride: successorProvider},
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
	if got := successorProvider.CallCount(); got != 0 {
		t.Fatalf("successor provider calls = %d, want zero redispatches", got)
	}
}

func newMetricsLineageProvider() *testutil.NativeMockProvider {
	// Return provider duration as a diagnostic fact instead of waiting for host
	// time. The live/resume assertion needs a positive latency sample, not a
	// scheduling-dependent wall-clock interval.
	return testutil.NewNativeMockProvider(providers.ExecuteResult{
		Content: "single dispatch COMPLETE",
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: metricsLineageProviderLatencyMillis,
			Metadata: map[string]string{
				workerexecution.ProviderResponseMetadataCompletionEvidence: "provider_response",
				workerexecution.ProviderResponseMetadataDurationMS:         strconv.FormatInt(metricsLineageProviderLatencyMillis, 10),
				workerexecution.ProviderResponseMetadataInputTokens:        strconv.FormatInt(metricsLineageInputTokens, 10),
				workerexecution.ProviderResponseMetadataOutputTokens:       strconv.FormatInt(metricsLineageOutputTokens, 10),
			},
		},
	})
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
