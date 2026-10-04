package agy_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestCommandEffectBuildsRecordedPrintArgv(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("recorded stream output"),
	})
	effect := newAgyCommandEffect(runner)
	workspace := t.TempDir()
	prompt := "Watch clip-fixture.mp4; preserve this path and the semicolon."
	var observed []byte

	result, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "agy-print-dispatch",
			Model:            "gemini-3.6-flash-high",
			ReasoningEffort:  " HIGH ",
			SkipPermissions:  true,
			PrintTimeout:     8 * time.Minute,
			UserMessage:      prompt,
			WorkingDirectory: workspace,
		},
	}, func(chunk []byte) error {
		observed = append(observed, chunk...)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(observed) != "recorded stream output" {
		t.Fatalf("observed output = %q, want recorded output", observed)
	}
	if result.DurationMillis < 0 {
		t.Fatalf("DurationMillis = %d, want non-negative", result.DurationMillis)
	}

	request := runner.LastRequest()
	if request.Command != "agy" {
		t.Fatalf("command = %q, want agy", request.Command)
	}
	wantArgs := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--add-dir", workspace,
		"--disable-slash-commands",
		"--model", "gemini-3.6-flash-high",
		"--effort", "high",
		"--dangerously-skip-permissions",
		"--print-timeout", "8m",
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("argv = %#v, want %#v", request.Args, wantArgs)
	}
	if request.WorkDir != workspace {
		t.Fatalf("work dir = %q, want %q", request.WorkDir, workspace)
	}
}

func TestCommandEffectSelectsJSONModeForStructuredOutput(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(`{"conversation_id":"structured-command","status":"SUCCESS","response":"ok","structured_output":{"ok":true},"json_schema":{"type":"object"},"duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}`),
	})
	effect := newAgyCommandEffect(runner)
	workspace := t.TempDir()
	schema := `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "agy-structured-dispatch",
			Model:            "gemini-3.6-flash-low",
			OutputSchema:     schema,
			WorkingDirectory: workspace,
			UserMessage:      "return a structured result",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantArgs := []string{
		"-p", "return a structured result",
		"--output-format", "json",
		"--add-dir", workspace,
		"--disable-slash-commands",
		"--json-schema", schema,
		"--model", "gemini-3.6-flash-low",
		"--print-timeout", "5m",
	}
	request := runner.LastRequest()
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("argv = %#v, want %#v", request.Args, wantArgs)
	}
}

func TestCommandEffectBuildsArgvForRecordedFileAndVideoTraces(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	workspace := filepath.Join(repoRoot, "tests", "functional", "providers", "agy", "testdata")
	tests := []struct {
		name       string
		trace      string
		model      string
		prompt     string
		printLimit time.Duration
		printArg   string
	}{
		{
			name:       "file read",
			trace:      "agy-trace-file-read.stream.jsonl",
			model:      "gemini-3.6-flash-low",
			prompt:     "Read the file fixture-note.txt in the workspace and report the values of alpha and beta.",
			printLimit: 5 * time.Minute,
			printArg:   "5m",
		},
		{
			name:       "video watch",
			trace:      "agy-trace-video-watch.stream.jsonl",
			model:      "gemini-3.6-flash-high",
			prompt:     "Watch the video file clip-fixture.mp4 in the workspace. Describe the visual content and state whether the audio track contains speech, music, noise, or silence.",
			printLimit: 8 * time.Minute,
			printArg:   "8m",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			trace, err := os.ReadFile(filepath.Join(workspace, test.trace))
			if err != nil {
				t.Fatalf("read recorded trace %q: %v", test.trace, err)
			}
			if !bytes.Contains(trace, []byte(`"event":"result"`)) {
				t.Fatalf("recorded trace %q has no terminal result event", test.trace)
			}

			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: trace})
			effect := newAgyCommandEffect(runner)
			_, err = effect.Execute(context.Background(), execution.ContinuationRequest{
				ExecuteRequest: providers.ExecuteRequest{
					Provider:         providers.IDAntigravity,
					AttemptID:        "agy-recorded-" + strings.ReplaceAll(test.name, " ", "-"),
					Model:            test.model,
					SkipPermissions:  true,
					PrintTimeout:     test.printLimit,
					UserMessage:      test.prompt,
					WorkingDirectory: workspace,
				},
			}, func([]byte) error { return nil })
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			request := runner.LastRequest()
			wantArgs := []string{
				"-p", test.prompt,
				"--output-format", "stream-json",
				"--add-dir", workspace,
				"--disable-slash-commands",
				"--model", test.model,
				"--dangerously-skip-permissions",
				"--print-timeout", test.printArg,
			}
			if request.Command != "agy" || !reflect.DeepEqual(request.Args, wantArgs) {
				t.Fatalf("command request = %#v, want agy with argv %#v", request, wantArgs)
			}
			if request.WorkDir != workspace {
				t.Fatalf("work directory = %q, want %q", request.WorkDir, workspace)
			}
		})
	}
}

func TestCommandEffectUsesFiveMinutePrintTimeoutByDefault(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("ok")})
	effect := newAgyCommandEffect(runner)
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "agy-print-default-timeout",
			Model:            "gemini-3.6-flash-low",
			WorkingDirectory: t.TempDir(),
			UserMessage:      "Read fixture-note.txt",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	request := runner.LastRequest()
	if got := argumentValue(request.Args, "--print-timeout"); got != "5m" {
		t.Fatalf("--print-timeout = %q, want 5m", got)
	}
	if !containsArgument(request.Args, "--add-dir") {
		t.Fatalf("argv = %#v, want mandatory --add-dir", request.Args)
	}
}

func TestCommandEffectPreservesTypedSeparateEffortRejection(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("Agy does not support a separate reasoning effort"),
		ExitCode: 1,
	})
	effect := newAgyCommandEffect(runner)
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "agy-typed-rejection",
			Model:            "gemini-3.6-flash-medium",
			WorkingDirectory: t.TempDir(),
			UserMessage:      "force a provider rejection",
		},
	}, func([]byte) error { return nil })
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Execute() error = %#v, want typed invalid-request failure", err)
	}
	if failure.Message != "Agy does not support a separate reasoning effort." {
		t.Fatalf("failure message = %q, want safe provider rejection", failure.Message)
	}
}

func TestCommandEffectRejectsUnsupportedModelAndEffortBeforeLaunch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		model   string
		effort  string
		wantErr string
	}{
		{name: "model", model: "gemini-pro", wantErr: "unsupported model"},
		{name: "effort", model: "gemini-3.6-flash-low", effort: "xhigh", wantErr: "unsupported reasoning effort"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := testutil.NewProviderCommandRunner()
			effect := newAgyCommandEffect(runner)
			_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
				ExecuteRequest: providers.ExecuteRequest{
					Provider:         providers.IDAntigravity,
					AttemptID:        "agy-invalid-" + test.name,
					Model:            test.model,
					ReasoningEffort:  test.effort,
					WorkingDirectory: t.TempDir(),
				},
			}, func([]byte) error { return nil })
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, test.wantErr)
			}
			if runner.CallCount() != 0 {
				t.Fatalf("runner calls = %d, want zero before validation", runner.CallCount())
			}
		})
	}
}

func newAgyCommandEffect(runner platformprocess.CommandRunner) agy.Effect {
	return agy.NewCommandEffect(
		executionwire.AdaptPlatformCommandRunner(runner),
		platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Millisecond),
		platformclock.Real{},
	)
}

func argumentValue(args []string, flag string) string {
	for index, arg := range args[:max(0, len(args)-1)] {
		if arg == flag {
			return args[index+1]
		}
	}
	return ""
}

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func TestCommandEffectDurationUsesInjectedClockOnSuccessAndFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		runErr   error
		wantKind providers.ExecuteFailureKind
	}{
		{name: "success"},
		{name: "timeout", runErr: context.DeadlineExceeded, wantKind: providers.ExecuteFailureKindTimeout},
		{name: "cancellation", runErr: context.Canceled, wantKind: providers.ExecuteFailureKindCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Millisecond)
			calls := 0
			effect := agy.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(clockAdvancingCommandRunner{
				run: func(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
					calls++
					if command.Command != "agy" || command.ExecutionScopeID != "owned-session" {
						t.Fatalf("command = %#v, want agy in owned session", command)
					}
					clock.SetTick(43)
					return platformprocess.CommandResult{Stdout: []byte("delivered"), Stderr: []byte("private diagnostic")}, tc.runErr
				},
			}), clock, clock)
			var observed strings.Builder
			result, err := effect.Execute(t.Context(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
				Provider:    providers.IDAntigravity,
				UserMessage: "perform work",
				Correlation: providers.ExecuteCorrelation{FactorySessionID: "owned-session"},
			}}, func(chunk []byte) error {
				observed.Write(chunk)
				return nil
			})
			if calls != 1 || result.DurationMillis != 43 || observed.String() != "delivered" || string(result.CapturedStdout) != "delivered" {
				t.Fatalf("calls = %d, result = %#v, stdout = %q", calls, result, observed.String())
			}
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
			} else {
				var failure providers.ExecuteFailure
				if !errors.As(err, &failure) || failure.Kind != tc.wantKind {
					t.Fatalf("error = %v, want %s", err, tc.wantKind)
				}
			}
		})
	}
}

type clockAdvancingCommandRunner struct {
	run func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)
}

func TestCommandEffectScheduledTimeoutAndCleanup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		timeout      time.Duration
		outcome      string
		wantKind     providers.ExecuteFailureKind
		wantSentinel error
	}{
		{name: "timeout", timeout: 10 * time.Millisecond, outcome: "timeout", wantKind: providers.ExecuteFailureKindTimeout, wantSentinel: providers.ErrExecuteTimeout},
		{name: "cancel", timeout: time.Hour, outcome: "cancel", wantKind: providers.ExecuteFailureKindCanceled, wantSentinel: providers.ErrExecuteCancelled},
		{name: "caller cancellation with deadline cause", outcome: "cancel with cause", wantKind: providers.ExecuteFailureKindCanceled, wantSentinel: providers.ErrExecuteCancelled},
		{name: "success before default timeout", outcome: "success"},
		{name: "failure before timeout", timeout: time.Hour, outcome: "failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scheduler := &observedCommandScheduler{Deterministic: platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Millisecond)}
			parent, cancel := context.WithCancelCause(t.Context())
			defer cancel(context.Canceled)
			commandErr := errors.New("command failed")
			runner := clockAdvancingCommandRunner{run: func(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
				return scheduledCommandOutcome(t, ctx, scheduler, cancel, tc.outcome, commandErr)
			}}
			effect := agy.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(runner), scheduler, scheduler)
			result, err := effect.Execute(parent, execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
				Provider: providers.IDAntigravity, PrintTimeout: tc.timeout,
			}}, func([]byte) error { return nil })
			assertCommandTimerStopped(t, scheduler, tc.timeout)
			if tc.wantKind != "" {
				var failure providers.ExecuteFailure
				if !errors.As(err, &failure) || failure.Kind != tc.wantKind || !errors.Is(err, tc.wantSentinel) {
					t.Fatalf("error = %v, want %s and %v", err, tc.wantKind, tc.wantSentinel)
				}
				if tc.outcome == "timeout" && result.DurationMillis != 10 {
					t.Fatalf("duration = %d, want injected 10ms", result.DurationMillis)
				}
			} else if tc.outcome == "failure" {
				if !errors.Is(err, commandErr) {
					t.Fatalf("error = %v, want original command error", err)
				}
			} else if err != nil || string(result.CapturedStdout) != "done" {
				t.Fatalf("result = %+v, error = %v, want completed output", result, err)
			}
		})
	}
}

type observedCommandScheduler struct {
	*platformclock.Deterministic
	duration time.Duration
	timer    *observedCommandTimer
}

func (clock *observedCommandScheduler) NewTimer(duration time.Duration) platformclock.Timer {
	clock.duration = duration
	clock.timer = &observedCommandTimer{Timer: clock.Deterministic.NewTimer(duration)}
	return clock.timer
}

type observedCommandTimer struct {
	platformclock.Timer
	stopped bool
}

func (timer *observedCommandTimer) Stop() bool {
	timer.stopped = true
	return timer.Timer.Stop()
}

func (runner clockAdvancingCommandRunner) Run(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, command)
}

func scheduledCommandOutcome(t *testing.T, ctx context.Context, scheduler *observedCommandScheduler, cancel context.CancelCauseFunc, outcome string, commandErr error) (platformprocess.CommandResult, error) {
	t.Helper()
	switch outcome {
	case "timeout":
		scheduler.SetTick(9)
		select {
		case <-ctx.Done():
			t.Fatal("command expired before its injected timeout")
		default:
		}
		scheduler.SetTick(10)
		<-ctx.Done()
		if ctx.Err() != context.DeadlineExceeded || !errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			t.Fatalf("runner context error = %v, cause = %v", ctx.Err(), context.Cause(ctx))
		}
	case "cancel", "cancel with cause":
		cause := error(context.Canceled)
		if outcome == "cancel with cause" {
			cause = context.DeadlineExceeded
		}
		cancel(cause)
		<-ctx.Done()
		if ctx.Err() != context.Canceled {
			t.Fatalf("runner context error = %v, want caller cancellation", ctx.Err())
		}
	case "failure":
		return platformprocess.CommandResult{}, commandErr
	default:
		return platformprocess.CommandResult{Stdout: []byte("done")}, nil
	}
	return platformprocess.CommandResult{}, ctx.Err()
}

func assertCommandTimerStopped(t *testing.T, scheduler *observedCommandScheduler, timeout time.Duration) {
	t.Helper()
	expectedTimeout := timeout
	if expectedTimeout == 0 {
		expectedTimeout = providers.DefaultAntigravityPrintTimeout
	}
	if scheduler.duration != expectedTimeout || scheduler.timer == nil || !scheduler.timer.stopped {
		t.Fatalf("timer duration = %v, timer = %+v, want %v and stopped", scheduler.duration, scheduler.timer, expectedTimeout)
	}
}
