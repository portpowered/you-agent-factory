package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/portpowered/infinite-you/pkg/root"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type cancellationRecordingClock struct{ at time.Time }

func (clock cancellationRecordingClock) Now() time.Time { return clock.at }

type cancellationRecordingRunner struct {
	started  chan struct{}
	canceled chan error
	once     sync.Once
}

func (runner *cancellationRecordingRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	<-ctx.Done()
	runner.canceled <- ctx.Err()
	return platformprocess.CommandResult{}, ctx.Err()
}

// Local Execute uses ~default; each parallel cell owns its process and profile.
// Explicit-scope peer behavior is observed separately, never inferred from this ID.
func TestExecuteCancellationPreservesRecordingTerminalOutcome(t *testing.T) {
	for _, failFinal := range []bool{false, true} {
		t.Run(fmt.Sprintf("final-write-fails=%t", failFinal), func(t *testing.T) {
			t.Parallel()
			runExecuteCancellationRecording(t, failFinal)
		})
	}
}

func runExecuteCancellationRecording(t *testing.T, failFinal bool) {
	t.Helper()
	at := time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)
	path := filepath.Join(t.TempDir(), "canceled.json")
	writeErr := errors.New("canceled recording final storage unavailable")
	id := recordings.RecordingID("019a07c0-0000-7000-8000-000000000031")
	runner := &cancellationRecordingRunner{started: make(chan struct{}), canceled: make(chan error, 1)}
	writeStarted, writeCompleted, releaseWrite := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var startOnce, completeOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	defer release()
	var service recordings.Service
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		Clock: cancellationRecordingClock{at: at}, ProviderCommandRunner: runner,
		FactorySessionRuntimeInstanceIDGenerator: func() string { return string(id) },
		RecordingsRootObserver:                   func(root recordings.Service) { service = root },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
		RecordingReadFile: os.ReadFile,
		RecordingWriteFile: func(destination string, data []byte) error {
			terminal := destination == path && recordingWriteHasTerminalEvent(data)
			if terminal {
				startOnce.Do(func() { close(writeStarted) })
				<-releaseWrite
				defer completeOnce.Do(func() { close(writeCompleted) })
				if failFinal {
					return writeErr
				}
			}
			return os.WriteFile(destination, data, 0o600)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := process.Close(ctx); err != nil && (!failFinal || !errors.Is(err, writeErr)) {
			t.Errorf("close canceled process: %v", err)
		}
	})
	inputs := cancellationRecordingInputs(t, path)
	ctx, cancel := context.WithCancel(inputs.Context)
	inputs.Context = ctx
	done := executeGatedRecordingCommand(t, process, inputs, func() { release(); cancel() })
	select {
	case <-runner.started:
	case err := <-done:
		t.Fatalf("Execute ended before provider admission: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("provider was never admitted")
	}
	prefix, err := service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: id})
	if err != nil || prefix.Status.FlushedThrough == nil {
		t.Fatalf("flush admitted prefix = (%#v, %v)", prefix, err)
	}
	peer := bindPublicRecording(t, service, "cancel-flush-peer", "019a07c0-0000-7000-8000-000000000032",
		filepath.Join(t.TempDir(), "peer.json"))
	peerFirst := recordPublicRecordingFact(t, service, peer, "peer-before-cancel", 0, at.Add(-time.Second))
	cancel()
	assertCanceledExecuteWaitsForTerminalWrite(t, writeStarted, done, release)
	select {
	case err := <-done:
		assertCanceledExecuteDiagnostic(t, err, writeErr)
	case <-time.After(30 * time.Second):
		t.Fatal("canceled Execute did not join")
	}
	select {
	case <-writeCompleted:
	default:
		t.Fatal("canceled Execute returned before terminal write completed")
	}
	select {
	case err := <-runner.canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("provider cancellation cause = %v", err)
		}
	default:
		t.Fatal("Execute returned before canceling provider attempt")
	}
	assertCanceledRecordingStatus(t, service, id, at, prefix.Status, failFinal, writeErr)
	assertCanceledRecordingHistory(t, service, id, path, failFinal)
	assertPeerFinalizesAfterCanceledWrite(t, service, peer, peerFirst, at)
}

func assertPeerFinalizesAfterCanceledWrite(t *testing.T, service recordings.Service, peer recordings.RecordingStatusFacts, first recordings.CanonicalEvent, at time.Time) {
	t.Helper()
	second := recordPublicRecordingFact(t, service, peer, "peer-after-canceled-flush", 1, at)
	finished, err := service.FinishRecording(recordings.FinishRecordingRequest{RecordingID: peer.RecordingID, FinishedAt: at})
	if err != nil || finished.Status.State != recordings.RecordingFinalized ||
		finished.Status.FlushedThrough == nil || *finished.Status.FlushedThrough != second.Cursor {
		t.Fatalf("peer finalization after canceled write = (%#v, %v)", finished, err)
	}
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: peer.RecordingID, Artifact: peer.Artifact, Scope: peer.Scope},
	})
	if err != nil || len(history.Events) != 2 {
		t.Fatalf("peer history after canceled write = (%#v, %v)", history, err)
	}
	assertPublicHistoricalFact(t, history.Events[0], first)
	assertPublicHistoricalFact(t, history.Events[1], second)
}

func assertCanceledExecuteWaitsForTerminalWrite(t *testing.T, started <-chan struct{}, done <-chan error, release func()) {
	t.Helper()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("canceled Execute returned before terminal write: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("canceled terminal writer did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("canceled Execute returned with terminal write blocked: %v", err)
	default:
	}
	release()
}

func assertCanceledExecuteDiagnostic(t *testing.T, err, writeErr error) {
	t.Helper()
	var diagnostic clidiag.CodedError
	if !errors.As(err, &diagnostic) || diagnostic.CLIErrorCode() != clidiag.DefaultFailureCode || errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Execute diagnostic = %v, want CLI_COMMAND_FAILED without cancellation cause", err)
	}
	// Batch reading can race teardown: both observed diagnostics belong to the
	// delegated cancellation-observer successor, not recording-owner injection.
	if !strings.Contains(err.Error(), "factory session not found") && !strings.Contains(err.Error(), "factory runtime is not running") && !errors.Is(err, writeErr) {
		t.Fatalf("uncharacterized cancellation diagnostic: %v", err)
	}
	t.Logf("Execute diagnostic (writer cause=%t): %v", errors.Is(err, writeErr), err)
}

func cancellationRecordingInputs(t *testing.T, path string) *support.CapturedInputs {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "cancel-recording-history")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	workPath := filepath.Join(t.TempDir(), "work.json")
	request, err := json.Marshal(work.WorkRequest{Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{Name: "canceled-work", WorkTypeID: "task", Payload: map[string]string{"title": "cancel recording"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workPath, request, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir,
		"--work", workPath, "--quiet", "--record", path})
	home := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	return inputs
}

func assertCanceledRecordingStatus(t *testing.T, service recordings.Service, id recordings.RecordingID, at time.Time, prefix recordings.RecordingStatusFacts, failFinal bool, writeErr error) {
	t.Helper()
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
	if err != nil || status.Status.FinalizedAt == nil || *status.Status.FinalizedAt != at {
		t.Fatalf("canceled terminal metadata = (%#v, %v), want exact UTC %v", status, err, at)
	}
	finished, terminalErr := service.FinishRecording(recordings.FinishRecordingRequest{RecordingID: id, FinishedAt: at.Add(time.Hour)})
	if errors.Is(terminalErr, context.Canceled) {
		t.Fatalf("repeated Finish unexpectedly retained caller cancellation: %v", terminalErr)
	}
	// The current runtime does not forward caller cancellation as a recording
	// producer error. Keep this characterization separate from the unproven
	// joined-cancellation acceptance criterion; never manufacture that cause.
	if failFinal {
		if finished.Status.State != recordings.RecordingFailed || !errors.Is(terminalErr, writeErr) || !canceledRecordingRetainsDurablePrefix(status.Status, prefix) {
			t.Fatalf("failed final flush = (%#v, %v), want retained prefix and storage cause", status, terminalErr)
		}
	} else if terminalErr != nil || finished.Status.State != recordings.RecordingFinalized || status.Status.FlushedThrough == nil || status.Status.LastEvent == nil || *status.Status.FlushedThrough != *status.Status.LastEvent {
		t.Fatalf("successful cancellation flush lacks terminal durability: %#v", status)
	}
	if finished.Status.FinalizedAt == nil || *finished.Status.FinalizedAt != at {
		t.Fatalf("repeated Finish changed terminal UTC: %#v", finished)
	}
}

func canceledRecordingRetainsDurablePrefix(status, prefix recordings.RecordingStatusFacts) bool {
	// A successful background flush may advance beyond the explicit pre-cancel
	// flush. The failed terminal write must retain that successful prefix, while
	// never acknowledging the terminal event that it could not publish.
	return status.FlushedThrough != nil && prefix.FlushedThrough != nil && status.LastEvent != nil &&
		status.FlushedThrough.StreamGenerationID == prefix.FlushedThrough.StreamGenerationID &&
		status.FlushedThrough.Sequence >= prefix.FlushedThrough.Sequence &&
		status.FlushedThrough.Sequence < status.LastEvent.Sequence
}

func assertCanceledRecordingHistory(t *testing.T, service recordings.Service, id recordings.RecordingID, path string, failFinal bool) {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: id,
			Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}},
	})
	if err != nil {
		t.Fatalf("public canceled history: %v", err)
	}
	seen := make(map[recordings.CanonicalEventKind]bool)
	for index, event := range history.Events {
		if index > 0 && event.Sequence <= history.Events[index-1].Sequence {
			t.Fatal("canceled historical facts lost admitted order")
		}
		seen[event.Kind] = true
	}
	for _, kind := range []recordings.CanonicalEventKind{"RUN_REQUEST", "WORK_REQUEST", "DISPATCH_REQUEST"} {
		if !seen[kind] {
			t.Fatalf("durable canceled history missing %s", kind)
		}
	}
	if seen["RUN_RESPONSE"] == failFinal {
		t.Fatalf("durable terminal fact present=%t, final flush failed=%t", seen["RUN_RESPONSE"], failFinal)
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
	if err != nil || status.Status.FlushedThrough == nil || len(history.Events) == 0 {
		t.Fatalf("durable canceled history has no watermark: (%#v, %v)", status, err)
	}
	// Historical reads reconstruct their own generation; the admitted sequence
	// must still exactly match the last successfully published runtime position.
	if last := history.Events[len(history.Events)-1]; last.Sequence != status.Status.FlushedThrough.Sequence {
		t.Fatalf("durable history ends at %d, acknowledged watermark is %d", last.Sequence, status.Status.FlushedThrough.Sequence)
	}
}

// The public scoped stream is the observer here. Recording is disabled so this
// witness makes no artifact or durable-flush claim.
func TestRuntimeScopeCleanupPreservesPeerReconnect(t *testing.T) {
	t.Parallel()
	var service recordings.Service
	support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) { service = root },
	})
	opening, ok := service.(recordings.RuntimeScopeService)
	if !ok {
		t.Fatal("Recordings root does not expose runtime opening")
	}
	at := time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)
	firstID := "019a07c0-0000-7000-8000-000000000011"
	peerID := "019a07c0-0000-7000-8000-000000000012"
	first := openPublicRuntimeScope(t, opening, firstID, at)
	peer := openPublicRuntimeScope(t, opening, peerID, at)
	firstEvent := appendPublicRuntimeFact(t, service, firstID, "first-runtime-fact", at)
	peerEvent := appendPublicRuntimeFact(t, service, peerID, "peer-runtime-first", at)
	firstEvent = assertPublicRuntimeDelivery(t, service, firstID, nil, firstEvent)
	peerEvent = assertPublicRuntimeDelivery(t, service, peerID, nil, peerEvent)
	if _, err := service.SubscribeFrom(t.Context(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: peerID}, Cursor: &firstEvent.Cursor,
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("foreign runtime cursor = %v, want ErrReconnectCursorUnavailable", err)
	}
	if err := first.Recorder.Finalize(at); err != nil {
		t.Fatalf("Finalize first runtime: %v", err)
	}
	if _, err := service.SubscribeFrom(t.Context(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: firstID},
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("closed runtime reconnect = %v, want ErrReconnectCursorUnavailable", err)
	}
	second := appendPublicRuntimeFact(t, service, peerID, "peer-runtime-second", at.Add(time.Second))
	second = assertPublicRuntimeDelivery(t, service, peerID, &peerEvent.Cursor, second)
	if second.Cursor.StreamGenerationID != peerEvent.Cursor.StreamGenerationID || second.Sequence != peerEvent.Sequence+1 {
		t.Fatalf("peer continuation lost ordered cursor: first=%#v second=%#v", peerEvent, second)
	}
	if err := peer.Recorder.Finalize(at.Add(time.Second)); err != nil {
		t.Fatalf("Finalize peer runtime: %v", err)
	}
}

type publicRuntimeTopology struct{}

func (publicRuntimeTopology) RecordingInitialStructure(...factorydefinitions.RuntimeDefinitionLookup) recordings.InitialStructurePayload {
	return recordings.InitialStructurePayload{}
}

func openPublicRuntimeScope(t *testing.T, opening recordings.RuntimeScopeService, sessionID string, at time.Time) recordings.RuntimeScopeResult {
	t.Helper()
	opened, err := opening.OpenRuntime(t.Context(), recordings.RuntimeScopeRequest{
		Topology: publicRuntimeTopology{}, Now: func() time.Time { return at },
		RecordingID: sessionID, FactorySessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("OpenRuntime(%s): %v", sessionID, err)
	}
	t.Cleanup(func() {
		if err := opened.Recorder.Finalize(at); err != nil {
			t.Errorf("runtime scope cleanup: %v", err)
		}
	})
	return opened
}

func appendPublicRuntimeFact(t *testing.T, service recordings.Service, sessionID, id string, at time.Time) recordings.CanonicalEvent {
	t.Helper()
	result, err := service.Append(recordings.AppendRecordedEventRequest{Event: recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
		RecordedAt: at, Kind: "WORK_REQUEST", Payload: `{"type":"FACTORY_REQUEST_BATCH","works":[]}`,
	}})
	if err != nil {
		t.Fatalf("Append(%s): %v", sessionID, err)
	}
	return result.Event
}

func assertPublicRuntimeDelivery(t *testing.T, service recordings.Service, sessionID string, cursor *recordings.CanonicalEventCursor, want recordings.CanonicalEvent) recordings.CanonicalEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("SubscribeFrom(%s): %v", sessionID, err)
	}
	if result.RetainedEventCount != 1 {
		t.Fatalf("selected retained events = %d, want one", result.RetainedEventCount)
	}
	outcome := result.Subscription.Next(ctx)
	if outcome.Kind != recordings.SubscriptionEvent || (cursor != nil && outcome.Event.Cursor.StreamGenerationID != cursor.StreamGenerationID) {
		t.Fatalf("selected runtime delivery = %#v, want %#v", outcome, want)
	}
	assertPublicHistoricalFact(t, outcome.Event, want)
	return outcome.Event
}

// The local run command owns ~default. Its isolated profile is intentional;
// explicit concurrent runtime scopes are characterized separately.
func TestExecuteFlushesTerminalRecordingBeforeReturning(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "flush-terminal-history")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	workPath := filepath.Join(t.TempDir(), "work.json")
	request, err := json.Marshal(work.WorkRequest{Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{Name: "terminal-work", WorkTypeID: "task", Payload: map[string]string{"title": "flush terminal history"}}}})
	if err != nil {
		t.Fatalf("encode Work Request: %v", err)
	}
	if err := os.WriteFile(workPath, request, 0o600); err != nil {
		t.Fatalf("write Work Request: %v", err)
	}
	path := filepath.Join(t.TempDir(), "terminal.json")
	started, completed, releaseWrite := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce, startOnce, completeOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	defer release()
	var service recordings.Service
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:  support.NewStaticSuccessCommandRunner("recording worker COMPLETE"),
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
		RecordingReadFile: os.ReadFile,
		RecordingWriteFile: func(destination string, data []byte) error {
			// Inspect only the external write to signal the terminal snapshot;
			// persisted customer facts are asserted through the public query below.
			terminal := recordingWriteHasTerminalEvent(data)
			if terminal {
				startOnce.Do(func() { close(started) })
				<-releaseWrite
			}
			err := os.WriteFile(destination, data, 0o600)
			if terminal {
				completeOnce.Do(func() { close(completed) })
			}
			return err
		},
	})
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir,
		"--work", workPath, "--quiet", "--record", path})
	home := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	done := executeGatedRecordingCommand(t, process, inputs, release)
	assertExecuteJoinsTerminalWrite(t, started, completed, done, release)
	assertPublicExecuteTerminalHistory(t, service, path)
}

func executeGatedRecordingCommand(t *testing.T, process support.Process, inputs *support.CapturedInputs, release func()) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(inputs.Context)
	inputs.Context = ctx
	done, joined := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(joined)
		done <- process.Execute(inputs.Input)
	}()
	t.Cleanup(func() {
		release()
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("recording command cleanup did not join Execute")
		}
	})
	return done
}

func assertExecuteJoinsTerminalWrite(t *testing.T, started, completed <-chan struct{}, done <-chan error, release func()) {
	t.Helper()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("Execute returned before terminal writer: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("terminal writer did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("Execute returned with terminal writer blocked: %v", err)
	default:
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Execute did not join terminal writer")
	}
	select {
	case <-completed:
	default:
		t.Fatal("Execute completed before terminal write completion")
	}
}

func recordingWriteHasTerminalEvent(data []byte) bool {
	var snapshot struct {
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
	}
	if json.Unmarshal(data, &snapshot) != nil {
		return false
	}
	for _, event := range snapshot.Events {
		if event.Type == "RUN_RESPONSE" {
			return true
		}
	}
	return false
}

func assertPublicExecuteTerminalHistory(t *testing.T, service recordings.Service, path string) {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: "execute-terminal", Artifact: recordings.RecordingArtifactReference(path),
			Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"},
		},
	})
	if err != nil {
		t.Fatalf("public terminal historical read: %v", err)
	}
	seen := make(map[recordings.CanonicalEventKind]bool)
	for index, event := range history.Events {
		if index > 0 && event.Sequence <= history.Events[index-1].Sequence {
			t.Fatalf("historical events out of admitted order: %#v", history.Events)
		}
		seen[event.Kind] = true
	}
	for _, kind := range []recordings.CanonicalEventKind{"RUN_REQUEST", "WORK_REQUEST", "DISPATCH_REQUEST", "DISPATCH_RESPONSE", "RUN_RESPONSE"} {
		if !seen[kind] {
			t.Fatalf("public terminal history missing %s: %#v", kind, history.Events)
		}
	}
	if len(history.Dispatches) != 1 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
		t.Fatalf("public terminal dispatches = %#v, want one completed dispatch", history.Dispatches)
	}
}

var errRecordingRecordingsEffect = errors.New("recording Recordings effect invoked during BuildProcess")

// TestRecordingsEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Recordings without invoking portable-recording
// filesystem ports, canonical submission/dispatch recording, or other
// Recordings-owned external effects before runtime lifecycle starts.
func TestRecordingsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newRecordingsEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalPortableRecordingFilesystem(); got != 0 {
		t.Fatalf(
			"portable-recording filesystem effect calls = %d during BuildProcess, want 0",
			got,
		)
	}
	if got := recorder.totalCanonicalRecording(); got != 0 {
		t.Fatalf(
			"canonical submission/dispatch recording effect calls = %d during BuildProcess, want 0",
			got,
		)
	}
}

type recordingsEffectRecorder struct {
	makeDirectories atomic.Int32
	createTempFile  atomic.Int32
	removePath      atomic.Int32
	renamePath      atomic.Int32
	submission      atomic.Int32
	dispatch        atomic.Int32
}

func newRecordingsEffectRecorder() *recordingsEffectRecorder {
	return &recordingsEffectRecorder{}
}

func (recorder *recordingsEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		RecordingMakeDirectories: recorder.recordMakeDirectories,
		RecordingCreateTempFile:  recorder.recordCreateTempFile,
		RecordingRemovePath:      recorder.recordRemovePath,
		RecordingRenamePath:      recorder.recordRenamePath,
		SubmissionRecorder:       recorder.recordSubmission,
		DispatchRecorder:         recorder.recordDispatch,
	}
}

func (recorder *recordingsEffectRecorder) totalPortableRecordingFilesystem() int32 {
	return recorder.makeDirectories.Load() +
		recorder.createTempFile.Load() +
		recorder.removePath.Load() +
		recorder.renamePath.Load()
}

func (recorder *recordingsEffectRecorder) totalCanonicalRecording() int32 {
	return recorder.submission.Load() + recorder.dispatch.Load()
}

func (recorder *recordingsEffectRecorder) recordMakeDirectories(string, fs.FileMode) error {
	recorder.makeDirectories.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordCreateTempFile(string, string) (recordings.RecordingTemporaryFile, error) {
	recorder.createTempFile.Add(1)
	return recordingRecordingsTempFile{}, errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordRemovePath(string) error {
	recorder.removePath.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordRenamePath(string, string) error {
	recorder.renamePath.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordSubmission(work.FactorySubmissionRecord) {
	recorder.submission.Add(1)
}

func (recorder *recordingsEffectRecorder) recordDispatch(recordings.FactoryDispatchRecord) {
	recorder.dispatch.Add(1)
}

type recordingRecordingsTempFile struct{}

func (recordingRecordingsTempFile) Write([]byte) (int, error) {
	return 0, errRecordingRecordingsEffect
}

func (recordingRecordingsTempFile) Name() string { return "" }

func (recordingRecordingsTempFile) Chmod(fs.FileMode) error {
	return errRecordingRecordingsEffect
}

func (recordingRecordingsTempFile) Sync() error { return errRecordingRecordingsEffect }

func (recordingRecordingsTempFile) Close() error { return errRecordingRecordingsEffect }

var _ recordings.RecordingTemporaryFile = recordingRecordingsTempFile{}

// Explicit lifecycle bindings use the public Recordings authority from the
// production process. Runtime opening and CLI shutdown have separate witnesses.
func TestFailedRecordingFinalFlushPreservesPeerHistory(t *testing.T) {
	t.Parallel()
	failedPath := filepath.Join(t.TempDir(), "failed.json")
	peerPath := filepath.Join(t.TempDir(), "peer.json")
	writeErr := errors.New("selected scope storage unavailable")
	writeStarted, releaseWrite := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	defer release()
	var service recordings.Service
	var providerRuns atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:  functionalReplayCommandRunner{calls: &providerRuns},
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		RecordingWriteFile: func(path string, data []byte) error {
			if path == failedPath {
				close(writeStarted)
				<-releaseWrite
				return writeErr
			}
			return os.WriteFile(path, data, 0o600)
		},
		RecordingReadFile: os.ReadFile,
	})
	if service == nil {
		t.Fatal("BuildProcess did not publish the Recordings authority")
	}
	failed := bindPublicRecording(t, service, "failed", "019a07c0-0000-7000-8000-000000000001", failedPath)
	peer := bindPublicRecording(t, service, "peer", "019a07c0-0000-7000-8000-000000000002", peerPath)
	finishedAt := time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)
	recordPublicRecordingFact(t, service, failed, "failed-fact", 0, finishedAt.Add(-time.Second))
	first := recordPublicRecordingFact(t, service, peer, "peer-first", 0, finishedAt.Add(-time.Second))
	assertGatedPublicRecordingFailure(t, service, failed.RecordingID, finishedAt, writeStarted, release, writeErr)
	second := recordPublicRecordingFact(t, service, peer, "peer-second", 1, finishedAt)
	finalized, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: peer.RecordingID, FinishedAt: finishedAt,
	})
	if err != nil || finalized.Status.State != recordings.RecordingFinalized ||
		finalized.Status.FlushedThrough == nil || *finalized.Status.FlushedThrough != second.Cursor {
		t.Fatalf("peer finalization = (%#v, %v)", finalized, err)
	}
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: peer.RecordingID, Artifact: recordings.RecordingArtifactReference(peerPath), Scope: peer.Scope,
		},
	})
	if err != nil {
		t.Fatalf("public peer historical read: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("peer history = %#v, want only peer facts in admitted order", history.Events)
	}
	assertPublicHistoricalFact(t, history.Events[0], first)
	assertPublicHistoricalFact(t, history.Events[1], second)
	assertRecordingBindingReusePreservesHistory(t, service, peer)
	assertReadOnlyPeerReplay(t, process, peerPath, &providerRuns)
}

func assertRecordingBindingReusePreservesHistory(t *testing.T, service recordings.Service, peer recordings.RecordingStatusFacts) {
	t.Helper()
	before, err := os.ReadFile(string(peer.Artifact))
	if err != nil {
		t.Fatalf("read selected artifact before conflicting reuse: %v", err)
	}
	_, err = service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: peer.RecordingID, Scope: peer.Scope,
		Artifact: recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), "conflicting.json")),
	})
	if !errors.Is(err, recordings.ErrRecordingBindingConflict) {
		t.Fatalf("conflicting recording identity reuse = %v, want ErrRecordingBindingConflict", err)
	}
	after, err := os.ReadFile(string(peer.Artifact))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("conflicting reuse changed the selected artifact: %v", err)
	}
}

func assertReadOnlyPeerReplay(t *testing.T, process support.Process, peerPath string, providerRuns *atomic.Int32) {
	t.Helper()
	beforeReplay, err := os.ReadFile(peerPath)
	if err != nil {
		t.Fatalf("read replay source: %v", err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", t.TempDir(), "--replay", peerPath, "--no-record"})
	home := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(peer replay): %v\n%s", err, inputs.Stderr())
	}
	afterReplay, err := os.ReadFile(peerPath)
	if err != nil || !reflect.DeepEqual(beforeReplay, afterReplay) || providerRuns.Load() != 0 {
		t.Fatalf("read-only peer replay changed source or executed provider: read=%v provider=%d", err, providerRuns.Load())
	}
}

func assertPublicHistoricalFact(t *testing.T, got, want recordings.CanonicalEvent) {
	t.Helper()
	var gotPayload, wantPayload any
	if err := json.Unmarshal([]byte(got.Payload), &gotPayload); err != nil {
		t.Fatalf("historical payload: %v", err)
	}
	if err := json.Unmarshal([]byte(want.Payload), &wantPayload); err != nil {
		t.Fatalf("admitted payload: %v", err)
	}
	if got.ID != want.ID || got.Sequence != want.Sequence || got.Scope != want.Scope ||
		got.RecordedAt != want.RecordedAt || got.Kind != want.Kind || !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("historical fact = %#v, want admitted fact %#v", got, want)
	}
}

func assertGatedPublicRecordingFailure(t *testing.T, service recordings.Service, id recordings.RecordingID, finishedAt time.Time, writeStarted <-chan struct{}, release func(), writeErr error) {
	t.Helper()
	type outcome struct {
		result recordings.FinishRecordingResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := service.FinishRecording(recordings.FinishRecordingRequest{
			RecordingID: id, FinishedAt: finishedAt,
		})
		done <- outcome{result, err}
	}()
	select {
	case <-writeStarted:
	case finished := <-done:
		t.Fatalf("finalization returned before writer start: %v", finished.err)
	case <-time.After(30 * time.Second):
		t.Fatal("final writer did not start")
	}
	select {
	case <-done:
		t.Fatal("scope finalization returned before the writer completed")
	default:
	}
	release()
	var failedOutcome outcome
	select {
	case failedOutcome = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("scope finalization did not join the writer")
	}
	status := failedOutcome.result.Status
	if !errors.Is(failedOutcome.err, writeErr) || status.State != recordings.RecordingFailed ||
		status.FlushedThrough != nil || status.FinalizedAt == nil || *status.FinalizedAt != finishedAt {
		t.Fatalf("failed scope = (%#v, %v), want exact UTC metadata and no false durability", status, failedOutcome.err)
	}
}

func bindPublicRecording(t *testing.T, service recordings.Service, id recordings.RecordingID, sessionID, path string) recordings.RecordingStatusFacts {
	t.Helper()
	opened, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: id, Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}, Artifact: recordings.RecordingArtifactReference(path),
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	t.Cleanup(func() {
		_, _ = service.StopRecording(recordings.StopRecordingRequest{RecordingID: id})
	})
	return opened.Status
}

func recordPublicRecordingFact(t *testing.T, service recordings.Service, status recordings.RecordingStatusFacts, id string, sequence recordings.CanonicalEventSequence, at time.Time) recordings.CanonicalEvent {
	t.Helper()
	event := recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Scope: status.Scope, RecordedAt: at, Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{StreamGenerationID: string(status.RecordingID), Sequence: sequence},
		Kind:   "WORK_REQUEST", Payload: `{"type":"FACTORY_REQUEST_BATCH","works":[]}`,
	}
	if sequence == 0 {
		event.Kind = "RUN_REQUEST"
		event.Payload = `{"recordedAt":"` + at.Format(time.RFC3339Nano) + `","factory":{"id":"factory-history","name":"history","metadata":{"factory_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}}}`
	}
	_, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: status.RecordingID, Event: event,
	})
	if err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	return event
}

type absentRecordingClock struct{}

func (*absentRecordingClock) Now() time.Time { panic("typed-nil process clock activated") }

func TestBuildProcessRejectsTypedNilRecordingClockBeforeActivation(t *testing.T) {
	t.Parallel()
	var clock *absentRecordingClock
	observed := false
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		Clock:                  clock,
		RecordingsRootObserver: func(recordings.Service) { observed = true },
		RecordingWriteFile:     func(string, []byte) error { t.Error("recording activated before invalid clock rejection"); return nil },
	})
	if err == nil || process != nil || err.Error() != "build application process: Clock must not be typed-nil; omit it to select the default" {
		t.Fatalf("BuildProcess = %v, %v; want typed-nil Clock rejection at the root boundary", process, err)
	}
	if observed {
		t.Fatal("invalid clock published a usable Recordings root")
	}
}

// The prefix is copied only after the public synchronous flush returns while
// step two is held at the command edge. No terminal metadata is edited and no
// OS kill/restart claim is made; I01 owns that separate production boundary.
func TestExecuteResumesUnfinalizedPrefixWithoutRepeatingCompletedWork(t *testing.T) {
	t.Parallel()
	dir := scaffoldRecordingContinuation(t)
	home := t.TempDir()
	sourcePath := filepath.Join(home, "source.json")
	prefixPath := filepath.Join(home, "prefix.json")
	successorPath := filepath.Join(home, "successor.json")
	sourceID := "019a07c0-0000-7000-8000-000000000041"
	successorID := "019a07c0-0000-7000-8000-000000000042"
	runner := &recordingContinuationRunner{secondStarted: make(chan struct{})}
	var source recordings.Service
	process := support.BuildProcess(t, recordingContinuationEdges(runner, sourceID, &source))
	inputs := recordingContinuationInputs(t, dir, home, []string{"--record", sourcePath}, true)
	ctx, cancel := context.WithCancel(inputs.Context)
	inputs.Context = ctx
	done := executeGatedRecordingCommand(t, process, inputs, cancel)
	select {
	case <-runner.secondStarted:
	case err := <-done:
		t.Fatalf("source returned before unfinished dispatch: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("unfinished provider dispatch did not start")
	}
	flushed, err := source.FlushRecording(recordings.FlushRecordingRequest{RecordingID: recordings.RecordingID(sourceID)})
	if err != nil || flushed.Status.FinalizedAt != nil || flushed.Status.FlushedThrough == nil {
		t.Fatalf("active durable prefix = (%#v, %v)", flushed, err)
	}
	prefix, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prefixPath, prefix, 0o600); err != nil {
		t.Fatal(err)
	}
	before := queryContinuationHistory(t, source, prefixPath, sourceID)
	selected, err := source.(recordings.RuntimeScopeService).LoadResumeInput(recordings.LoadResumeInputRequest{Path: prefixPath})
	if err != nil {
		t.Fatalf("public source recovery identity: %v", err)
	}
	if len(before.Dispatches) != 2 || before.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
		t.Fatalf("prefix dispatches = %#v, want completed first and unfinished second", before.Dispatches)
	}
	for _, event := range before.Events {
		if event.Kind == "RUN_RESPONSE" {
			t.Fatal("prefix contains terminal run metadata")
		}
	}
	cancel()
	select {
	case err := <-done:
		assertCanceledExecuteDiagnostic(t, err, nil)
	case <-time.After(30 * time.Second):
		t.Fatal("source cancellation did not join")
	}
	var successor recordings.Service
	successorRunner := support.NewRecordingCommandRunner("unfinished step resumed COMPLETE")
	replacement := support.BuildProcess(t, recordingContinuationEdges(successorRunner, successorID, &successor))
	resumeInput, err := successor.(recordings.RuntimeScopeService).LoadResumeInput(recordings.LoadResumeInputRequest{Path: prefixPath})
	if err != nil || resumeInput.Input.Legacy == nil || resumeInput.RecoveryMetadata.SourceRecordingID == "" {
		t.Fatalf("selected public resume identity = (%#v, %v)", resumeInput.RecoveryMetadata, err)
	}
	if resumeInput.RecoveryMetadata != selected.RecoveryMetadata || resumeInput.Input.ArtifactDigest != selected.Input.ArtifactDigest ||
		resumeInput.SourceCanonicalSessionID != selected.SourceCanonicalSessionID {
		t.Fatal("replacement graph changed selected source recovery identity")
	}
	resumed := recordingContinuationInputs(t, dir, home, []string{"--resume", prefixPath, "--record", successorPath}, false)
	if err := replacement.Execute(resumed.Input); err != nil {
		t.Fatalf("Execute resume: %v\n%s", err, resumed.Stderr())
	}
	after := queryContinuationHistory(t, successor, successorPath, successorID)
	assertContinuationFacts(t, before, after)
	if got := len(successorRunner.Requests()); got != 1 {
		t.Fatalf("successor provider calls = %d, want only unfinished step", got)
	}
	assertContinuationExport(t, successor, after, successorID, successorPath)
	if got := len(successorRunner.Requests()); got != 1 {
		t.Fatalf("public export/replay executed provider: %d calls", got)
	}
	unchanged, err := os.ReadFile(prefixPath)
	if err != nil || !reflect.DeepEqual(prefix, unchanged) {
		t.Fatalf("resume changed its selected source bytes: %v", err)
	}
}

type recordingContinuationRunner struct {
	calls         atomic.Int32
	secondStarted chan struct{}
}

func (runner *recordingContinuationRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if runner.calls.Add(1) == 1 {
		return support.NewStaticSuccessCommandRunner("first step COMPLETE").Run(ctx, request)
	}
	close(runner.secondStarted)
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

func recordingContinuationEdges(runner platformprocess.CommandRunner, id string, observer *recordings.Service) serviceedges.Edges {
	return serviceedges.Edges{
		ProviderCommandRunner:                    runner,
		RecordingsRootObserver:                   func(service recordings.Service) { *observer = service },
		FactorySessionRuntimeInstanceIDGenerator: func() string { return id },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
		RecordingReadFile:  os.ReadFile,
		RecordingWriteFile: func(path string, data []byte) error { return os.WriteFile(path, data, 0o600) },
	}
}

func recordingContinuationInputs(t *testing.T, dir, home string, selection []string, admit bool) *support.CapturedInputs {
	t.Helper()
	args := []string{"you", "run", "--dir", dir, "--quiet"}
	if admit {
		path := filepath.Join(home, "work.json")
		if err := os.WriteFile(path, []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"continued-work","workTypeName":"task","payload":{"subject":"retained lineage"}}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--work", path)
	}
	inputs := support.FakeInputs(t.Context(), append(args, selection...))
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	return inputs
}

func queryContinuationHistory(t *testing.T, service recordings.Service, path, id string) recordings.HistoricalRecordingQueryResult {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: recordings.RecordingID(id),
			Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}},
	})
	if err != nil {
		t.Fatalf("public continuation history: %v", err)
	}
	return history
}

func assertContinuationFacts(t *testing.T, before, after recordings.HistoricalRecordingQueryResult) {
	t.Helper()
	byID := make(map[recordings.CanonicalEventID]recordings.CanonicalEvent)
	for index, event := range after.Events {
		if _, exists := byID[event.ID]; exists || (index > 0 && event.Sequence <= after.Events[index-1].Sequence) {
			t.Fatal("successor history lost unique admitted order")
		}
		byID[event.ID] = event
	}
	for _, event := range before.Events {
		got, ok := byID[event.ID]
		if !ok {
			t.Fatalf("successor lost admitted event %s", event.ID)
		}
		assertPublicHistoricalFact(t, got, event)
	}
	if len(after.Events) <= len(before.Events) || len(after.Dispatches) != 3 {
		t.Fatalf("successor did not continue selected history: %#v", after.Dispatches)
	}
	if after.Dispatches[0].ID != before.Dispatches[0].ID || after.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted ||
		after.Dispatches[1].ID != before.Dispatches[1].ID || after.Dispatches[1].Status != recordings.FactoryDispatchStatusInterrupted ||
		after.Dispatches[2].ID == before.Dispatches[1].ID || after.Dispatches[2].Status != recordings.FactoryDispatchStatusCompleted ||
		after.Dispatches[2].TransitionID != before.Dispatches[1].TransitionID {
		t.Fatalf("successor dispatches = %#v, want original completed/interrupted attempts and new completed continuation", after.Dispatches)
	}
}

func scaffoldRecordingContinuation(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "recording-continuation",
		"workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "init", "type": "INITIAL"}, {"name": "processing", "type": "PROCESSING"},
			{"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"},
		}}},
		"workers": []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}},
		"workstations": []map[string]any{
			{"name": "step-one", "worker": "worker-a", "inputs": []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "processing"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}},
			{"name": "step-two", "worker": "worker-b", "inputs": []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}},
		},
	})
	for _, worker := range []string{"worker-a", "worker-b"} {
		support.WriteAgentConfig(t, dir, worker, support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	}
	return dir
}
