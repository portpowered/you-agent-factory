package lifecycle_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each immutable recording mode owns a host/profile. Its two API peers share
// that root process and prove the API admission contract against CLI selection.
func TestPeerRecordingRetainedAcrossCloseAndHostStop(t *testing.T) {
	for _, mode := range []string{"default", "explicit", "off"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			runPeerRecordingRetention(t, mode)
		})
	}
}

func runPeerRecordingRetention(t *testing.T, mode string) {
	t.Helper()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	runner := support.NewRecordingCommandRunner("attributable peer COMPLETE")
	var service recordings.Service
	// The shared host fixture disables recording unless an explicit path is
	// supplied. Restore the normal CLI default for this recording-owned cell.
	args := []string{"--no-record=false"}
	if mode == "explicit" {
		args = []string{"--record", filepath.Join(t.TempDir(), "host.json")}
	} else if mode == "off" {
		args = []string{"--no-record"}
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: args, Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			ProviderCommandRunner:              runner,
			RecordingsRootObserver:             func(root recordings.Service) { service = root },
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	invokePeerRecordingWork(t, server.URL(), "~default")
	first := support.OpenFactorySessionAt(t, server.URL(), dir).Session.Id
	second := support.OpenFactorySessionAt(t, server.URL(), dir).Session.Id
	if first == second {
		t.Fatal("independent peer identities collided")
	}
	invokePeerRecordingWork(t, server.URL(), first)
	support.CloseFactorySessionAt(t, server.URL(), first)
	invokePeerRecordingWork(t, server.URL(), second)
	assertPeerRecordingPrematureDelete(t, server.URL(), second)
	retained := listPeerRecordingHistory(t, server.URL())
	if mode != "off" {
		if _, found := retained[first]; !found {
			t.Fatalf("closed peer absent from private host history")
		}
	}
	// The second completed peer remains running. Host stop must join and flush it.
	server.Stop(t)
	server.Close(t)
	replayProcess := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }})
	for _, id := range []string{first, second} {
		paths, err := filepath.Glob(filepath.Join(home, ".you-agent-factory", "recordings", "*", "*", "*", id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if mode == "off" {
			if len(paths) != 0 {
				t.Fatalf("unrecorded peer has artifacts")
			}
			continue
		}
		if len(paths) != 1 {
			t.Fatalf("peer %s artifacts=%d; want one", id, len(paths))
		}
		path := paths[0]
		if !strings.HasPrefix(path, filepath.Join(home, ".you-agent-factory", "recordings")+string(os.PathSeparator)) || filepath.Base(path) != id+".json" {
			t.Fatalf("peer destination %q does not belong to selected profile and UUID %s", path, id)
		}
		history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(id), Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: id},
		}})
		if err != nil || len(history.Dispatches) != 1 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
			t.Fatalf("peer %s retained completed dispatch=(%+v, %v)", id, history, err)
		}
		assertPeerRecordingReplay(t, replayProcess, runner, home, dir, id, path)
	}
}

func listPeerRecordingHistory(t *testing.T, baseURL string) map[string]string {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions?scope=history")
	retained := map[string]string{}
	if listed.RecordedSessions != nil {
		for _, session := range *listed.RecordedSessions {
			retained[session.SessionId] = session.ArtifactReference
		}
	}
	return retained
}

func assertPeerRecordingPrematureDelete(t *testing.T, baseURL, id string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusConflict || !bytes.Contains(data, []byte("RUNNING")) {
		t.Fatalf("premature delete status=%d body=%s error=%v", response.StatusCode, data, err)
	}
}

func assertPeerRecordingReplay(t *testing.T, process support.Process, runner *support.RecordingCommandRunner, home, dir, id, path string) {
	t.Helper()
	before := len(runner.Requests())
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--replay", path, "--session", id, "--no-record"})
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("retained peer replay: %v; stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if len(runner.Requests()) != before {
		t.Fatal("retained replay called a provider")
	}
}

func invokePeerRecordingWork(t *testing.T, baseURL, id string) {
	t.Helper()
	support.SubmitSessionWorkAt(t, baseURL, id, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: map[string]string{"title": "complete this peer work"},
	})
	support.WaitForSessionTerminalStatus(t, baseURL, id, 15*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+id+"/work")
	if support.CountWorkAtCustomerState(listed, "task:complete") != 1 {
		t.Fatalf("peer completed Work=%+v", listed)
	}
}
