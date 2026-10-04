package wire

import (
	"context"
	"io"
	"reflect"
	"strings"
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

	got, err := provideAgyPTYAllocator(serviceedges.Edges{
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	})
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
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{
		AgyPTYHost:  host,
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	})
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
	service, err := provideProvidersService(serviceedges.Edges{
		ProviderCommandRunner: runner,
		AgyPTYHost:            host,
	})
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

func (clock *recordingPTYNow) Now() time.Time { clock.calls.Add(1); return time.Unix(0, 0) }
func TestProvideAgyPTYAllocatorUsesProcessSchedulerWithNowOnlyOverride(t *testing.T) {
	t.Parallel()
	timer := &watchWaitTimer{ticks: make(chan time.Time)}
	scheduler := &watchWaitScheduler{timer: timer}
	override := &recordingPTYNow{}
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{Clock: scheduler, AgyPTYClock: override, AgyPTYHost: &completedPTYHost{}})
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
