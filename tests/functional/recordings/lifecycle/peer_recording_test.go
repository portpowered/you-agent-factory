package lifecycle_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	cancelAndDeletePeerRecording(t, server.URL(), first)
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

func cancelAndDeletePeerRecording(t *testing.T, baseURL, id string) {
	t.Helper()
	cancelPeerRecording(t, baseURL, id)
	deletePeerRecording(t, baseURL, id)
}

func cancelPeerRecording(t *testing.T, baseURL, id string) {
	t.Helper()
	response, err := http.Post(baseURL+"/factory-sessions/"+id+"/cancel", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s error=%v", response.StatusCode, data, readErr)
	}
	support.WaitForSessionStopped(t, baseURL, id, 15*time.Second)

}

func deletePeerRecording(t *testing.T, baseURL, id string) {
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
	if response.StatusCode != http.StatusNoContent {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("delete canceled peer status=%d body=%s", response.StatusCode, data)
	}
}

type peerRecordingRendezvous struct {
	arrivals chan string
	canceled chan error
	releases map[string]chan struct{}
	once     sync.Once
}

func (runner *peerRecordingRendezvous) unblock() {
	runner.once.Do(func() {
		for _, release := range runner.releases {
			close(release)
		}
	})
}

func (runner *peerRecordingRendezvous) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if release, found := runner.releases[request.WorkDir]; found {
		runner.arrivals <- request.WorkDir
		select {
		case <-ctx.Done():
			runner.canceled <- ctx.Err()
			return platformprocess.CommandResult{}, ctx.Err()
		case <-release:
		}
	}
	return support.NewStaticSuccessCommandRunner("isolated peer COMPLETE").Run(ctx, request)
}

// Four provider arrivals prove overlap on one host. Only the selected provider
// is canceled; the three held peers keep their own Work and recording streams.
func TestPeerRecordingCancellationKeepsThreePeersUsable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	hostDir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	runner := &peerRecordingRendezvous{arrivals: make(chan string, 16), canceled: make(chan error, 4), releases: make(map[string]chan struct{})}
	dirs := make([]string, 4)
	for i := range dirs {
		dirs[i] = support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
		runner.releases[dirs[i]] = make(chan struct{})
	}
	var service recordings.Service
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostDir, Args: []string{"--no-record=false"}, WaitForServiceModeRuntime: true,
		Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			RecordingsRootObserver: func(root recordings.Service) { service = root },
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.unblock)
	ids := make([]string, 4)
	for i, dir := range dirs {
		ids[i] = support.OpenFactorySessionAt(t, server.URL(), dir).Session.Id
		support.SubmitSessionWorkAt(t, server.URL(), ids[i], factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": fmt.Sprintf("peer-%d", i)}})
	}
	arrived := make(map[string]bool)
	for range dirs {
		select {
		case dir := <-runner.arrivals:
			arrived[dir] = true
		case <-time.After(15 * time.Second):
			t.Fatal("four independent providers did not rendezvous")
		}
	}
	if len(arrived) != 4 {
		t.Fatalf("provider arrivals=%v", arrived)
	}
	cancelPeerRecording(t, server.URL(), ids[0])
	retained := support.GetJSON[factoryapi.ListWorkResponse](t, server.URL()+"/factory-sessions/"+ids[0]+"/work")
	if support.CountWorkAtCustomerState(retained, "task:complete") != 0 {
		t.Fatal("canceled held Work became complete")
	}
	assertPeerRecordingCursor(t, server.URL(), ids[0])
	deletePeerRecording(t, server.URL(), ids[0])
	select {
	case err := <-runner.canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("selected provider cancellation=%v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("selected cancellation did not join its provider")
	}
	runner.unblock()
	for _, id := range ids[1:] {
		support.WaitForSessionTerminalStatus(t, server.URL(), id, 15*time.Second)
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, server.URL()+"/factory-sessions/"+id+"/work")
		if support.CountWorkAtCustomerState(listed, "task:complete") != 1 {
			t.Fatalf("peer %s failed to complete: %+v", id, listed)
		}
		assertPeerRecordingCursor(t, server.URL(), id)
		invokePeerRecordingWork(t, server.URL(), id)
	}
	invokePeerRecordingWork(t, server.URL(), "~default")
	server.Stop(t)
	server.Close(t)
	assertPeerRecordingIsolatedArtifacts(t, service, home, ids)
}

func assertPeerRecordingIsolatedArtifacts(t *testing.T, service recordings.Service, home string, ids []string) {
	t.Helper()
	for i, id := range ids {
		paths, err := filepath.Glob(filepath.Join(home, ".you-agent-factory", "recordings", "*", "*", "*", id+".json"))
		if err != nil || len(paths) != 1 {
			t.Fatalf("peer %s distinct artifact: paths=%v error=%v", id, paths, err)
		}
		history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(id), Artifact: recordings.RecordingArtifactReference(paths[0]), Scope: recordings.CanonicalEventScope{FactorySessionID: id},
		}})
		if err != nil || len(history.Dispatches) == 0 {
			t.Fatalf("peer %s retained dispatches=%+v error=%v", id, history.Dispatches, err)
		}
		for _, dispatch := range history.Dispatches {
			if i == 0 && dispatch.Status == recordings.FactoryDispatchStatusCompleted {
				t.Fatal("held canceled dispatch became complete in retained history")
			}
			if i != 0 && dispatch.Status != recordings.FactoryDispatchStatusCompleted {
				t.Fatalf("peer %s lost completed dispatch: %+v", id, dispatch)
			}
		}
	}
}

func assertPeerRecordingCursor(t *testing.T, baseURL, id string) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, baseURL, id)
	if len(events) < 2 {
		t.Fatalf("peer %s has no completed event history", id)
	}
	for i, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != id {
			t.Fatalf("foreign session event in %s: %+v", id, event.Context)
		}
		if i > 0 && event.Context.Sequence <= events[i-1].Context.Sequence {
			t.Fatal("canonical event order regressed")
		}
	}
	// Startup structure frames have no session sequence. Reconnect within the
	// scoped stream after SESSION_STARTED, whose identity is retained too.
	firstIndex := 0
	for firstIndex < len(events) && events[firstIndex].Context.SessionSequence == nil {
		firstIndex++
	}
	if firstIndex == len(events) {
		t.Fatal("no session-scoped event cursor")
	}
	first := events[firstIndex]
	sequence := support.ReconnectSequenceForFactoryEvent(first)
	suffix := support.GetFactoryEventsAfterForSessionAt(t, baseURL, id, support.FactoryEventReadCursor{AfterEventID: first.Id, AfterSequence: &sequence})
	// Live peers can append lifecycle observations between these two public
	// snapshots. The acknowledged head's complete suffix must remain a prefix.
	expected := events[firstIndex+1:]
	if len(suffix) < len(expected) {
		t.Fatalf("cursor suffix=%d, missing retained suffix of %d", len(suffix), len(expected))
	}
	for i, event := range expected {
		if event.Id != suffix[i].Id {
			t.Fatal("cursor changed retained event identity")
		}
	}
	for _, event := range suffix {
		if event.Context.SessionId != nil && *event.Context.SessionId != id {
			t.Fatal("cursor returned a foreign session event")
		}
	}
}

func TestPeerRecordingOccupiedTargetRetry(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	const id = "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"
	at := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
	occupied := filepath.Join(home, ".you-agent-factory", "recordings", "2041", "02", "03", id+".json")
	if err := os.MkdirAll(filepath.Dir(occupied), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("unrelated retained artifact")
	if err := os.WriteFile(occupied, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	var sessions support.FactorySessionStarter
	runner := support.NewRecordingCommandRunner("retry COMPLETE")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: []string{"--no-record=false"}, WaitForServiceModeRuntime: true,
		Env:   append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{Clock: cancellationRecordingClock{at: at}, ProviderCommandRunner: runner, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }},
		BeforeStart: func(_ testing.TB, process support.Process, _ root.Input) {
			sessions = process.(support.ApplicationProcess).FactorySessions()
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	request := factorysessions.SessionStartRequest{
		SessionID: id, Mode: factorysessions.SessionOperationModeLive, ActivationOnly: true, FolderPath: dir,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Mode: factorysessions.SessionRuntimeModeService},
	}
	if result, err := sessions.Start(t.Context(), request); !errors.Is(err, runtimeartifact.ErrNamedReservationExists) || result.SessionID != "" {
		t.Fatalf("occupied opening result=%+v error=%v", result, err)
	}
	data, err := os.ReadFile(occupied)
	if err != nil || !bytes.Equal(data, foreign) || len(runner.Requests()) != 0 {
		t.Fatalf("failed reservation changed artifact or dispatched Work: data=%q error=%v", data, err)
	}
	response, err := http.Get(server.URL() + "/factory-sessions/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("failed opening published session: %d", response.StatusCode)
	}
	// Correct only this fixture-owned collision. Product must never remove it.
	if err := os.Remove(occupied); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Start(t.Context(), request); err != nil {
		t.Fatalf("corrected same-identity retry: %v", err)
	}
	invokePeerRecordingWork(t, server.URL(), request.SessionID)
	cancelAndDeletePeerRecording(t, server.URL(), request.SessionID)
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
