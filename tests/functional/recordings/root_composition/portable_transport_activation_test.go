package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRecordingsPortableBuildValidateAndTransportsActivateThroughRootBuildProcessAfterLifecycle
// proves portable recording build/validate and Recordings HTTP/CLI/MCP transport
// surfaces activate only after runtime lifecycle on a process constructed only
// through root.BuildProcess. Deeper transport unit/adapter coverage remains
// under pkg/services/recordings/transports and replay_contracts; this test
// closes the explicit public-process activation gap for the Recordings FUN suite
// home.
func TestRecordingsPortableBuildValidateAndTransportsActivateThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, recordingsTransportActivationFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"FUN Recordings transport activation"}`))

	recordArtifactPath := filepath.Join(t.TempDir(), "fun-recordings-transport-activation.replay.json")
	edges := serviceedges.Edges{}
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", recordArtifactPath},
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { recordServer.Stop(t) })

	baseURL := recordServer.URL()
	support.WaitForTerminalStatus(t, baseURL, 15*time.Second)
	waitForRecordingsActivationArtifact(t, recordArtifactPath)

	events := support.GetFactoryEventsAt(t, baseURL)
	if recordingsActivationLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("Recordings HTTP events transport missing dispatch response events after lifecycle")
	}

	durableSession := startRecordingsSurfaceActivationDurableSession(t, baseURL)
	artifactList := support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](
		t,
		recordingsSurfaceActivationSessionEndpoint(baseURL, durableSession.SessionId)+"/artifacts",
	)
	if len(artifactList.Artifacts) == 0 {
		t.Fatalf("Recordings HTTP artifact transport list = %#v, want artifacts after lifecycle", artifactList.Artifacts)
	}
	artifactDetail := support.GetJSON[factoryapi.FactorySessionArtifactDetail](
		t,
		recordingsSurfaceActivationSessionEndpoint(baseURL, durableSession.SessionId)+
			"/artifacts/"+artifactList.Artifacts[0].Id,
	)
	if artifactDetail.ContentHash == nil || strings.TrimSpace(*artifactDetail.ContentHash) == "" {
		t.Fatalf("Recordings HTTP artifact transport detail = %#v, want non-empty content hash after lifecycle", artifactDetail)
	}

	cliArtifactPath := filepath.Join(t.TempDir(), "fun-recordings-cli-transport.replay.json")
	assertRecordingsCLITransportRecordPathActivates(t, dir, cliArtifactPath)

	recordingsService := recordingsTransportActivationService(t, edges)
	assertRecordingsPortableValidateAdverseOutcome(t, recordingsService)
	assertRecordingsMCPTransportActivatesAfterLifecycle(t, recordingsService, durableSession.SessionId)
}

func recordingsTransportActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "recordings-transport-activation",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "mock-worker"}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "mock-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func assertRecordingsCLITransportRecordPathActivates(t *testing.T, factoryDir, artifactPath string) {
	t.Helper()

	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{APIServerStarter: api.Start})
	args := []string{
		"you", "run",
		"--dir", factoryDir,
		"--with-mock-workers",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--record", artifactPath,
	}
	inputs := support.FakeInputs(context.Background(), args)
	inputs.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Recordings CLI transport Process.Execute(run --record) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	adapter := recordingscli.New()
	resolved, err := adapter.ResolveRecordPath(recordingscli.InvocationRequest{
		RecordPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("Recordings CLI adapter ResolveRecordPath() error = %v", err)
	}
	if resolved.ServicePath != artifactPath {
		t.Fatalf("Recordings CLI adapter ServicePath = %q, want %q", resolved.ServicePath, artifactPath)
	}

	waitForRecordingsActivationArtifact(t, artifactPath)
}

type recordingsTransportActivationLedger struct {
	recordings.Ledger
}

func recordingsTransportActivationService(
	t *testing.T,
	edges serviceedges.Edges,
) recordings.Service {
	t.Helper()

	_ = support.BuildProcess(t, edges)
	service, err := recordingswire.NewService(
		&recordingsTransportActivationLedger{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		func(path string, payload []byte) error {
			return os.WriteFile(path, payload, 0o600)
		},
	)
	if err != nil {
		t.Fatalf("compose Recordings service for transport activation: %v", err)
	}
	return service
}

func assertRecordingsPortableValidateAdverseOutcome(t *testing.T, service recordings.Service) {
	t.Helper()

	if _, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf(
			"DecodePortableArtifact malformed payload error = %v, want %v",
			err,
			recordings.ErrInvalidPortableArtifact,
		)
	}
}

func assertRecordingsMCPTransportActivatesAfterLifecycle(
	t *testing.T,
	service recordings.Service,
	recordingID string,
) {
	t.Helper()

	ctx := context.Background()
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: service})

	malformedRaw, err := operation(ctx, mcprecording.ToolReadPortableArtifact, json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("Recordings MCP transport CallTool malformed input error = %v", err)
	}
	assertRecordingsMCPBadRequestToolResponse(t, malformedRaw)

	missingRaw, err := operation(
		ctx,
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":"`+recordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("Recordings MCP transport CallTool(load_replay) error = %v", err)
	}
	assertRecordingsMCPReplayNotFoundToolResponse(t, missingRaw)
}

func assertRecordingsMCPBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()

	var response mcprecording.ToolResponse[recordings.ReadPortableArtifactResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode MCP bad-request tool response: %v\n%s", err, raw)
	}
	if response.Error == nil {
		t.Fatalf("MCP malformed input response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("MCP malformed input error code = %q, want BAD_REQUEST", response.Error.Code)
	}
}

func assertRecordingsMCPReplayNotFoundToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()

	var response mcprecording.ToolResponse[recordings.LoadReplayRecordingResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode MCP load_replay tool response: %v\n%s", err, raw)
	}
	if response.Error == nil {
		t.Fatalf("MCP load_replay response = %s, want typed not-found envelope", raw)
	}
	if response.Error.Code != "recording.replay.not_found" {
		t.Fatalf("MCP load_replay error code = %q, want recording.replay.not_found", response.Error.Code)
	}
}
