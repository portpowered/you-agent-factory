package root_composition_test

import (
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRecordingsPortableBuildValidateAndTransportsActivateThroughRootBuildProcessAfterLifecycle
// proves portable recording build/validate and Recordings HTTP/CLI transport
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
	edges := serviceedges.Edges{
		RecordingMakeDirectories: os.MkdirAll,
		RecordingCreateTempFile: func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		RecordingRemovePath: os.Remove,
		RecordingRenamePath: os.Rename,
		RecordingReadFile:   os.ReadFile,
	}
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
	cliFactoryDir := support.ScaffoldFactory(t, recordingsTransportActivationFactoryConfig())
	testutil.WriteSeedFile(t, cliFactoryDir, "task", []byte(`{"title":"FUN Recordings CLI transport activation"}`))
	assertRecordingsCLITransportRecordPathActivates(t, cliFactoryDir, cliArtifactPath)

	recordingsService := recordingsTransportActivationService(t, edges)
	assertRecordingsPortableValidateAdverseOutcome(t, recordingsService)
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
	inputs := support.FakeInputs(t.Context(), args)
	home := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
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

	var service recordings.Service
	edges.RecordingsRootObserver = func(root recordings.Service) { service = root }
	_ = support.BuildProcess(t, edges)
	if service == nil {
		t.Fatal("canonical Recordings root was not observed")
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
