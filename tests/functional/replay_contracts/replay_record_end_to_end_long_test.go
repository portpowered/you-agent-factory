//go:build functionallong

package replay_contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/service"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

const recordReplayLiveScriptEnv = "AGENT_FACTORY_RECORD_REPLAY_LIVE_SCRIPT"
const recordReplayScriptSecretEnv = "SCRIPT_REPLAY_API_TOKEN"
const recordReplayScriptSecretValue = "raw-script-replay-secret-value"

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

	var startup bytes.Buffer
	if err := runcli.Run(context.Background(), runcli.RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		SuppressDashboardRendering: true,
		StartupOutput:              &startup,
		Logger:                     zap.NewNop(),
	}); err != nil {
		t.Fatalf("default record run failed: %v", err)
	}

	artifactPath := recordedPathFromCLIOutput(t, startup.String())
	wantRoot := filepath.Join(homeDir, ".you-agent-factory", "recordings")
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

}

func TestRecordReplayEndToEnd_PublicRecordingPreservesExternalAndGeneratedWorkRequestLineage(t *testing.T) {
	support.SkipLongFunctional(t, "slow public record/replay generated-batch smoke")

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
		RecordPath:                artifactPath,
		WaitForServiceModeRuntime: true,
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.ProviderOverride = provider
			cfg.SkipBuiltInRunnerPrerequisiteValidation = true
		},
	})
	requestID := "request-replay-external-batch"
	response := upsertFactoryWorkRequestOverHTTP(t, server.URL(), requestID, []byte(`{
		"requestId":"request-replay-external-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"external-first","workId":"work-external-first","workTypeName":"task","traceId":"trace-replay-batch","payload":"external first"},
			{"name":"external-fanout","workId":"work-external-fanout","workTypeName":"task","traceId":"trace-replay-batch","payload":"external fanout"}
		],
		"relations":[{"type":"DEPENDS_ON","sourceWorkName":"external-fanout","targetWorkName":"external-first"}]
	}`))
	if response.RequestId != requestID || len(response.Works) != 2 {
		t.Fatalf("public work-request response = %#v, want request id and two works", response)
	}
	waitForCondition(t, 10*time.Second, func() bool {
		return provider.CallCount("processor") >= 4 && provider.CallCount("finisher") >= 3
	})
	server.Stop(t)
	events := waitForRecordedEvents(t, artifactPath, 5*time.Second)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, requestID, "external-submit", 2, 1)
	assertGeneratedReplayRequestMetadata(t, events, "")
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
	assertGeneratedReplayRequestMetadata(t, testutil.GeneratedFactoryEvents(t, artifact.Events), generatedRequest.RequestID)
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

func recordedPathFromCLIOutput(t *testing.T, output string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, "Recording saved: "); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("startup output = %q, want auto-generated recording path", output)
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
