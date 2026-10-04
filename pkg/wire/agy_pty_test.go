package wire

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

type recordingAgyPTYHost struct{ allocated bool }
type recordingAgyPTY struct{}

func (*recordingAgyPTY) Close() error           { return nil }
func (*recordingAgyPTY) Kind() platformpty.Kind { return platformpty.KindPOSIX }
func (h *recordingAgyPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	h.allocated = true
	return &recordingAgyPTY{}, nil
}
func (*recordingAgyPTYHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, nil
}

func TestEdgesDoNotExposeComposedAgyPTYAllocator(t *testing.T) {
	t.Parallel()

	if _, exposed := reflect.TypeOf(serviceedges.Edges{}).FieldByName("AgyPTYAllocator"); exposed {
		t.Fatal("Edges exposes composed AgyPTYAllocator; only Host and Clock effects may be replaced")
	}
}

func TestProvideAgyPTYAllocatorSelectsInertPlatformAdapter(t *testing.T) {
	t.Parallel()

	got, err := provideAgyPTYAllocator(selectedTestTimeEdges(serviceedges.Edges{
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	}))
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	if got == nil {
		t.Fatal("provideAgyPTYAllocator() = nil, want inert platform adapter")
	}
}

func TestProvideAgyPTYAllocatorPreservesInjectedNativeHost(t *testing.T) {
	t.Parallel()

	host := &recordingAgyPTYHost{}
	allocator, err := provideAgyPTYAllocator(selectedTestTimeEdges(serviceedges.Edges{
		AgyPTYHost:  host,
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	}))
	if err != nil {
		t.Fatal(err)
	}
	session, err := allocator.Allocate(context.Background(), providerswire.PTYProcessLaunch{
		Executable: "agy", Argv: []string{"agy"},
	}, providerswire.DefaultPTYSessionConfig())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	defer session.Close()
	if !host.allocated {
		t.Fatal("injected native host was not used")
	}
}

func TestProvideProvidersServicePrefersAgyCommandRunnerWithInjectedPTYHost(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(`{"event":"result","result":{"conversation_id":"wire-agy-selection","status":"SUCCESS","response":"WIRE_OK","duration_seconds":1,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
	})
	host := &recordingAgyPTYHost{}
	workDir := t.TempDir()
	service, err := provideProvidersService(selectedTestTimeEdges(serviceedges.Edges{
		ProviderCommandRunner: runner,
		AgyPTYHost:            host,
	}))
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAntigravity,
		AttemptID:        "agy-command-selection",
		Model:            "gemini-3.6-flash-high",
		SkipPermissions:  true,
		WorkingDirectory: workDir,
		UserMessage:      "wire selection",
	})
	if err != nil {
		t.Fatalf("Execute(antigravity) error = %v", err)
	}
	if result.Content != "WIRE_OK" {
		t.Fatalf("Execute(antigravity) content = %q, want WIRE_OK", result.Content)
	}
	if host.allocated {
		t.Fatal("injected Agy PTY host was selected; command runner must own canonical print dispatch")
	}

	wantArgs := []string{
		"-p", "wire selection",
		"--output-format", "stream-json",
		"--add-dir", workDir,
		"--disable-slash-commands",
		"--model", "gemini-3.6-flash-high",
		"--dangerously-skip-permissions",
		"--print-timeout", "5m",
	}
	request := runner.LastRequest()
	if request.Command != "agy" {
		t.Fatalf("provider command = %q, want agy", request.Command)
	}
	if request.WorkDir != workDir {
		t.Fatalf("provider workdir = %q, want %q", request.WorkDir, workDir)
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("provider argv = %#v, want %#v", request.Args, wantArgs)
	}
}

type completedPTYProcess struct{}

func (completedPTYProcess) Wait() error      { return nil }
func (completedPTYProcess) Terminate() error { return nil }
func (completedPTYProcess) Close()           {}
func (completedPTYProcess) PID() int         { return 0 }
func (completedPTYProcess) ExitCode() int    { return 0 }

type completedPTYHost struct{ recordingAgyPTYHost }

func (*completedPTYHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return completedPTYProcess{}, io.NopCloser(strings.NewReader("PTY_OK")), nil
}

type recordingPTYNow struct{ calls atomic.Int32 }

func TestProviderClockSelectionKeepsDurationAndSchedulingCapabilitiesSeparate(t *testing.T) {
	t.Parallel()
	override := &recordingPTYNow{}
	nowOnly := serviceedges.Edges{Clock: override}
	if effectiveProviderCommandClock(nowOnly) != override || override.calls.Load() != 0 {
		t.Fatal("duration clock must preserve the inert Now-only override")
	}
	if _, ok := effectiveProviderScheduler(nowOnly).(platformclock.Real); !ok {
		t.Fatal("Now-only override must retain the canonical host scheduler")
	}
	scheduler := &watchWaitScheduler{timer: &watchWaitTimer{ticks: make(chan time.Time)}}
	if effectiveProviderScheduler(serviceedges.Edges{Clock: scheduler}) != scheduler || scheduler.calls != 0 {
		t.Fatal("timer-capable override must retain the same inert scheduler")
	}
}

func (clock *recordingPTYNow) Now() time.Time { clock.calls.Add(1); return time.Unix(0, 0) }
func TestProvideAgyPTYAllocatorUsesProcessSchedulerWithNowOnlyOverride(t *testing.T) {
	t.Parallel()
	timer := &watchWaitTimer{ticks: make(chan time.Time)}
	scheduler := &watchWaitScheduler{timer: timer}
	override := &recordingPTYNow{}
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{Clock: scheduler, ProcessScheduler: scheduler, AgyPTYClock: override, AgyPTYHost: &completedPTYHost{}})
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.calls != 0 || override.calls.Load() != 0 {
		t.Fatal("construction must not start time effects")
	}
	session, err := allocator.Allocate(context.Background(), providerswire.PTYProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, providerswire.DefaultPTYSessionConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.Run(context.Background())
	if err != nil || result.CleanedText != "PTY_OK" || result.TimedOut {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if override.calls.Load() == 0 || scheduler.calls != 2 || scheduler.delay != 250*time.Millisecond || !timer.stopped {
		t.Fatalf("effects: Now calls %d; timer calls %d; drain %v; stopped %v", override.calls.Load(), scheduler.calls, scheduler.delay, timer.stopped)
	}
}

// These fixtures observe the returned allocator through its native effect
// contracts. No application graph or OS process is involved.
type selectionPTYClock struct {
	platformclock.TimerSource
	created  chan *selectionPTYTimer
	nowCalls atomic.Int32
}

type selectionPTYTimer struct {
	platformclock.Timer
	delay   time.Duration
	stopped atomic.Bool
}

func newSelectionPTYClock(at int64) *selectionPTYClock {
	return &selectionPTYClock{
		TimerSource: platformclock.NewDeterministic(time.Unix(at, 0), time.Millisecond),
		created:     make(chan *selectionPTYTimer, 8),
	}
}

func (c *selectionPTYClock) Now() time.Time {
	c.nowCalls.Add(1)
	return c.TimerSource.Now()
}

func (c *selectionPTYClock) NewTimer(delay time.Duration) platformclock.Timer {
	timer := &selectionPTYTimer{Timer: c.TimerSource.NewTimer(delay), delay: delay}
	c.created <- timer
	return timer
}

func (c *selectionPTYClock) advance(tick int) {
	c.TimerSource.(*platformclock.Deterministic).SetTick(tick)
}

func (timer *selectionPTYTimer) Stop() bool {
	timer.stopped.Store(true)
	return timer.Timer.Stop()
}

type selectionPTYNow struct{ source platformclock.Source }

func (c selectionPTYNow) Now() time.Time { return c.source.Now() }

type selectionPTYReader struct {
	deadlines chan time.Time
	closed    chan struct{}
	once      sync.Once
	reads     int
}

func (r *selectionPTYReader) SetReadDeadline(at time.Time) error {
	r.deadlines <- at
	return nil
}

func (r *selectionPTYReader) Read(buf []byte) (int, error) {
	r.reads++
	if r.reads == 1 {
		return copy(buf, "\x1b[32mPTY_OK\x1b[0m"), nil
	}
	<-r.closed
	return 0, io.EOF
}

func (r *selectionPTYReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

type selectionPTYProcess struct {
	completedPTYProcess
	done       chan struct{}
	once       sync.Once
	terminated atomic.Bool
	closed     atomic.Bool
	waitErr    error
	exitCode   int
}

func (p *selectionPTYProcess) Wait() error { <-p.done; return p.waitErr }
func (p *selectionPTYProcess) finish()     { p.once.Do(func() { close(p.done) }) }
func (p *selectionPTYProcess) Terminate() error {
	p.terminated.Store(true)
	p.finish()
	return nil
}
func (p *selectionPTYProcess) Close()        { p.closed.Store(true) }
func (p *selectionPTYProcess) ExitCode() int { return p.exitCode }

type selectionPTYHost struct {
	reader      *selectionPTYReader
	proc        *selectionPTYProcess
	allocated   atomic.Int32
	closed      atomic.Int32
	launch      platformpty.ProcessLaunch
	allocateErr error
	startErr    error
}

type selectionPTYAllocation struct{ host *selectionPTYHost }

func (a selectionPTYAllocation) Kind() platformpty.Kind { return platformpty.KindPOSIX }
func (a selectionPTYAllocation) Close() error           { a.host.closed.Add(1); return nil }
func (h *selectionPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	h.allocated.Add(1)
	if h.allocateErr != nil {
		return nil, h.allocateErr
	}
	return selectionPTYAllocation{host: h}, nil
}
func (h *selectionPTYHost) Start(launch platformpty.ProcessLaunch, _ platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	h.launch = launch
	return h.proc, h.reader, h.startErr
}

type selectionPTYTerminal struct {
	result providerswire.PTYSessionResult
	err    error
}

func awaitSelectionPTY[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(30 * time.Second):
		t.Fatal("PTY effect did not reach its observed gate")
		var zero T
		return zero
	}
}

func startSelectionPTY(t *testing.T, edges serviceedges.Edges) (*selectionPTYHost, context.CancelFunc, <-chan selectionPTYTerminal) {
	t.Helper()
	host := &selectionPTYHost{
		reader: &selectionPTYReader{deadlines: make(chan time.Time, 8), closed: make(chan struct{})},
		proc:   &selectionPTYProcess{done: make(chan struct{})},
	}
	edges.AgyPTYHost = host
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatal(err)
	}
	if host.allocated.Load() != 0 {
		t.Fatal("construction allocated a PTY")
	}
	for _, source := range []platformclock.Source{edges.Clock, edges.ProcessScheduler, edges.AgyPTYClock} {
		if nowOnly, ok := source.(selectionPTYNow); ok {
			source = nowOnly.source
		}
		if clock, ok := source.(*selectionPTYClock); ok && (clock.nowCalls.Load() != 0 || len(clock.created) != 0) {
			t.Fatal("construction caused clock or timer effects")
		}
	}
	session, err := allocator.Allocate(context.Background(), providerswire.PTYProcessLaunch{
		Executable: "agy", Argv: []string{"agy", "--print", "selected time"},
	}, providerswire.DefaultPTYSessionConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan selectionPTYTerminal, 1)
	joined := make(chan struct{})
	go func() {
		result, err := session.Run(ctx)
		done <- selectionPTYTerminal{result, err}
		close(joined)
	}()
	t.Cleanup(func() {
		cancel()
		_ = host.reader.Close()
		host.proc.finish()
		awaitSelectionPTY(t, joined)
		_ = session.Close()
	})
	return host, cancel, done
}

func assertSelectionPTYCleanup(t *testing.T, host *selectionPTYHost, timers ...*selectionPTYTimer) {
	t.Helper()
	if host.closed.Load() != 1 || !host.proc.closed.Load() {
		t.Fatalf("owned resource cleanup: allocation=%d process=%v", host.closed.Load(), host.proc.closed.Load())
	}
	select {
	case <-host.reader.closed:
	default:
		t.Fatal("capture reader remains open")
	}
	for _, timer := range timers {
		if !timer.stopped.Load() {
			t.Fatal("owned timer remains unstopped")
		}
	}
}

func assertSelectionPTYPending(t *testing.T, host *selectionPTYHost, done <-chan selectionPTYTerminal, timer *selectionPTYTimer) {
	t.Helper()
	select {
	case got := <-done:
		t.Fatalf("Run completed before selected wait: %#v", got)
	case <-host.reader.closed:
		t.Fatal("reader closed before selected wait")
	case <-timer.C():
		t.Fatal("timer fired before selected boundary")
	default:
	}
}

func TestAgyPTYTimeSelectionDrain(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"same", "distinct", "full-specialized", "now-specialized", "legacy-explicit"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wall, scheduler, specialized := newSelectionPTYClock(100), newSelectionPTYClock(200), newSelectionPTYClock(300)
			edges := serviceedges.Edges{Clock: wall, ProcessScheduler: scheduler}
			facts, waits := wall, scheduler
			switch name {
			case "same":
				edges.ProcessScheduler, waits = wall, wall
			case "full-specialized":
				edges.AgyPTYClock, facts, waits = specialized, specialized, specialized
			case "now-specialized":
				edges.AgyPTYClock, facts = selectionPTYNow{specialized}, specialized
			case "legacy-explicit":
				edges.Clock = selectionPTYNow{wall}
			}
			host, _, done := startSelectionPTY(t, edges)
			assertSelectionPTYDrain(t, host, done, facts, waits, wall, scheduler, specialized)
		})
	}
}

func assertSelectionPTYDrain(t *testing.T, host *selectionPTYHost, done <-chan selectionPTYTerminal, facts, waits *selectionPTYClock, clocks ...*selectionPTYClock) {
	t.Helper()
	first := awaitSelectionPTY(t, waits.created)
	if first.delay != 100*time.Millisecond {
		t.Fatalf("supervision cadence = %v", first.delay)
	}
	for range 2 {
		at := awaitSelectionPTY(t, host.reader.deadlines)
		if want := facts.TimerSource.Now().Add(100 * time.Millisecond); !at.Equal(want) {
			t.Fatalf("capture deadline = %v, want selected facts %v", at, want)
		}
	}
	host.proc.finish()
	drain := awaitSelectionPTY(t, waits.created)
	if drain.delay != 250*time.Millisecond || !first.stopped.Load() {
		t.Fatalf("drain bound = %v, supervision stopped = %v", drain.delay, first.stopped.Load())
	}
	for _, c := range clocks {
		if c == waits {
			continue
		}
		c.advance(1000)
		if len(c.created) != 0 {
			t.Fatal("unselected source created a PTY timer")
		}
		assertSelectionPTYPending(t, host, done, drain)
	}
	waits.advance(249)
	assertSelectionPTYPending(t, host, done, drain)
	factAt := facts.TimerSource.Now()
	waits.advance(250)
	got := awaitSelectionPTY(t, done)
	assertSelectionPTYSuccess(t, got)
	if facts != waits && !facts.TimerSource.Now().Equal(factAt) {
		t.Fatal("scheduler advance changed duration facts")
	}
	if !reflect.DeepEqual(host.launch.Argv, []string{"agy", "--print", "selected time"}) {
		t.Fatalf("launch argv = %v", host.launch.Argv)
	}
	assertSelectionPTYCleanup(t, host, first, drain)
}

func assertSelectionPTYSuccess(t *testing.T, got selectionPTYTerminal) {
	t.Helper()
	if got.err != nil || got.result.CleanedText != "PTY_OK" || got.result.TimedOut || got.result.ExitCode != 0 {
		t.Fatalf("Run = %#v, %v", got.result, got.err)
	}
}

func TestAgyPTYTimeSelectionCancellationKeepsPeerPending(t *testing.T) {
	t.Parallel()
	wallA, wallB := newSelectionPTYClock(100), newSelectionPTYClock(200)
	waitsA, waitsB := newSelectionPTYClock(300), newSelectionPTYClock(400)
	a, cancel, doneA := startSelectionPTY(t, serviceedges.Edges{Clock: wallA, ProcessScheduler: waitsA})
	b, _, doneB := startSelectionPTY(t, serviceedges.Edges{Clock: wallB, ProcessScheduler: waitsB})
	firstA, firstB := awaitSelectionPTY(t, waitsA.created), awaitSelectionPTY(t, waitsB.created)
	for range 2 {
		awaitSelectionPTY(t, a.reader.deadlines)
		awaitSelectionPTY(t, b.reader.deadlines)
	}
	cancel()
	drainA := awaitSelectionPTY(t, waitsA.created)
	if !a.proc.terminated.Load() || drainA.delay != 250*time.Millisecond {
		t.Fatal("cancellation did not terminate and drain")
	}
	waitsA.advance(250)
	gotA := awaitSelectionPTY(t, doneA)
	if !errors.Is(gotA.err, context.Canceled) || gotA.result.TimedOut {
		t.Fatalf("cancel = %#v, %v", gotA.result, gotA.err)
	}
	assertSelectionPTYCleanup(t, a, firstA, drainA)
	assertSelectionPTYPending(t, b, doneB, firstB)
	if b.proc.terminated.Load() || b.closed.Load() != 0 {
		t.Fatal("cancellation crossed into peer")
	}
	b.proc.finish()
	drainB := awaitSelectionPTY(t, waitsB.created)
	waitsB.advance(250)
	gotB := awaitSelectionPTY(t, doneB)
	if gotB.err != nil || gotB.result.CleanedText != "PTY_OK" || gotB.result.TimedOut {
		t.Fatalf("peer = %#v, %v", gotB.result, gotB.err)
	}
	assertSelectionPTYCleanup(t, b, firstB, drainB)
}

func TestAgyPTYTimeSelectionLegacyRealScheduler(t *testing.T) {
	t.Parallel()
	// Root owns omission normalization. Observe consumption of its documented
	// Real selection without making a real-time latency assertion.
	waits := &selectionPTYClock{TimerSource: platformclock.Real{}, created: make(chan *selectionPTYTimer, 8)}
	facts := &recordingPTYNow{}
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{Clock: facts, ProcessScheduler: waits, AgyPTYHost: &completedPTYHost{}})
	if err != nil {
		t.Fatal(err)
	}
	if facts.calls.Load() != 0 || len(waits.created) != 0 {
		t.Fatal("construction caused time effects")
	}
	session, err := allocator.Allocate(context.Background(), providerswire.PTYProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, providerswire.DefaultPTYSessionConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.Run(context.Background())
	if err != nil || result.CleanedText != "PTY_OK" || facts.calls.Load() == 0 {
		t.Fatalf("legacy Run = %#v, %v", result, err)
	}
	first, drain := awaitSelectionPTY(t, waits.created), awaitSelectionPTY(t, waits.created)
	if first.delay != 100*time.Millisecond || drain.delay != 250*time.Millisecond || !first.stopped.Load() || !drain.stopped.Load() {
		t.Fatal("normalized Real scheduler was not consumed and stopped")
	}
}

func TestAgyPTYTimeSelectionFailuresDisposeOwnedResources(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"allocate", "start", "wait", "nonzero"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			failure := errors.New("controlled PTY failure")
			wall, waits := newSelectionPTYClock(100), newSelectionPTYClock(200)
			host := &selectionPTYHost{proc: &selectionPTYProcess{done: make(chan struct{})}}
			switch name {
			case "allocate":
				host.allocateErr = failure
			case "start":
				host.startErr = failure
			case "wait":
				host.proc.waitErr = failure
			}
			allocator, err := provideAgyPTYAllocator(serviceedges.Edges{Clock: wall, ProcessScheduler: waits, AgyPTYHost: host})
			if err != nil {
				t.Fatal(err)
			}
			session, err := allocator.Allocate(context.Background(), providerswire.PTYProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, providerswire.DefaultPTYSessionConfig())
			if name == "allocate" {
				assertSelectionPTYAllocationFailure(t, session, err, host, waits)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			if name == "start" {
				_, err := session.Run(context.Background())
				if !errors.Is(err, failure) || host.closed.Load() != 1 || len(waits.created) != 0 {
					t.Fatalf("start = %v, closes=%d", err, host.closed.Load())
				}
				return
			}
			assertSelectionPTYExitFailure(t, session, host, wall, waits, name)
		})
	}
}

func assertSelectionPTYAllocationFailure(t *testing.T, session providerswire.PTYSession, err error, host *selectionPTYHost, waits *selectionPTYClock) {
	t.Helper()
	// The owner wraps its allocation sentinel and retains the native cause as
	// text; it deliberately does not wrap the native error.
	if err == nil || err.Error() != "agypty: PTY allocation failed: controlled PTY failure" || session != nil || host.closed.Load() != 0 {
		t.Fatalf("allocation = %v, %v", session, err)
	}
	if len(waits.created) != 0 {
		t.Fatal("failed allocation created a timer")
	}
}

func assertSelectionPTYExitFailure(t *testing.T, session providerswire.PTYSession, host *selectionPTYHost, wall, waits *selectionPTYClock, name string) {
	t.Helper()
	// An EOF reader makes drain complete without advancing a scheduler timer.
	// Preserve the existing wait-error result classification (ExitCode -1).
	host.proc.finish()
	if name == "nonzero" {
		host.proc.exitCode = 7
	}
	// Both failures are expressed at the native process boundary.
	host.reader = &selectionPTYReader{deadlines: make(chan time.Time, 8), closed: make(chan struct{})}
	_ = host.reader.Close()
	result, err := session.Run(context.Background())
	wantExit := -1
	if name == "nonzero" {
		wantExit = 7
	}
	if err != nil || result.ExitCode != wantExit || result.TimedOut || result.CleanedText != "PTY_OK" {
		t.Fatalf("%s result = %#v, %v", name, result, err)
	}
	first, drain := awaitSelectionPTY(t, waits.created), awaitSelectionPTY(t, waits.created)
	if first.delay != 100*time.Millisecond || drain.delay != 250*time.Millisecond || len(wall.created) != 0 {
		t.Fatal("failure changed timer selection or durations")
	}
	assertSelectionPTYCleanup(t, host, first, drain)
}
