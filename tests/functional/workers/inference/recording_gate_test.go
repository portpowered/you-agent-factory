package inference_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWSRFT004DurableOpeningGatesProviderHandoff proves the opening barrier
// through the canonical root-built process. The recording writer is an
// injected deterministic persistence edge wrapped only to observe accepted
// order and inject one opening persistence failure; the ProviderCommandRunner
// remains the only provider effect at this boundary.
//
// WSR-FT-004: durable opening before the first provider call and zero provider
// calls when opening durability fails.
func TestWSRFT004DurableOpeningGatesProviderHandoff(t *testing.T) {
	t.Parallel()
	t.Run("durable opening precedes provider call", func(t *testing.T) {
		t.Parallel()
		probe := newWSRFT004RecordingProbe(t, false)
		runner := newWSRFT004ProviderRunner(t, probe)
		dir := wsrFT004Factory(t)

		runWSRFT004FactoryShared(t, dir, runner, probe)

		if got := runner.CallCount(); got != 1 {
			t.Fatalf("provider command calls = %d, want exactly one call after durable opening", got)
		}
		probe.assertOpeningBeforeProvider(t)
		_ = probe.LiveProjection(t)
	})

	t.Run("opening durability failure prevents provider call", func(t *testing.T) {
		t.Parallel()
		probe := newWSRFT004RecordingProbe(t, true)
		runner := newWSRFT004ProviderRunner(t, probe)
		dir := wsrFT004Factory(t)

		runWSRFT004FactoryShared(t, dir, runner, probe)

		if got := runner.CallCount(); got != 0 {
			t.Fatalf("provider command calls = %d, want zero after opening durability failure", got)
		}
		probe.assertOpeningFailure(t)
	})
}

// TestWSRFT005CompletedWorkerReplayParity proves the root-composed durable
// read side reloads the finalized source history and returns the exact live
// reduction. The replay call is made after the provider run and its runner
// count must remain unchanged, proving replay does not reopen provider or
// Worker execution.
//
// WSR-FT-005: exact finalized live-to-reloaded-replay equality, terminal-last
// behavior, no duplicates, and zero provider invocations during replay.
func TestWSRFT005CompletedWorkerReplayParity(t *testing.T) {
	t.Parallel()
	probe := newWSRFT004RecordingProbe(t, false)
	runner := newWSRFT004ProviderRunner(t, probe)
	dir := wsrFT004Factory(t)

	reader := runWSRFT004FactoryShared(t, dir, runner, probe)

	recordingID, workerSessionID := probe.RecordingIdentity(t)
	live := probe.LiveProjection(t)
	snapshot, err := reader.LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) error = %v", recordingID, err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("durable Worker snapshot = %#v, want one completed session", snapshot)
	}

	providerCallsBeforeReplay := runner.CallCount()
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
		Snapshot:        snapshot,
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(%q, %q) error = %v", recordingID, workerSessionID, err)
	}
	if providerCallsAfterReplay := runner.CallCount(); providerCallsAfterReplay != providerCallsBeforeReplay {
		t.Fatalf("provider command calls changed during replay: before=%d after=%d", providerCallsBeforeReplay, providerCallsAfterReplay)
	}
	if !reflect.DeepEqual(live, replayed.Projection) {
		t.Fatalf("live projection = %#v, replay projection = %#v", live, replayed.Projection)
	}
	if !replayed.Projection.Complete || replayed.Projection.Terminal == nil {
		t.Fatalf("replayed projection = %#v, want completed terminal history", replayed.Projection)
	}
	last := replayed.Projection.Records[len(replayed.Projection.Records)-1]
	if replayed.Projection.Terminal.Position != last.ID.Position {
		t.Fatalf("terminal position = %d, last record position = %d; terminal must be last", replayed.Projection.Terminal.Position, last.ID.Position)
	}
	identities := make(map[events.AppendIdentity]struct{}, len(replayed.Projection.Records))
	for _, record := range replayed.Projection.Records {
		if _, exists := identities[record.Identity()]; exists {
			t.Fatalf("replayed history contains duplicate source identity %#v", record.Identity())
		}
		identities[record.Identity()] = struct{}{}
	}
}

// TestWSRFT008PostHandoffRecordingLossPreservesExecutionTruth injects a
// capture failure after the durable opening barrier for both a successful and
// an unsuccessful provider result. The Work execution remains authoritative;
// only the Worker recording projection degrades to the readable prefix.
//
// WSR-FT-008: post-handoff recording loss never rewrites execution outcome,
// and a correlated terminal fact upgrades the surviving prefix to DEGRADED.
func TestWSRFT008PostHandoffRecordingLossPreservesExecutionTruth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fixture    string
		exitCode   int
		wantPhase  workers.Phase
		wantStatus recordings.WorkerRecordingStatus
	}{
		{name: "successful execution", fixture: "executor_success", wantPhase: workers.PhaseCompleted, wantStatus: recordings.WorkerRecordingStatusDegraded},
		{name: "failed execution", fixture: "executor_failure_no_arcs", exitCode: 1, wantPhase: workers.PhaseFailed, wantStatus: recordings.WorkerRecordingStatusDegraded},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			probe := newWSRFT004RecordingProbe(t, false)
			probe.failPosition = 2
			runner := newWSRFT004ProviderRunner(t, probe)
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, test.fixture))
			support.ClearSeedInputs(t, dir)
			loaded := loadOpeningRecordFixture(t, "codex", "success")
			support.WriteAgentConfig(t, dir, "worker", sharedInferenceWithExecutorProvider(
				support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
				"CODEX",
			))
			queueWSRFT004ProviderResult(t, runner, test.exitCode, "injected execution result")
			runSharedInferenceFactory(t, dir, sharedInferenceScenario{
				commandRunner:         runner,
				workerRecordingWriter: probe,
				submittedWork:         sharedInferenceWork("WSR-FT-008 recording loss"),
			}, sharedInferenceScenarioTimeout)
			reader := recordings.WorkerRecordingReader(probe)
			recordingID, workerSessionID := probe.RecordingIdentity(t)
			snapshot, err := reader.LoadWorkerRecording(t.Context(), recordingID)
			if err != nil {
				t.Fatalf("LoadWorkerRecording(%q) error = %v", recordingID, err)
			}
			if len(snapshot.Sessions) != 1 {
				t.Fatalf("durable Worker snapshot = %#v, want one session", snapshot)
			}
			session := snapshot.Sessions[0]
			if session.Status != test.wantStatus || session.Failure != "PERSISTENCE_FAILED" {
				t.Fatalf("durable Worker session = %#v, want DEGRADED/PERSISTENCE_FAILED", session)
			}
			if session.ExecutionTerminal == nil || session.ExecutionTerminal.Phase != test.wantPhase || session.ExecutionTerminal.Status != string(test.wantPhase) {
				t.Fatalf("durable execution terminal = %#v, want %q execution truth", session.ExecutionTerminal, test.wantPhase)
			}
			if len(session.Records) != 1 || session.Records[0].ID.Position != 1 {
				t.Fatalf("durable prefix = %#v, want only opening record", session.Records)
			}
			replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
				Snapshot: snapshot, WorkerSessionID: workerSessionID,
			})
			if err != nil {
				t.Fatalf("ReplayWorkerRecording(%q) error = %v", recordingID, err)
			}
			if replayed.Projection.Status != test.wantStatus || replayed.Projection.Terminal != nil {
				t.Fatalf("replayed projection = %#v, want degraded prefix without fabricated terminal", replayed.Projection)
			}
			if runner.CallCount() != 1 {
				t.Fatalf("provider command calls = %d, want exactly one", runner.CallCount())
			}
		})
	}
}

func wsrFT004Factory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, dir)
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	support.WriteAgentConfig(t, dir, "worker", sharedInferenceWithExecutorProvider(
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
		"CODEX",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"WSR-FT-004 durable opening"}`))
	return dir
}

func runWSRFT004FactoryShared(
	t *testing.T,
	dir string,
	runner *wsrFT004ProviderRunner,
	writer recordings.WorkerRecordingWriter,
) recordings.WorkerRecordingReader {
	t.Helper()
	support.ClearSeedInputs(t, dir)
	queueWSRFT004ProviderResult(t, runner, 0)
	runSharedInferenceFactory(t, dir, sharedInferenceScenario{
		commandRunner:         runner,
		workerRecordingWriter: writer,
		submittedWork:         sharedInferenceWork("WSR-FT-004 durable opening"),
	}, sharedInferenceScenarioTimeout)
	reader, ok := writer.(recordings.WorkerRecordingReader)
	if !ok || reader == nil {
		t.Fatalf("recording writer type %T does not expose a reader", writer)
	}
	return reader
}

func runWSRFT004FactoryWithProcess(
	t *testing.T,
	dir string,
	edges serviceedges.Edges,
) recordings.WorkerRecordingReader {
	t.Helper()
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerRunner, ok := edges.ProviderCommandRunner.(*wsrFT004ProviderRunner)
	if !ok {
		t.Fatalf("provider runner type = %T, want WSR-FT-004 probe runner", edges.ProviderCommandRunner)
	}
	queueWSRFT004ProviderResult(t, providerRunner, exitCode)

	processValue, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	process := processValue
	reader := process.WorkerRecordingReader()
	if reader == nil {
		t.Fatal("root-built process returned a nil Recordings reader")
	}
	support.CleanupProcess(t, process)
	recordPath := filepath.Join(t.TempDir(), "wsr-ft-004.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--quiet", "--record", recordPath,
	})
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("recorded factory Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	return reader
}

func queueWSRFT004ProviderResult(
	t *testing.T,
	runner *wsrFT004ProviderRunner,
	exitCode int,
	stderr ...string,
) {
	t.Helper()
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	resultStderr := loaded.Stderr
	if len(stderr) > 0 {
		resultStderr = stderr[0]
	}
	runner.delegate.Queue(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(resultStderr),
		ExitCode: exitCode,
	})
}

type wsrFT004RecordingProbe struct {
	delegate          recordings.WorkerRecordingWriter
	failOpening       bool
	failPosition      events.AggregateSequence
	failFailureMarker bool

	mu           sync.Mutex
	events       []string
	failure      *recordings.WorkerRecordingFailure
	failureOnce  sync.Once
	recordingID  string
	workerID     string
	live         recordings.WorkerRecordingProjection
	liveByWorker map[string]recordings.WorkerRecordingProjection
}

func newWSRFT004RecordingProbe(t *testing.T, failOpening bool) *wsrFT004RecordingProbe {
	t.Helper()
	return &wsrFT004RecordingProbe{
		delegate:     newWSRFT004RecordingStore(),
		failOpening:  failOpening,
		liveByWorker: make(map[string]recordings.WorkerRecordingProjection),
	}
}

type wsrFT004RecordingStore struct {
	mu        sync.Mutex
	snapshots map[string]recordings.WorkerRecordingSnapshot
}

func newWSRFT004RecordingStore() *wsrFT004RecordingStore {
	return &wsrFT004RecordingStore{snapshots: make(map[string]recordings.WorkerRecordingSnapshot)}
}

func (store *wsrFT004RecordingStore) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot := cloneWSRFT004Snapshot(store.snapshots[record.RecordingID])
	snapshot.RecordingID = record.RecordingID
	session := findWSRFT004Session(&snapshot, record.WorkerSessionID)
	history := append([]events.Record(nil), session.Records...)
	history = append(history, record.Record.Detached())
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       record.RecordingID,
		WorkerSessionID:   record.WorkerSessionID,
		Topic:             record.Record.ID.Topic,
		Failure:           session.Failure,
		ExecutionTerminal: session.ExecutionTerminal,
		Records:           history,
	})
	if err != nil {
		return err
	}
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.ExecutionTerminal = projection.ExecutionTerminal
	session.Records = cloneWSRFT004Records(projection.Records)
	store.snapshots[record.RecordingID] = snapshot
	return nil
}

func (store *wsrFT004RecordingStore) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot := cloneWSRFT004Snapshot(store.snapshots[failure.RecordingID])
	snapshot.RecordingID = failure.RecordingID
	session := findWSRFT004Session(&snapshot, failure.WorkerSessionID)
	session.Topic = failure.Topic
	session.Failure = failure.Code
	session.ExecutionTerminal = failure.ExecutionTerminal
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       snapshot.RecordingID,
		WorkerSessionID:   session.WorkerSessionID,
		Topic:             session.Topic,
		Failure:           session.Failure,
		ExecutionTerminal: session.ExecutionTerminal,
		Records:           session.Records,
	})
	if err != nil {
		return err
	}
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.ExecutionTerminal = projection.ExecutionTerminal
	session.Records = cloneWSRFT004Records(projection.Records)
	store.snapshots[failure.RecordingID] = snapshot
	return nil
}

func (store *wsrFT004RecordingStore) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot, ok := store.snapshots[recordingID]
	if !ok {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("recording %q not found", recordingID)
	}
	return cloneWSRFT004Snapshot(snapshot), nil
}

func findWSRFT004Session(
	snapshot *recordings.WorkerRecordingSnapshot,
	workerSessionID string,
) *recordings.WorkerSessionRecordingSnapshot {
	for index := range snapshot.Sessions {
		if snapshot.Sessions[index].WorkerSessionID == workerSessionID {
			return &snapshot.Sessions[index]
		}
	}
	snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{
		WorkerSessionID: workerSessionID,
	})
	return &snapshot.Sessions[len(snapshot.Sessions)-1]
}

func cloneWSRFT004Snapshot(
	snapshot recordings.WorkerRecordingSnapshot,
) recordings.WorkerRecordingSnapshot {
	clone := snapshot
	clone.Sessions = make([]recordings.WorkerSessionRecordingSnapshot, len(snapshot.Sessions))
	for index, session := range snapshot.Sessions {
		clone.Sessions[index] = session
		clone.Sessions[index].Records = cloneWSRFT004Records(session.Records)
	}
	return clone
}

func cloneWSRFT004Records(records []events.Record) []events.Record {
	clone := make([]events.Record, len(records))
	for index, record := range records {
		clone[index] = record.Detached()
	}
	return clone
}

func (probe *wsrFT004RecordingProbe) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	if probe.failOpening && record.Record.ID.Position == 1 {
		probe.mu.Lock()
		probe.events = append(probe.events, "opening-rejected")
		probe.mu.Unlock()
		return errors.New("injected opening durability failure")
	}
	if probe.failPosition > 0 && record.Record.ID.Position == probe.failPosition {
		probe.mu.Lock()
		probe.events = append(probe.events, fmt.Sprintf("post-opening-failure:%d", record.Record.ID.Position))
		probe.mu.Unlock()
		return errors.New("injected post-opening durability failure")
	}
	if err := probe.delegate.PersistWorkerRecord(ctx, record); err != nil {
		return err
	}
	probe.mu.Lock()
	probe.recordingID = record.RecordingID
	probe.workerID = record.WorkerSessionID
	previous := probe.liveByWorker[record.WorkerSessionID]
	history := append([]events.Record(nil), previous.Records...)
	history = append(history, record.Record.Detached())
	live, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:     record.RecordingID,
		WorkerSessionID: record.WorkerSessionID,
		Topic:           record.Record.ID.Topic,
		Records:         history,
	})
	if err != nil {
		probe.mu.Unlock()
		return fmt.Errorf("reduce accepted Worker history: %w", err)
	}
	probe.liveByWorker[record.WorkerSessionID] = live
	probe.live = live
	probe.events = append(probe.events, fmt.Sprintf("durable:%d", record.Record.ID.Position))
	probe.mu.Unlock()
	return nil
}

func (probe *wsrFT004RecordingProbe) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	reader, ok := probe.delegate.(recordings.WorkerRecordingReader)
	if !ok {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecording(ctx, recordingID)
}

func (probe *wsrFT004RecordingProbe) RecordingIdentity(t *testing.T) (string, string) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.recordingID == "" || probe.workerID == "" {
		t.Fatalf("recording identity = (%q, %q), want durable Worker identity", probe.recordingID, probe.workerID)
	}
	return probe.recordingID, probe.workerID
}

func (probe *wsrFT004RecordingProbe) LiveProjection(t *testing.T) recordings.WorkerRecordingProjection {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if !probe.live.Complete {
		t.Fatalf("live Worker projection = %#v, want completed history", probe.live)
	}
	projection := probe.live
	projection.Records = make([]events.Record, len(probe.live.Records))
	for index, record := range probe.live.Records {
		projection.Records[index] = record.Detached()
	}
	projection.Opening = probe.live.Opening.Detached()
	if probe.live.Terminal != nil {
		terminal := *probe.live.Terminal
		projection.Terminal = &terminal
	}
	return projection
}

func (probe *wsrFT004RecordingProbe) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	if probe.failFailureMarker {
		return errors.New("injected degradation-marker persistence failure")
	}
	writer, ok := probe.delegate.(recordings.WorkerRecordingFailureWriter)
	if !ok {
		return errors.New("durable Worker writer has no failure side")
	}
	if err := writer.PersistWorkerRecordingFailure(ctx, failure); err != nil {
		return err
	}
	probe.failureOnce.Do(func() {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		copy := failure
		probe.failure = &copy
		probe.events = append(probe.events, "failure:"+failure.Code)
	})
	return nil
}

func (probe *wsrFT004RecordingProbe) recordProviderCall() {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.events = append(probe.events, "provider-call")
}

func (probe *wsrFT004RecordingProbe) assertOpeningBeforeProvider(t *testing.T) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	opening, provider := eventIndexes(probe.events, "durable:1", "provider-call")
	if opening < 0 || provider < 0 || opening >= provider {
		t.Fatalf("durable Worker opening/provider order = %#v, want durable:1 before provider-call", probe.events)
	}
}

func (probe *wsrFT004RecordingProbe) assertOpeningFailure(t *testing.T) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.failure == nil || probe.failure.Code != "PERSISTENCE_FAILED" {
		t.Fatalf("durable opening failure = %#v, want PERSISTENCE_FAILED classification", probe.failure)
	}
	if probe.failure.RecordingID == "" || probe.failure.WorkerSessionID == "" || probe.failure.Topic == "" {
		t.Fatalf("durable opening failure identity = %#v, want recording, Worker Session, and topic", probe.failure)
	}
	if opening, provider := eventIndexes(probe.events, "durable:1", "provider-call"); opening >= 0 || provider >= 0 {
		t.Fatalf("opening failure events = %#v, want no durable opening or provider call", probe.events)
	}
}

func eventIndexes(events []string, first, second string) (int, int) {
	firstIndex, secondIndex := -1, -1
	for index, event := range events {
		if event == first && firstIndex < 0 {
			firstIndex = index
		}
		if event == second && secondIndex < 0 {
			secondIndex = index
		}
	}
	return firstIndex, secondIndex
}

type wsrFT004ProviderRunner struct {
	delegate *testutil.ProviderCommandRunner
	probe    *wsrFT004RecordingProbe
}

func newWSRFT004ProviderRunner(t *testing.T, probe *wsrFT004RecordingProbe) *wsrFT004ProviderRunner {
	t.Helper()
	return &wsrFT004ProviderRunner{delegate: testutil.NewProviderCommandRunner(), probe: probe}
}

func (runner *wsrFT004ProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.probe.recordProviderCall()
	return runner.delegate.Run(ctx, request)
}

func (runner *wsrFT004ProviderRunner) CallCount() int {
	return runner.delegate.CallCount()
}
