package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The serving default and both direct CLI modes share one process. Explicit
// session identities and destinations keep invocation output and ownership local.
func TestPeerRecordingDirectCLIModesUnderRecordedHost(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	runner := support.NewRecordingCommandRunner("direct selected COMPLETE")
	var service recordings.Service
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: []string{"--no-record=false"}, WaitForServiceModeRuntime: true,
		Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			RecordingsRootObserver: func(root recordings.Service) { service = root }},
	})
	t.Cleanup(func() { server.Stop(t) })
	pattern := filepath.Join(t.TempDir(), "__factory_session_id__.json")
	foreignPath := strings.ReplaceAll(pattern, "__factory_session_id__", "unrelated")
	foreign := []byte("unrelated recording bytes")
	if err := os.WriteFile(foreignPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	for i, mode := range []string{"off", "explicit"} {
		id := []string{"019a07c0-0000-7000-8000-000000000211", "019a07c0-0000-7000-8000-000000000212"}[i]
		inputs := cancellationRecordingInputs(t, pattern)
		if mode == "off" {
			inputs.Args = append(inputs.Args[:len(inputs.Args)-2], "--no-record")
		}
		inputs.Args = append(inputs.Args, "--session", id)
		if err := server.Execute(t, inputs.Input); err != nil {
			t.Fatalf("direct %s: %v; stderr=%s", mode, err, inputs.Stderr())
		}
		if inputs.Stdout() != "Batch completed successfully.\n" {
			t.Fatalf("direct %s output=%q", mode, inputs.Stdout())
		}
		path := strings.ReplaceAll(pattern, "__factory_session_id__", id)
		if mode == "off" {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("disabled direct recording created destination: %v", err)
			}
		} else {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("explicit direct destination: %v", err)
			}
			assertOwnershipHistory(t, service, id, path)
		}
		generated, err := filepath.Glob(filepath.Join(home, ".you-agent-factory", "recordings", "*", "*", "*", id+".json"))
		if err != nil || len(generated) != 0 {
			t.Fatalf("direct %s inherited automatic target: %v, %v", mode, generated, err)
		}
	}
	if data, err := os.ReadFile(foreignPath); err != nil || !bytes.Equal(data, foreign) {
		t.Fatalf("direct mode changed unrelated bytes: %q, %v", data, err)
	}
	invokePeerRecordingWork(t, server.URL(), "~default")
}

// Fault injection is confined to the filesystem claim edge. The public Start
// contract must release the earlier UUID reservation and permit corrected retry.
func TestPeerRecordingFailedClaimRetry(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	const id = "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"
	cause := errors.New("PRIVATE_PEER_CLAIM_FAILURE")
	var failed atomic.Bool
	failed.Store(true)
	claim, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatal(err)
	}
	var sessions support.FactorySessionStarter
	runner := support.NewRecordingCommandRunner("corrected claim COMPLETE")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: []string{"--no-record=false"}, WaitForServiceModeRuntime: true,
		Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			RecordingTargetClaim: func(ctx context.Context, path, marker string) (io.Closer, error) {
				if filepath.Base(path) == id+".json" && failed.Load() {
					return nil, cause
				}
				return claim.TryLockTarget(ctx, path, marker)
			},
		},
		BeforeStart: func(_ testing.TB, process support.Process, _ root.Input) {
			sessions = process.(support.ApplicationProcess).FactorySessions()
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	request := factorysessions.SessionStartRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive, ActivationOnly: true, FolderPath: dir,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Mode: factorysessions.SessionRuntimeModeService}}
	if result, err := sessions.Start(t.Context(), request); !errors.Is(err, cause) || !errors.Is(err, recordings.ErrRecordingBindingConflict) || result.SessionID != "" {
		t.Fatalf("failed claim result=%+v error=%v", result, err)
	}
	paths, err := filepath.Glob(filepath.Join(home, ".you-agent-factory", "recordings", "*", "*", "*", id+".json"))
	if err != nil || len(paths) != 0 || len(runner.Requests()) != 0 {
		t.Fatalf("failed claim leaked artifact or dispatched: paths=%v error=%v", paths, err)
	}
	response, err := http.Get(server.URL() + "/factory-sessions/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("failed claim published live session: %d", response.StatusCode)
	}
	failed.Store(false)
	if _, err := sessions.Start(t.Context(), request); err != nil {
		t.Fatalf("corrected same-UUID claim retry: %v", err)
	}
	invokePeerRecordingWork(t, server.URL(), id)
	cancelAndDeletePeerRecording(t, server.URL(), id)
	invokePeerRecordingWork(t, server.URL(), "~default")
	// Closed peer remains readable through the API-owned durable history surface.
	if _, found := listPeerRecordingHistory(t, server.URL())[id]; !found {
		t.Fatal("corrected claim retry lost peer history")
	}
}

func TestPeerRecordingFinalWriteFailureKeepsPeerUsable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	var selected atomic.Value
	selected.Store("")
	cause := errors.New("PRIVATE_PEER_STORAGE_FAILURE")
	baseURL, service, recordingID := startPeerRecordingFaultHost(t, home, dir, &selected, cause)
	invokePeerRecordingWork(t, baseURL, "~default")
	first := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	second := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	invokePeerRecordingWork(t, baseURL, first)
	prefix, err := service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: recordingID})
	if err != nil || prefix.Status.FlushedThrough == nil {
		t.Fatalf("selected durable prefix: %+v, %v", prefix, err)
	}
	selected.Store(first)
	t.Cleanup(func() { selected.Store("") })
	response, err := http.Post(baseURL+"/factory-sessions/"+first+"/cancel", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || bytes.Contains(data, []byte(cause.Error())) {
		t.Fatalf("unsafe cancellation response: status=%d body=%s error=%v", response.StatusCode, data, readErr)
	}
	support.WaitForSessionStopped(t, baseURL, first, 15*time.Second)
	assertFailedPeerRecordingClose(t, baseURL, first, cause)
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: recordingID})
	if err != nil || status.Status.State != recordings.RecordingFailed || len(status.Status.Failures) == 0 || !canceledRecordingRetainsDurablePrefix(status.Status, prefix.Status) {
		t.Fatalf("failed peer claimed durability: %+v, %v", status, err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: recordingID}); !errors.Is(err, cause) {
		t.Fatalf("selected flush lost storage cause: %v", err)
	}
	invokePeerRecordingWork(t, baseURL, second)
	cancelAndDeletePeerRecording(t, baseURL, second)
	invokePeerRecordingWork(t, baseURL, "~default")
	selected.Store("") // Permit ownership cleanup; the failed status remains factual.
}

func startPeerRecordingFaultHost(t *testing.T, home, dir string, selected *atomic.Value, cause error) (string, recordings.Service, recordings.RecordingID) {
	t.Helper()
	var service recordings.Service
	var nextRuntime atomic.Uint32
	runtimeIDs := []string{"019a07c0-0000-7000-8000-000000000221", "019a07c0-0000-7000-8000-000000000222", "019a07c0-0000-7000-8000-000000000223"}
	api := support.NewProcessAPIServer()
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter:                         api.Start,
		FactorySessionRuntimeInstanceIDGenerator: func() string { return runtimeIDs[nextRuntime.Add(1)-1] },
		ProviderCommandRunner:                    support.NewStaticSuccessCommandRunner("unaffected peer COMPLETE"),
		FactorySessionResolveHomeDirectory:       func() (string, error) { return home, nil },
		RecordingsRootObserver:                   func(root recordings.Service) { service = root },
		RecordingWriteFile: func(path string, data []byte) error {
			if filepath.Base(path) == selected.Load().(string)+".json" {
				return cause
			}
			return os.WriteFile(path, data, 0o600)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet"})
	inputs.WorkingDirectory = dir
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	// This fault cohort owns cleanup so the exact expected storage cause is
	// checked instead of being treated as an unexpected generic fixture error.
	t.Cleanup(func() {
		selected.Store("")
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, cause) {
				t.Errorf("joined host: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("fault host did not join")
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		if err := process.Close(closeCtx); !errors.Is(err, cause) {
			t.Errorf("joined shutdown lost selected storage cause: %v", err)
		}
	})
	baseURL := api.WaitForURL(t)
	return baseURL, service, recordings.RecordingID(runtimeIDs[1])
}

func TestPeerRecordingPreservesForeignBoardReference(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	foreignPath := filepath.Join(t.TempDir(), "foreign.json")
	foreign := []byte("unrelated canonical artifact")
	if err := os.WriteFile(foreignPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	referencePath := filepath.Join(dir, ".you-agent-factory", "current-board.json")
	reference, err := json.Marshal(map[string]string{
		"schemaVersion": "factory-sessions.current-board.v1", "factoryDirectory": t.TempDir(),
		"factorySessionId": "~default", "artifactReference": foreignPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(referencePath, reference, 0o600); err != nil {
		t.Fatal(err)
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: []string{"--record", filepath.Join(t.TempDir(), "explicit-host.json")}, WaitForServiceModeRuntime: true,
		Env:   append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{ProviderCommandRunner: support.NewStaticSuccessCommandRunner("owned peer COMPLETE"), FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }},
	})
	t.Cleanup(func() { server.Stop(t) })
	id := support.OpenFactorySessionAt(t, server.URL(), dir).Session.Id
	invokePeerRecordingWork(t, server.URL(), id)
	cancelAndDeletePeerRecording(t, server.URL(), id)
	invokePeerRecordingWork(t, server.URL(), "~default")
	server.Stop(t)
	server.Close(t)
	for path, want := range map[string][]byte{referencePath: reference, foreignPath: foreign} {
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("peer replaced protected destination %s: %q, %v", path, got, err)
		}
	}
}

func assertFailedPeerRecordingClose(t *testing.T, baseURL, first string, cause error) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+first, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode < 400 || bytes.Contains(data, []byte(cause.Error())) {
		t.Fatalf("unsafe or successful failed close: status=%d body=%s error=%v", response.StatusCode, data, readErr)
	}

}
