package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each immutable time pair gets an isolated process/profile and explicit session.
// Provider completion and timer registration are barriers, never elapsed-time proof.
func TestSelectedProcessTimeControlsRuntimeFacts(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	factClock := platformclock.NewDeterministic(base, time.Hour)
	facts := &selectedTimeSource{clock: factClock, readiness: newControlledReadinessTimers(factClock)}
	release := make(chan struct{})
	runner := &selectedTimeRunner{started: make(chan struct{}), delegate: support.NewGatedSuccessCommandRunner("selected time COMPLETE", release)}
	fixture := startSelectedTimeRun(t, facts, nil, runner)
	awaitSelectedTimeSignal(t, runner.started)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeWorkRequest, base)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeDispatchRequest, base)
	facts.clock.SetTick(1)
	close(release)
	support.WaitForSessionTerminalStatus(t, fixture.url, fixture.session, 30*time.Second)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeDispatchResponse, base.Add(time.Hour))
	assertSelectedTimeWork(t, fixture)
	fixture.command.Stop(t)
}

func TestSelectedSchedulerControlsRuntimeMemorySamples(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := &selectedTimeNowOnlySource{clock: platformclock.NewDeterministic(base, time.Hour)}
	scheduler := &selectedTimeScheduler{
		Deterministic: platformclock.NewDeterministic(base.Add(24*time.Hour), time.Millisecond),
		registered:    make(chan struct{}, 16),
	}
	scheduler.readiness = newControlledReadinessTimers(scheduler.Deterministic)
	release := make(chan struct{})
	runner := &selectedTimeRunner{started: make(chan struct{}), delegate: support.NewGatedSuccessCommandRunner("separate scheduler COMPLETE", release)}
	fixture := startSelectedTimeRun(t, facts, scheduler, runner)
	awaitSelectedTimeSignal(t, runner.started)
	awaitSelectedTimeSignal(t, scheduler.registered)
	// Readiness delivery leaves the sampling scheduler at a known origin.
	origin := scheduler.Now()
	if !origin.Equal(base.Add(24 * time.Hour)) {
		t.Fatalf("post-start sampling origin = %v, want %v", origin, base.Add(24*time.Hour))
	}
	originTick := int(origin.Sub(base.Add(24*time.Hour)) / time.Millisecond)
	assertSelectedMemorySamples(t, fixture.metrics, 1)
	// Advancing facts does not progress B's timer; the registered timer has not fired.
	facts.clock.SetTick(1)
	assertSelectedMemorySamples(t, fixture.metrics, 1)
	// Before the registered 5ms poll deadline, no new poll is registered.
	scheduler.SetTick(originTick + 4)
	select {
	case <-scheduler.registered:
		t.Fatal("metrics poll fired before its logical deadline")
	default:
	}
	assertSelectedMemorySamples(t, fixture.metrics, 1)
	// A completed poll before the 10s memory deadline still has one sample.
	scheduler.SetTick(originTick + 9000)
	awaitSelectedTimeSignal(t, scheduler.registered)
	assertSelectedMemorySamples(t, fixture.metrics, 1)
	scheduler.SetTick(originTick + 10000)
	awaitSelectedTimeSignal(t, scheduler.registered)
	assertSelectedMemorySamples(t, fixture.metrics, 2)
	close(release)
	support.WaitForSessionTerminalStatus(t, fixture.url, fixture.session, 30*time.Second)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeWorkRequest, base)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeDispatchRequest, base)
	assertSelectedRuntimeEventTime(t, fixture, factoryapi.FactoryEventTypeDispatchResponse, base.Add(time.Hour))
	assertSelectedTimeWork(t, fixture)
	fixture.command.Stop(t)
}

// Hide replay TickSetter: this source is controlled by the operator fixture,
// rather than the runtime's recorded-tick advancement protocol.
type selectedTimeSource struct {
	clock     *platformclock.Deterministic
	readiness *controlledReadinessTimers
}

func (source *selectedTimeSource) Now() time.Time { return source.clock.Now() }
func (source *selectedTimeSource) NewTimer(d time.Duration) platformclock.Timer {
	return source.readiness.NewTimer(d)
}
func (source *selectedTimeSource) After(d time.Duration) <-chan time.Time {
	return source.readiness.After(d)
}

type selectedTimeNowOnlySource struct{ clock *platformclock.Deterministic }

func (source *selectedTimeNowOnlySource) Now() time.Time { return source.clock.Now() }

type selectedTimeScheduler struct {
	*platformclock.Deterministic
	registered chan struct{}
	readiness  *controlledReadinessTimers
}

func (clock *selectedTimeScheduler) NewTimer(duration time.Duration) platformclock.Timer {
	timer := clock.readiness.NewTimer(duration)
	// Only a metrics rearm proves the preceding metrics observation completed.
	// Startup polls and its failure ceiling cannot satisfy this barrier.
	if duration == 5*time.Millisecond {
		clock.registered <- struct{}{}
	}
	return timer
}

type selectedTimeRunner struct {
	started  chan struct{}
	delegate platformprocess.CommandRunner
}

func (runner *selectedTimeRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	close(runner.started)
	return runner.delegate.Run(ctx, request)
}

type selectedTimeFixture struct {
	url, session, metrics string
	command               *support.ProcessCommand
}

func startSelectedTimeRun(t *testing.T, facts platformclock.Source, scheduler platformclock.TimerSource, runner platformprocess.CommandRunner) selectedTimeFixture {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "selected-process-time")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"selected time"}`))
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
	return startSelectedTimeHost(t, dir, facts, scheduler, runner)
}

func startSelectedTimeHost(t *testing.T, dir string, facts platformclock.Source, scheduler platformclock.TimerSource, runner platformprocess.CommandRunner) selectedTimeFixture {
	t.Helper()
	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock: facts, ProcessScheduler: scheduler, ProviderCommandRunner: runner, APIServerStarter: api.Start,
	})
	session := uuid.NewString()
	metrics := filepath.Join(t.TempDir(), "metrics")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--session", session, "--continuously", "--with-server", "--quiet", "--no-record", "--runtime-metrics-dir", metrics,
	})
	// All profile/cache roots are scenario-owned, including Windows locations.
	home := t.TempDir()
	inputs.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"), "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	inputs.WorkingDirectory = dir
	// First-run profile installation is a prerequisite, outside host readiness.
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Env, dir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	var readiness *controlledReadinessTimers
	if scheduler != nil {
		readiness = scheduler.(*selectedTimeScheduler).readiness
	} else if source, ok := facts.(*selectedTimeSource); ok {
		readiness = source.readiness
	}
	if readiness == nil {
		return selectedTimeFixture{url: api.WaitForURL(t), session: session, metrics: metrics, command: command}
	}
	return selectedTimeFixture{url: readiness.WaitForURL(t, api), session: session, metrics: metrics, command: command}
}

func awaitSelectedTimeSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("selected time effect barrier not reached")
	}
}

func assertSelectedRuntimeEventTime(t *testing.T, fixture selectedTimeFixture, kind factoryapi.FactoryEventType, want time.Time) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, fixture.url, fixture.session)
	for _, event := range events {
		if event.Type == kind {
			if !event.Context.EventTime.Equal(want) {
				t.Fatalf("%s time = %v, want %v", kind, event.Context.EventTime, want)
			}
			return
		}
	}
	t.Fatalf("public Factory Events lack %s", kind)
}

func assertSelectedTimeWork(t *testing.T, fixture selectedTimeFixture) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(fixture.url, fixture.session, "/work"))
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed Work = %d, want 1: %#v", got, listed.Results)
	}
	events := support.GetFactoryEventsForSessionAt(t, fixture.url, fixture.session)
	assertSelectedEventOrder(t, events)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil || payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.Output == nil || !strings.Contains(*payload.Output, "COMPLETE") {
			t.Fatalf("terminal dispatch = %#v, error %v", payload, err)
		}
		return
	}
	t.Fatal("accepted dispatch response missing")
}

func assertSelectedMemorySamples(t *testing.T, root string, want int) {
	t.Helper()
	count := 0
	for _, path := range functionalMetricArtifactPaths(t, root) {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("read metrics sample: %v", err)
			}
			if record["metric_name"] == "runtime.memory.heap_alloc" {
				count++
			}
		}
	}
	if count != want {
		t.Fatalf("runtime.memory.heap_alloc samples = %d, want %d", count, want)
	}
}

func assertSelectedEventOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	expected := []factoryapi.FactoryEventType{factoryapi.FactoryEventTypeWorkRequest, factoryapi.FactoryEventTypeDispatchRequest, factoryapi.FactoryEventTypeDispatchResponse}
	next := 0
	for _, event := range events {
		if event.Type == expected[next] {
			next++
		}
		if next == len(expected) {
			return
		}
	}
	t.Fatalf("Factory Event history lacks ordered admission/dispatch/completion: next %s", expected[next])
}

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
