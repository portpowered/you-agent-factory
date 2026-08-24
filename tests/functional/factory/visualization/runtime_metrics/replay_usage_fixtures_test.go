package runtime_metrics_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type replayUsageFixture struct {
	name            string
	file            string
	sessionID       string
	dispatchID      string
	workerSessionID string
	provider        string
	model           string
	inputTokens     int64
	outputTokens    int64
	totalTokens     int64
}

func TestReplayUsageFixturesAreCanonicalAndSideEffectFree(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	fixtures := []replayUsageFixture{
		{
			name:            "priced_codex_gpt_5_codex",
			file:            filepath.Join("tests", "functional", "factory", "visualization", "runtime_metrics", "testdata", "codex-gpt-5-codex.factory-recording.v1.json"),
			sessionID:       "cost-replay-priced-session",
			dispatchID:      "cost-replay-priced-dispatch",
			workerSessionID: "cost-replay-priced-worker-session",
			provider:        "codex",
			model:           "gpt-5-codex",
			inputTokens:     1_000_000,
			outputTokens:    2_000_000,
			totalTokens:     3_000_000,
		},
		{
			name:            "unpriced_claude_sonnet_4_6",
			file:            filepath.Join("tests", "functional", "factory", "visualization", "runtime_metrics", "testdata", "claude-sonnet-4-6.factory-recording.v1.json"),
			sessionID:       "cost-replay-unpriced-session",
			dispatchID:      "cost-replay-unpriced-dispatch",
			workerSessionID: "cost-replay-unpriced-worker-session",
			provider:        "claude",
			model:           "claude-sonnet-4-6",
			inputTokens:     1_200,
			outputTokens:    300,
			totalTokens:     1_500,
		},
	}
	seen := map[string]string{}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(repoRoot, fixture.file)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture before replay: %v", err)
			}
			artifact := testutil.LoadReplayArtifact(t, path)
			assertReplayUsageFixture(t, artifact, fixture, seen)
			executeReplayUsageFixture(t, path)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture after replay: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("replay modified the committed fixture")
			}
		})
	}
}

func assertReplayUsageFixture(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
	fixture replayUsageFixture,
	seen map[string]string,
) {
	t.Helper()
	assertReplayUsageEvents(t, artifact, fixture)
	assertReplayFactorySnapshot(t, artifact)
	assertReplayWorkerSessionAssociation(t, artifact, fixture)
	assertReplayDispatchUsage(t, artifact, fixture)
	assertReplayProviderDiagnostics(t, artifact, fixture)
	assertDistinctFixtureIdentity(t, fixture, seen)
}

func assertReplayUsageEvents(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
	fixture replayUsageFixture,
) {
	t.Helper()
	if artifact.SchemaVersion != interfaces.ReplayV1SourceFormat {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, interfaces.ReplayV1SourceFormat)
	}
	if artifact.RecordedAt.IsZero() || len(artifact.Events) != 8 {
		t.Fatalf("recording metadata = recordedAt %v, events %d; want non-zero and 8 events", artifact.RecordedAt, len(artifact.Events))
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeInferenceRequest,
		interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeDispatchResponse,
		interfaces.FactoryEventTypeRunResponse,
	}
	seenIDs := map[string]bool{}
	for index, event := range artifact.Events {
		if event.SchemaVersion != interfaces.FactoryEventSchemaVersionV1 {
			t.Fatalf("event %d schema version = %q, want %q", index, event.SchemaVersion, interfaces.FactoryEventSchemaVersionV1)
		}
		if event.Type != wantTypes[index] || event.Context.Sequence != index || event.Id == "" {
			t.Fatalf("event %d = type %q, sequence %d, id %q; want %q, %d, non-empty id", index, event.Type, event.Context.Sequence, event.Id, wantTypes[index], index)
		}
		if seenIDs[event.Id] {
			t.Fatalf("event id %q is duplicated", event.Id)
		}
		seenIDs[event.Id] = true
		if event.Context.SessionID == nil || *event.Context.SessionID != fixture.sessionID {
			t.Fatalf("event %d session id = %v, want %q", index, event.Context.SessionID, fixture.sessionID)
		}
		switch event.Type {
		case interfaces.FactoryEventTypeModelRequest,
			interfaces.FactoryEventTypeModelResponse,
			interfaces.FactoryEventTypeScriptRequest,
			interfaces.FactoryEventTypeScriptResponse:
			t.Fatalf("fixture contains an external execution event: %s", event.Type)
		}
	}
}

func assertReplayFactorySnapshot(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	var run interfaces.RunRequestEventPayload
	if err := artifact.Events[0].DecodePayload(&run); err != nil {
		t.Fatalf("decode RUN_REQUEST payload: %v", err)
	}
	if run.Factory == nil {
		t.Fatal("RUN_REQUEST payload has no Factory snapshot")
	}
	snapshotJSON, err := json.Marshal(run.Factory)
	if err != nil {
		t.Fatalf("marshal Factory snapshot: %v", err)
	}
	if _, err := factoryconfigmapping.GeneratedFactoryFromOpenAPIJSON(snapshotJSON); err != nil {
		t.Fatalf("decode Factory snapshot at public boundary: %v", err)
	}
}

func assertReplayWorkerSessionAssociation(t *testing.T, artifact *interfaces.ReplayArtifact, fixture replayUsageFixture) {
	t.Helper()
	var association interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := artifact.Events[3].DecodePayload(&association); err != nil {
		t.Fatalf("decode Worker Session association payload: %v", err)
	}
	if association.WorkerSessionID != fixture.workerSessionID {
		t.Fatalf("Worker Session association = %q, want %q", association.WorkerSessionID, fixture.workerSessionID)
	}
	if artifact.Events[3].Context.DispatchID == nil || *artifact.Events[3].Context.DispatchID != fixture.dispatchID {
		t.Fatalf("association dispatch id = %v, want %q", artifact.Events[3].Context.DispatchID, fixture.dispatchID)
	}
}

func assertReplayDispatchUsage(t *testing.T, artifact *interfaces.ReplayArtifact, fixture replayUsageFixture) {
	t.Helper()
	var response workers.DispatchResponseEventPayload
	if err := artifact.Events[6].DecodePayload(&response); err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
	}
	if response.Usage == nil {
		t.Fatal("DISPATCH_RESPONSE has no usage payload")
	}
	assertUsageValue(t, "input tokens", response.Usage.InputTokens, fixture.inputTokens)
	assertUsageValue(t, "output tokens", response.Usage.OutputTokens, fixture.outputTokens)
	assertUsageValue(t, "total tokens", response.Usage.TotalTokens, fixture.totalTokens)
}

func assertReplayProviderDiagnostics(t *testing.T, artifact *interfaces.ReplayArtifact, fixture replayUsageFixture) {
	t.Helper()
	var inference workers.InferenceResponseEventPayload
	if err := artifact.Events[5].DecodePayload(&inference); err != nil {
		t.Fatalf("decode INFERENCE_RESPONSE payload: %v", err)
	}
	diagnostics, err := workers.WorkDiagnosticsFromSafeEventPayload(inference.Diagnostics)
	if err != nil {
		t.Fatalf("decode replay-safe provider diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("INFERENCE_RESPONSE has no provider diagnostics")
	}
	if diagnostics.Provider.Provider != fixture.provider || diagnostics.Provider.Model != fixture.model {
		t.Fatalf("provider diagnostics = %#v, want %s/%s", diagnostics.Provider, fixture.provider, fixture.model)
	}
}

func assertDistinctFixtureIdentity(t *testing.T, fixture replayUsageFixture, seen map[string]string) {
	t.Helper()
	for label, value := range map[string]string{
		"session":        fixture.sessionID,
		"dispatch":       fixture.dispatchID,
		"worker session": fixture.workerSessionID,
	} {
		if previous, exists := seen[value]; exists {
			t.Fatalf("%s id %q is reused by %s and %s fixtures", label, value, previous, fixture.name)
		}
		seen[value] = fixture.name
	}
}

func assertUsageValue(t *testing.T, label string, value *int64, want int64) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s = nil, want %d", label, want)
	}
	if *value != want {
		t.Fatalf("%s = %d, want %d", label, *value, want)
	}
}

func executeReplayUsageFixture(t *testing.T, fixturePath string) {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	api := support.NewProcessAPIServer()
	providerRunner := support.NewRecordingCommandRunner("unexpected provider execution")
	scriptRunner := support.NewRecordingCommandRunner("unexpected script execution")
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: providerRunner,
		ScriptCommandRunner:   scriptRunner,
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"run",
		"--dir",
		workingDirectory,
		"--server",
		"http://127.0.0.1:1",
		"--quiet",
		"--replay",
		fixturePath,
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you run --replay failed: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("you run --replay stdout = %q, want empty output", inputs.Stdout())
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0", providerRunner.CallCount())
	}
	if scriptRunner.CallCount() != 0 {
		t.Fatalf("script command runner calls = %d, want 0", scriptRunner.CallCount())
	}
}
