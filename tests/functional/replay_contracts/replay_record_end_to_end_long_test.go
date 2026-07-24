//go:build functionallong

package replay_contracts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const recordReplayLiveScriptEnv = "AGENT_FACTORY_RECORD_REPLAY_LIVE_SCRIPT"
const recordReplayScriptSecretEnv = "SCRIPT_REPLAY_API_TOKEN"
const recordReplayScriptSecretValue = "raw-script-replay-secret-value"
const recordReplayProviderSecretEnv = "ANTHROPIC_API_KEY"
const recordReplayProviderSecretValue = "raw-provider-replay-secret-value"

func setRecordReplayHomeEnv(t *testing.T, homeDir string) {
	t.Helper()

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	t.Setenv("HOMEPATH", string(os.PathSeparator))
}

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
	recordOutput, err := runRecordReplayCLIWithCapturedStdout(
		t,
		dir,
		"--work", workFile,
		"--record", artifactPath,
	)
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

	replayOutput, err := runRecordReplayCLIWithCapturedStdout(
		t,
		t.TempDir(),
		"--replay", artifactPath,
		"--no-record",
	)
	if err != nil {
		t.Fatalf("replay run failed: %v", err)
	}
	if replayOutput != "" {
		t.Fatalf("replay run stdout = %q, want empty output with dashboard rendering suppressed", replayOutput)
	}

	replay := observeReplayThroughRoot(t, artifactPath, 10*time.Second)
	assertReplayPlaceCounts(t, replay.Work, map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})
}

func TestRecordReplayEndToEnd_DefaultLiveRecordingPathReplaysThroughExistingFlow(t *testing.T) {
	support.SkipLongFunctional(t, "slow default record/replay CLI end-to-end smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	helperPath := writeRecordReplayScriptHelper(t)
	writeRecordReplayScriptWorker(t, dir, helperPath)

	workFile := filepath.Join(t.TempDir(), "initial-work.json")
	writeRecordReplayWorkFile(t, workFile)

	homeDir := t.TempDir()
	setRecordReplayHomeEnv(t, homeDir)
	t.Setenv(recordReplayLiveScriptEnv, "1")
	t.Setenv(recordReplayScriptSecretEnv, recordReplayScriptSecretValue)

	_, err := runRecordReplayCLIWithCapturedStdout(t, dir, "--work", workFile)
	if err != nil {
		t.Fatalf("default record run failed: %v", err)
	}

	wantRoot := filepath.Join(homeDir, ".you-agent-factory", "recordings")
	artifactPath := singleRecordedArtifactPath(t, wantRoot)
	if !strings.HasPrefix(artifactPath, wantRoot+string(os.PathSeparator)) {
		t.Fatalf("artifact path = %q, want root under %q", artifactPath, wantRoot)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected default-recorded artifact to contain at least one dispatch")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("expected default-recorded artifact to contain at least one completion")
	}

	if err := os.Unsetenv(recordReplayLiveScriptEnv); err != nil {
		t.Fatalf("unset live script env: %v", err)
	}
	if err := os.Unsetenv(recordReplayScriptSecretEnv); err != nil {
		t.Fatalf("unset script secret env: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture dir: %v", err)
	}

	replayOutput, err := runRecordReplayCLIWithCapturedStdout(
		t,
		t.TempDir(),
		"--replay", artifactPath,
		"--no-record",
	)
	if err != nil {
		t.Fatalf("replay run failed: %v", err)
	}
	if replayOutput != "" {
		t.Fatalf("replay run stdout = %q, want empty output with dashboard rendering suppressed", replayOutput)
	}

	replay := observeReplayThroughRoot(t, artifactPath, 10*time.Second)
	assertReplayPlaceCounts(t, replay.Work, map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})
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
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
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

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), recordReplayExternalBatchWorkRequest())
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, "request-replay-external-batch", "external-submit", 2, 1)
	generatedRequest := findReplayWorkRequestBySourcePrefix(t, artifact, "worker-output:")
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
	assertGeneratedReplayRequestMetadata(
		t,
		testutil.GeneratedFactoryEvents(t, artifact.Events),
		generatedRequest.RequestID,
	)

	replay := observeReplayThroughRoot(t, artifactPath, 10*time.Second)
	assertReplayPlaceCounts(t, replay.Work, map[string]int{
		"task:complete": 3, "task:init": 0, "task:failed": 0,
	})
	if !replayWorkIncludesID(replay.Work, "work-generated-alpha") ||
		!replayWorkIncludesID(replay.Work, "work-generated-beta") {
		t.Fatalf("replay Work listing missing generated alpha/beta: %#v", replay.Work.Results)
	}
	assertGeneratedReplayRequestMetadata(t, replay.Events, "")
}

func recordReplayExternalBatchWorkRequest() factoryapi.WorkRequest {
	return factoryapi.WorkRequest{
		RequestId: "request-replay-external-batch",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "external-first",
				WorkId:       strPtr("work-external-first"),
				WorkTypeName: strPtr("task"),
				TraceId:      strPtr("trace-replay-batch"),
				Payload:      "external first",
			},
			{
				Name:         "external-fanout",
				WorkId:       strPtr("work-external-fanout"),
				WorkTypeName: strPtr("task"),
				TraceId:      strPtr("trace-replay-batch"),
				Payload:      "external fanout",
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "external-fanout",
			TargetWorkName: "external-first",
		}},
	}
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
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "provider-replay-env-work",
		TraceID:    "provider-replay-env-trace",
		Payload:    []byte("exercise provider replay env redaction"),
	})

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Step one done. COMPLETE","session_id":"provider-replay-1"}` + "\n")},
		platformprocess.CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Step two done. COMPLETE","session_id":"provider-replay-2"}` + "\n")},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	assertReplayPlaceCounts(t, support.GetDefaultSession(t, server.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
	server.Stop(t)

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

func findReplayWorkRequestBySourcePrefix(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
	sourcePrefix string,
) *recordedFactoryWorkRequestEvent {
	t.Helper()
	for _, event := range replayWorkRequestEvents(t, artifact) {
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
	works := factoryWorksValue(record.Payload.Works)
	if len(works) != 2 ||
		!factoryWorksIncludeID(works, "work-generated-alpha") ||
		!factoryWorksIncludeID(works, "work-generated-beta") {
		t.Fatalf("generated WORK_REQUEST works = %#v, want generated alpha and beta", works)
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
	return replayWorkRequestEventsFromEvents(t, testutil.GeneratedFactoryEvents(t, artifact.Events))
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

	req := work.SubmitRequest{
		WorkID:     "record-replay-e2e-work",
		WorkTypeID: "task",
		TraceID:    "record-replay-e2e-trace",
		Payload:    []byte("exercise end-to-end record/replay"),
	}
	support.WriteWorkRequestFile(t, path, req)
}

func runRecordReplayCLIWithCapturedStdout(
	t *testing.T,
	workingDirectory string,
	flags ...string,
) (string, error) {
	t.Helper()

	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: api.Start,
	})
	args := []string{"you", "run", "--dir", workingDirectory, "--server", "http://127.0.0.1:1", "--quiet"}
	args = append(args, flags...)
	inputs := support.FakeInputs(context.Background(), args)
	inputs.WorkingDirectory = workingDirectory
	runErr := process.Execute(inputs.Input)
	return inputs.Stdout(), runErr
}

func singleRecordedArtifactPath(t *testing.T, root string) string {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk recording root %s: %v", root, err)
	}
	if len(paths) != 1 {
		t.Fatalf("recording artifacts under %s = %#v, want exactly one", root, paths)
	}
	return paths[0]
}

func recordedPathFromCLIOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, "Recording saved: "); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("startup output = %q, want recording path", output)
	return ""
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

func commandEnvContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
