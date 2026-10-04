package replay_contracts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestComposedRecordReplayUsesRootBuildProcessAndExecute proves the ordinary
// record/replay flow through the customer process boundary. BuildProcess stays
// inert until Process.Execute opens the lifecycle, and replay reconstructs the
// terminal Work state without changing the source artifact or calling a host.
func TestComposedRecordReplayUsesRootBuildProcessAndExecute(t *testing.T) {
	config := replayContractFactoryConfig()
	config["workstations"].([]map[string]any)[0]["outputSchema"] =
		`{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}`
	factoryDir := support.ScaffoldFactory(t, config)
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"composed Recordings record/replay"}`))
	artifactPath := filepath.Join(t.TempDir(), "composed-recordings.replay.json")
	effects := newComposedRecordingEffects()

	recordAPI := support.NewProcessAPIServer()
	recordProcess := buildComposedProcess(t, effects, recordAPI, support.NewStaticSuccessCommandRunner(`{"verdict":"pass"}`))
	assertComposedBuildIsInert(t, effects, artifactPath)
	recordCommand, recordURL := startComposedRun(t, recordProcess, recordAPI, factoryDir, nil, "--record", artifactPath)
	support.WaitForTerminalStatus(t, recordURL, 15*time.Second)
	assertComposedRecordedState(t, recordURL, artifactPath)
	recordCommand.Stop(t)

	recordedPayload, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read finalized recording artifact: %v", err)
	}
	if len(recordedPayload) == 0 {
		t.Fatal("finalized recording artifact is empty")
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if composedEventCount(artifact.Events, factoryapi.FactoryEventTypeDispatchRequest) == 0 || composedEventCount(artifact.Events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatalf("recording artifact events = %#v, want dispatch request and response", artifact.Events)
	}
	writeCallsAfterRecord := effects.writeCalls.Load()

	replayAPI := support.NewProcessAPIServer()
	replayRunner := &composedReplayCommandRunner{}
	replayProcess := buildComposedProcess(t, effects, replayAPI, replayRunner)
	replayCommand, replayURL := startComposedRun(t, replayProcess, replayAPI, t.TempDir(), nil, "--replay", artifactPath, "--no-record")
	support.WaitForTerminalStatus(t, replayURL, 15*time.Second)
	assertComposedReplayedState(t, replayURL)
	replayCommand.Stop(t)

	if replayRunner.calls.Load() != 0 {
		t.Fatalf("provider command calls during historical replay = %d, want 0", replayRunner.calls.Load())
	}
	if got, err := os.ReadFile(artifactPath); err != nil {
		t.Fatalf("read recording artifact after replay: %v", err)
	} else if !bytes.Equal(got, recordedPayload) {
		t.Fatal("historical replay mutated the source recording artifact")
	}
	if got := effects.writeCalls.Load(); got != writeCallsAfterRecord {
		t.Fatalf("RecordingWriteFile calls after replay = %d, want unchanged at %d", got, writeCallsAfterRecord)
	}
}

func buildComposedProcess(
	t *testing.T,
	effects *composedRecordingEffects,
	api *support.ProcessAPIServer,
	runner platformprocess.CommandRunner,
) support.Process {
	t.Helper()
	process, err := root.BuildProcess(t.Context(), effects.edges(api, runner))
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	return process
}

func assertComposedBuildIsInert(t *testing.T, effects *composedRecordingEffects, artifactPath string) {
	t.Helper()
	if got := effects.totalCalls(); got != 0 {
		t.Fatalf("Recordings edge calls during inert BuildProcess = %d, want 0", got)
	}
	if _, err := os.Stat(artifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recording artifact after inert BuildProcess: stat error = %v, want not-exist", err)
	}
}

func startComposedRun(
	t *testing.T,
	process support.Process,
	api *support.ProcessAPIServer,
	workingDirectory string,
	readiness *controlledReadinessTimers,
	recordingArgs ...string,
) (*support.ProcessCommand, string) {
	t.Helper()
	args := []string{"you", "run", "--dir", workingDirectory, "--continuously", "--with-server", "--quiet"}
	args = append(args, recordingArgs...)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = isolatedReplayEnvironmentFor(t)
	inputs.Input.WorkingDirectory = workingDirectory
	command := support.StartProcessCommand(t, process, inputs.Input)
	if readiness != nil {
		return command, readiness.WaitForURL(t, api)
	}
	return command, api.WaitForURL(t)
}

func assertComposedRecordedState(t *testing.T, baseURL, artifactPath string) {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, baseURL)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("recorded Work at task:complete = %d, want 1; listed=%#v", got, listed.Results)
	}
	events := support.GetFactoryEventsAt(t, baseURL)
	if composedLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("recorded public Factory Events missing dispatch response")
	}
	assertComposedStructuredResult(t, events)
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("recording artifact after terminal observation: %v", err)
	}
}

func assertComposedReplayedState(t *testing.T, baseURL string) {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, baseURL)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("replayed Work at task:complete = %d, want 1; listed=%#v", got, listed.Results)
	}
	events := support.GetFactoryEventsAt(t, baseURL)
	if composedLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("replayed public Factory Events missing dispatch response")
	}
	assertComposedStructuredResult(t, events)
}

func assertComposedStructuredResult(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	want := map[string]any{"verdict": "pass"}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode public dispatch response: %v", err)
		}
		if !reflect.DeepEqual(payload.StructuredResult, want) {
			t.Fatalf("public dispatch response structured result = %#v, want %#v", payload.StructuredResult, want)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal public dispatch response: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("decode public dispatch response JSON: %v", err)
		}
		rawPayload, ok := raw["payload"].(map[string]any)
		if !ok || !reflect.DeepEqual(rawPayload["structuredResult"], want) {
			t.Fatalf("public dispatch response JSON structuredResult = %#v, want %#v", rawPayload["structuredResult"], want)
		}
		return
	}
	t.Fatal("public Factory Events missing structured dispatch response")
}

func composedEventCount(events []factorydefinitions.FactoryEvent, kind factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if string(event.Type) == string(kind) {
			count++
		}
	}
	return count
}

func composedLiveEventCount(events []factoryapi.FactoryEvent, kind factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}

type composedRecordingEffects struct {
	filesystemCalls atomic.Int32
	readCalls       atomic.Int32
	writeCalls      atomic.Int32
}

func newComposedRecordingEffects() *composedRecordingEffects {
	return &composedRecordingEffects{}
}

func (effects *composedRecordingEffects) totalCalls() int32 {
	return effects.filesystemCalls.Load() + effects.readCalls.Load() + effects.writeCalls.Load()
}

func (effects *composedRecordingEffects) edges(
	api *support.ProcessAPIServer,
	providerRunner platformprocess.CommandRunner,
) serviceedges.Edges {
	return serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
		RecordingWriteFile: func(path string, payload []byte) error {
			effects.writeCalls.Add(1)
			return os.WriteFile(path, payload, 0o600)
		},
		RecordingMakeDirectories: func(path string, mode fs.FileMode) error {
			effects.filesystemCalls.Add(1)
			return os.MkdirAll(path, mode)
		},
		RecordingCreateTempFile: func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			effects.filesystemCalls.Add(1)
			return os.CreateTemp(dir, pattern)
		},
		RecordingRemovePath: func(path string) error {
			effects.filesystemCalls.Add(1)
			return os.Remove(path)
		},
		RecordingRenamePath: func(oldPath, newPath string) error {
			effects.filesystemCalls.Add(1)
			return os.Rename(oldPath, newPath)
		},
		RecordingReadFile: func(path string) ([]byte, error) {
			effects.readCalls.Add(1)
			return os.ReadFile(path)
		},
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			effects.readCalls.Add(1)
			return os.ReadFile(path)
		},
		APIServerStarter: api.Start,
	}
}

type composedReplayCommandRunner struct {
	calls atomic.Int32
}

func (runner *composedReplayCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("historical replay must not execute provider commands")
}

var _ platformprocess.CommandRunner = (*composedReplayCommandRunner)(nil)

// TestReplayUsesRecordedArtifactClockThroughComposedProcess characterizes
// regenerated public event origins, not corrected replay-clock selection.
// Each local Current Factory command opens its own ~default process/profile;
// replay's local selection is the customer boundary being characterized.
func TestReplayUsesRecordedArtifactClockThroughComposedProcess(t *testing.T) {
	for _, name := range []string{"default-process-clock", "explicit-process-clock"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			characterizeComposedReplayClock(t, name == "explicit-process-clock")
		})
	}
}

func characterizeComposedReplayClock(t *testing.T, explicit bool) {
	t.Helper()
	payload := selectedTickArtifactPayload(t, true)
	artifactPath := filepath.Join(t.TempDir(), "known-ticks.replay.json")
	if err := os.WriteFile(artifactPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	api := support.NewProcessAPIServer()
	runner := &composedReplayCommandRunner{}
	effects := newComposedRecordingEffects()
	edges := effects.edges(api, runner)
	processTime := time.Date(2040, time.January, 1, 0, 0, 0, 0, time.UTC)
	scheduler := platformclock.NewDeterministic(processTime.Add(24*time.Hour), time.Second)
	var readiness *controlledReadinessTimers
	if explicit {
		readiness = newControlledReadinessTimers(scheduler)
		edges.Clock = replayOriginClock{at: processTime}
		edges.ProcessScheduler = readiness
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("root.BuildProcess: %v", err)
	}
	support.CleanupProcess(t, process)
	before := time.Now()
	command, url := startComposedRun(t, process, api, t.TempDir(), readiness, "--replay", artifactPath, "--no-record")
	support.WaitForTerminalStatus(t, url, 15*time.Second)
	listed := support.ListDefaultSessionWork(t, url)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("replayed terminal Work count = %d, want 1", got)
	}
	events := support.GetFactoryEventsAt(t, url)
	after := time.Now()
	seenRun, seenWork := false, false
	for _, event := range events {
		at := event.Context.EventTime
		t.Logf("regenerated replay fact id=%s type=%s tick=%d time=%s explicit-process-clock=%t", event.Id, event.Type, event.Context.Tick, at.Format(time.RFC3339Nano), explicit)
		if explicit && !at.Equal(processTime) {
			t.Errorf("replay fact %s time = %v, want selected process time %v", event.Id, at, processTime)
		}
		if !explicit && (at.Before(before) || at.After(after)) {
			t.Errorf("replay fact %s time = %v, want wall time in [%v, %v]", event.Id, at, before, after)
		}
		if event.Type == factoryapi.FactoryEventTypeRunRequest {
			seenRun = true
		}
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			seenWork = true
			if event.Context.Tick != 1 {
				t.Errorf("replayed Work Request tick = %d, want recorded tick 1", event.Context.Tick)
			}
		}
	}
	if !seenRun || !seenWork {
		t.Fatalf("public replay facts missing Run Request or Work Request: run=%t work=%t", seenRun, seenWork)
	}
	if explicit {
		// C04: independent scheduling cannot rewrite historical public facts.
		scheduler.SetTick(60)
		if got := support.GetFactoryEventsAt(t, url); !reflect.DeepEqual(got, events) {
			t.Fatal("scheduler advancement changed terminal replay history")
		}
		if got := support.ListDefaultSessionWork(t, url); !reflect.DeepEqual(got, listed) {
			t.Fatal("scheduler advancement changed terminal replay Work")
		}
	}
	command.Stop(t)
	if runner.calls.Load() != 0 {
		t.Fatalf("replay provider calls = %d, want 0", runner.calls.Load())
	}
	if got, err := os.ReadFile(artifactPath); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("source artifact changed after replay: %v", err)
	}
}

type replayOriginClock struct{ at time.Time }

func (clock replayOriginClock) Now() time.Time { return clock.at }

// controlledReadinessTimers delivers startup polls at the injected timer edge,
// without advancing customer fact time or the metrics sampling origin. All
// other timers retain the selected scheduler's logical deadlines.
type controlledReadinessTimers struct {
	platformclock.TimerSource
	polls chan *platformclock.Deterministic
}

func newControlledReadinessTimers(scheduler platformclock.TimerSource) *controlledReadinessTimers {
	return &controlledReadinessTimers{TimerSource: scheduler, polls: make(chan *platformclock.Deterministic)}
}

func (scheduler *controlledReadinessTimers) NewTimer(duration time.Duration) platformclock.Timer {
	if duration != 10*time.Millisecond {
		return scheduler.TimerSource.NewTimer(duration)
	}
	poll := platformclock.NewDeterministic(scheduler.Now(), duration)
	timer := poll.NewTimer(duration)
	scheduler.polls <- poll
	return timer
}

func (scheduler *controlledReadinessTimers) After(duration time.Duration) <-chan time.Time {
	return scheduler.NewTimer(duration).C()
}

// WaitForURL drives only registered readiness polls until the real transport
// starts. Channel registration/delivery synchronizes startup; the transport wait timeout
// is only a deadlock ceiling. The transport waiter completes before this observation returns.
func (scheduler *controlledReadinessTimers) WaitForURL(t testing.TB, server *support.ProcessAPIServer) string {
	t.Helper()
	type result struct {
		url string
		err error
	}
	ready := make(chan result, 1)
	go func() {
		url, err := server.WaitForBaseURL(60 * time.Second)
		ready <- result{url: url, err: err}
	}()
	for {
		select {
		case poll := <-scheduler.polls:
			poll.SetTick(1)
		case observed := <-ready:
			if observed.err != nil {
				t.Fatal(observed.err)
			}
			return observed.url
		}
	}
}

func TestControlledReadinessPreservesSelectedSchedulerDeadlines(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	selected := platformclock.NewDeterministic(base, time.Millisecond)
	scheduler := newControlledReadinessTimers(selected)
	metrics := scheduler.NewTimer(5 * time.Millisecond)
	defer metrics.Stop()
	deadline := scheduler.NewTimer(time.Second)
	defer deadline.Stop()
	server := support.NewProcessAPIServer()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		// Exercise multiple readiness registrations, including the After seam.
		<-scheduler.After(10 * time.Millisecond)
		poll := scheduler.NewTimer(10 * time.Millisecond)
		<-poll.C()
		poll.Stop()
		done <- server.Start(ctx, platformhttpserver.StartRequest{Handler: http.NotFoundHandler()})
	}()
	url := scheduler.WaitForURL(t, server)
	if !strings.HasPrefix(url, "http://") || !scheduler.Now().Equal(base) {
		t.Fatalf("startup URL/time = %q/%v, want HTTP URL and %v", url, scheduler.Now(), base)
	}
	for _, timer := range []platformclock.Timer{metrics, deadline} {
		select {
		case <-timer.C():
			t.Fatal("readiness delivery fired a selected scheduler timer")
		default:
		}
	}
	selected.SetTick(5)
	select {
	case <-metrics.C():
	default:
		t.Fatal("selected scheduler did not deliver the metrics deadline")
	}
	select {
	case <-deadline.C():
		t.Fatal("startup ceiling fired before its selected logical deadline")
	default:
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
