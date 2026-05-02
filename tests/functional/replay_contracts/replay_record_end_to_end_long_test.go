//go:build functionallong

package replay_contracts

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

const recordReplayLiveScriptEnv = "AGENT_FACTORY_RECORD_REPLAY_LIVE_SCRIPT"
const recordReplayScriptSecretEnv = "SCRIPT_REPLAY_API_TOKEN"
const recordReplayScriptSecretValue = "raw-script-replay-secret-value"
const recordReplayProviderSecretEnv = "ANTHROPIC_API_KEY"
const recordReplayProviderSecretValue = "raw-provider-replay-secret-value"

func TestRecordReplayEndToEnd_CLIRecordReplayAndRegressionHarnessSucceed(t *testing.T) {
	support.SkipLongFunctional(t, "slow record/replay CLI end-to-end smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	helperPath := writeRecordReplayScriptHelper(t)
	writeRecordReplayScriptWorker(t, dir, helperPath)

	workFile := filepath.Join(t.TempDir(), "initial-work.json")
	writeRecordReplayWorkFile(t, workFile)
	artifactPath := filepath.Join(t.TempDir(), "cli-recording.replay.json")

	t.Setenv(recordReplayLiveScriptEnv, "1")
	t.Setenv(recordReplayScriptSecretEnv, recordReplayScriptSecretValue)
	recordOutput, err := runRecordReplayCLIWithCapturedStdout(t, runcli.RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		RecordPath:                 artifactPath,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("record run failed: %v", err)
	}
	if recordOutput != "" {
		t.Fatalf("record run stdout = %q, want empty output with dashboard rendering suppressed", recordOutput)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected recorded artifact to contain at least one dispatch")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("expected recorded artifact to contain at least one completion")
	}
	assertReplayArtifactDoesNotContainRawValue(t, artifactPath, recordReplayScriptSecretValue)
	assertReplayArtifactCommandEnvRedacted(t, artifact, recordReplayScriptSecretEnv)

	if err := os.Unsetenv(recordReplayLiveScriptEnv); err != nil {
		t.Fatalf("unset live script env: %v", err)
	}
	if err := os.Unsetenv(recordReplayScriptSecretEnv); err != nil {
		t.Fatalf("unset script secret env: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture dir: %v", err)
	}

	replayOutput, err := runRecordReplayCLIWithCapturedStdout(t, runcli.RunConfig{
		Dir:                        t.TempDir(),
		Port:                       0,
		ReplayPath:                 artifactPath,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("replay run failed: %v", err)
	}
	if replayOutput != "" {
		t.Fatalf("replay run stdout = %q, want empty output with dashboard rendering suppressed", replayOutput)
	}

	h := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	h.Service.Assert().
		HasTokenInPlace("task:done").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// portos:func-length-exception owner=agent-factory reason=record-replay-e2e-fixture review=2026-07-18 removal=split-record-run-replay-run-and-artifact-assertions-before-next-record-replay-change
func TestRecordReplayEndToEnd_FactoryRequestBatchAndWorkerGeneratedBatchReplayDeterministically(t *testing.T) {
	support.SkipLongFunctional(t, "slow record/replay generated-batch determinism smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "factory_request_batch"))
	artifactPath := filepath.Join(t.TempDir(), "batch-recording.replay.json")

	support.WriteAgentConfig(t, dir, "processor", `---
type: MODEL_WORKER
model: test-model
---
Process the input task.
`)
	support.WriteAgentConfig(t, dir, "finisher", `---
type: MODEL_WORKER
model: test-model
---
Finish the input task.
`)

	generatedBatchOutput := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"generated-alpha","workId":"work-generated-alpha","workTypeName":"task","payload":"generated alpha"},{"name":"generated-beta","workId":"work-generated-beta","workTypeName":"task","payload":"generated beta"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"generated-beta","targetWorkName":"generated-alpha","requiredState":"complete"}]},"metadata":{"parentLineage":["request-replay-external-batch","work-external-fanout"],"relationContext":[{"type":"DEPENDS_ON","sourceWorkName":"generated-beta","targetWorkName":"generated-alpha","requiredState":"complete"}]}}`
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "record external first"},
			{Content: generatedBatchOutput},
			{Content: "record generated alpha"},
			{Content: "record generated beta"},
		},
		"finisher": {
			{Content: "finish external first"},
			{Content: "finish generated alpha"},
			{Content: "finish generated beta"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-replay-external-batch",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "external-first", WorkID: "work-external-first", WorkTypeID: "task", TraceID: "trace-replay-batch", Payload: "external first"},
			{Name: "external-fanout", WorkID: "work-external-fanout", WorkTypeID: "task", TraceID: "trace-replay-batch", Payload: "external fanout"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "external-fanout",
			TargetWorkName: "external-first",
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, "request-replay-external-batch", "external-submit", 2, 1)
	generatedRequest := findReplayWorkRequestBySourcePrefix(artifact, "worker-output:")
	if generatedRequest == nil {
		t.Fatalf("replay artifact did not record worker-generated work request: %#v", replayWorkRequestEvents(t, artifact))
	}
	if generatedRequest.RequestID == "" || !strings.HasPrefix(generatedRequest.RequestID, "generated-request-") {
		t.Fatalf("generated request_id = %q, want deterministic generated-request-*", generatedRequest.RequestID)
	}
	if got := len(factoryWorksValue(generatedRequest.Payload.Works)); got != 2 {
		t.Fatalf("generated work items = %d, want 2", got)
	}
	if got := len(factoryRelationsValue(generatedRequest.Payload.Relations)); got != 1 {
		t.Fatalf("generated relations = %d, want 1", got)
	}
	assertGeneratedReplayRequestMetadata(t, artifact.Events, generatedRequest.RequestID)

	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("task:complete", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	snapshot, err := replayHarness.Service.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after replay: %v", err)
	}
	if !snapshotContainsWorkID(snapshot, "work-generated-alpha") || !snapshotContainsWorkID(snapshot, "work-generated-beta") {
		t.Fatalf("replay snapshot missing generated work tokens for alpha/beta")
	}
	replayEvents, err := replayHarness.Service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents after replay: %v", err)
	}
	assertGeneratedReplayRequestMetadata(t, replayEvents, "")
}

func TestRecordReplayEndToEnd_ProviderCommandDiagnosticsPersistRedactedEnv(t *testing.T) {
	support.SkipLongFunctional(t, "slow record/replay provider diagnostics smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "provider-recording.replay.json")
	t.Setenv(recordReplayProviderSecretEnv, recordReplayProviderSecretValue)

	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	support.WriteAgentConfig(t, dir, "worker-b", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Finish the input task.
`)
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "provider-replay-env-work",
		TraceID:    "provider-replay-env-trace",
		Payload:    []byte("exercise provider replay env redaction"),
	})

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Step one done. COMPLETE")},
		workers.CommandResult{Stdout: []byte("Step two done. COMPLETE")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)

	h.RunUntilComplete(t, 10*time.Second)
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() == 0 {
		t.Fatal("expected provider command runner to be called")
	}
	if !commandEnvContains(runner.LastRequest().Env, recordReplayProviderSecretEnv+"="+recordReplayProviderSecretValue) {
		t.Fatalf("provider command env did not receive raw %s", recordReplayProviderSecretEnv)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayArtifactDoesNotContainRawValue(t, artifactPath, recordReplayProviderSecretValue)
	assertReplayArtifactCommandEnvRedacted(t, artifact, recordReplayProviderSecretEnv)
}

func assertReplayWorkRequestRecorded(t *testing.T, artifact *interfaces.ReplayArtifact, requestID, source string, workItems int, relations int) {
	t.Helper()

	for _, record := range replayWorkRequestEvents(t, artifact) {
		if record.RequestID != requestID {
			continue
		}
		if record.Source != source {
			t.Fatalf("work request %s source = %q, want %q", requestID, record.Source, source)
		}
		if got := len(factoryWorksValue(record.Payload.Works)); got != workItems {
			t.Fatalf("work request %s work items = %d, want %d", requestID, got, workItems)
		}
		if got := len(factoryRelationsValue(record.Payload.Relations)); got != relations {
			t.Fatalf("work request %s relations = %d, want %d", requestID, got, relations)
		}
		return
	}
	t.Fatalf("replay artifact missing work request %s: %#v", requestID, replayWorkRequestEvents(t, artifact))
}

func findReplayWorkRequestBySourcePrefix(artifact *interfaces.ReplayArtifact, sourcePrefix string) *recordedFactoryWorkRequestEvent {
	for _, event := range replayWorkRequestEvents(nil, artifact) {
		if strings.HasPrefix(event.Source, sourcePrefix) {
			return &event
		}
	}
	return nil
}

func assertGeneratedReplayRequestMetadata(t *testing.T, events []factoryapi.FactoryEvent, requestID string) {
	t.Helper()

	record := findReplayGeneratedWorkRequest(t, events, requestID)
	if !strings.HasPrefix(record.Source, "worker-output:") {
		t.Fatalf("generated request source = %q, want worker-output source", record.Source)
	}
	if got := strings.Join(stringSlicePointerValue(record.Payload.ParentLineage), ","); got != "request-replay-external-batch,work-external-fanout" {
		t.Fatalf("generated parent lineage = %#v, want replay batch lineage", stringSlicePointerValue(record.Payload.ParentLineage))
	}
	relations := factoryRelationsValue(record.Payload.Relations)
	if len(relations) != 1 ||
		relations[0].SourceWorkName != "generated-beta" ||
		relations[0].TargetWorkName != "generated-alpha" ||
		stringPointerValue(relations[0].RequiredState) != "complete" {
		t.Fatalf("generated relation metadata = %#v, want generated-beta depends on generated-alpha complete", relations)
	}

	world, err := projections.ReconstructFactoryWorldState(events, lastFactoryEventTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	replayed, ok := world.WorkRequestsByID[record.RequestID]
	if !ok {
		t.Fatalf("replayed request state missing generated request %q", record.RequestID)
	}
	if replayed.Source != record.Source {
		t.Fatalf("replayed source = %q, want %q", replayed.Source, record.Source)
	}
	if got := strings.Join(replayed.ParentLineage, ","); got != "request-replay-external-batch,work-external-fanout" {
		t.Fatalf("replayed parent lineage = %#v, want replay batch lineage", replayed.ParentLineage)
	}
	if len(replayed.WorkItems) != 2 {
		t.Fatalf("replayed generated work items = %d, want 2", len(replayed.WorkItems))
	}
}

func findReplayGeneratedWorkRequest(t *testing.T, events []factoryapi.FactoryEvent, requestID string) recordedFactoryWorkRequestEvent {
	t.Helper()

	for _, event := range replayWorkRequestEventsFromEvents(t, events) {
		if requestID != "" && event.RequestID == requestID {
			return event
		}
		if requestID == "" && strings.HasPrefix(event.Source, "worker-output:") {
			return event
		}
	}
	t.Fatalf("replay events missing generated work request %q: %#v", requestID, replayWorkRequestEventsFromEvents(t, events))
	return recordedFactoryWorkRequestEvent{}
}

type recordedFactoryWorkRequestEvent struct {
	RequestID string
	Source    string
	Payload   factoryapi.WorkRequestEventPayload
}

func replayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}
	return replayWorkRequestEventsFromEvents(t, artifact.Events)
}

func replayWorkRequestEventsFromEvents(t *testing.T, events []factoryapi.FactoryEvent) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}
	var out []recordedFactoryWorkRequestEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			if t == nil {
				panic(err)
			}
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		source := stringPointerValue(payload.Source)
		if source == "" {
			source = stringPointerValue(event.Context.Source)
		}
		out = append(out, recordedFactoryWorkRequestEvent{
			RequestID: stringPointerValue(event.Context.RequestId),
			Source:    source,
			Payload:   payload,
		})
	}
	return out
}

func lastFactoryEventTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}

func snapshotContainsWorkID(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) bool {
	if snapshot == nil {
		return false
	}
	for _, token := range snapshot.Marking.Tokens {
		if token != nil && token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func writeRecordReplayScriptHelper(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "record_replay_script_helper.go")
	source := `package main

import (
	"fmt"
	"os"
)

func main() {
	if os.Getenv("` + recordReplayLiveScriptEnv + `") != "1" {
		fmt.Fprintln(os.Stderr, "live script execution is disabled during replay")
		os.Exit(2)
	}
	if os.Getenv("` + recordReplayScriptSecretEnv + `") != "` + recordReplayScriptSecretValue + `" {
		fmt.Fprintln(os.Stderr, "script secret env was not available during record execution")
		os.Exit(3)
	}
	fmt.Print("recorded-script-output")
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write script helper: %v", err)
	}
	return filepath.ToSlash(path)
}

func writeRecordReplayScriptWorker(t *testing.T, dir, helperPath string) {
	t.Helper()

	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	content := `---
type: SCRIPT_WORKER
command: go
args:
  - run
  - "` + helperPath + `"
---
`
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write script worker AGENTS.md: %v", err)
	}
}

func writeRecordReplayWorkFile(t *testing.T, path string) {
	t.Helper()

	req := interfaces.SubmitRequest{
		WorkID:     "record-replay-e2e-work",
		WorkTypeID: "task",
		TraceID:    "record-replay-e2e-trace",
		Payload:    []byte("exercise end-to-end record/replay"),
	}
	support.WriteWorkRequestFile(t, path, req)
}

func runRecordReplayCLIWithCapturedStdout(t *testing.T, cfg runcli.RunConfig) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	readCh := make(chan []byte, 1)
	readErrCh := make(chan error, 1)
	go func() {
		data, readErr := io.ReadAll(readPipe)
		readCh <- data
		readErrCh <- readErr
	}()

	os.Stdout = writePipe
	runErr := runcli.Run(context.Background(), cfg)
	os.Stdout = oldStdout

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close captured stdout writer: %v", err)
	}
	output := <-readCh
	if err := <-readErrCh; err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatalf("close captured stdout reader: %v", err)
	}

	return string(output), runErr
}

func assertReplayArtifactDoesNotContainRawValue(t *testing.T, artifactPath, rawValue string) {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read replay artifact %s: %v", artifactPath, err)
	}
	if strings.Contains(string(data), rawValue) {
		t.Fatalf("replay artifact %s leaked raw environment value %q", artifactPath, rawValue)
	}
}

func assertReplayArtifactCommandEnvRedacted(t *testing.T, artifact *interfaces.ReplayArtifact, envKey string) {
	t.Helper()

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal replay artifact: %v", err)
	}
	if strings.Contains(string(data), envKey) || strings.Contains(string(data), workers.RedactedCommandEnvValue) {
		t.Fatalf("replay artifact leaked command env metadata for %s", envKey)
	}
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func factoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func factoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSlicePointerValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func commandEnvContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
