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
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectBuildsRecordedPrintArgv(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("recorded stream output"),
	})
	effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
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
	effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
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
	workspace := filepath.Join(repoRoot, "docs", "temp", "agy-traces")
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
			effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
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
	effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
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
			effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
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

func TestCommandEffectTimeoutIsProviderFailure(t *testing.T) {
	t.Parallel()

	runner := blockingCommandRunner{}
	effect := agy.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "agy-print-timeout",
			Model:            "gemini-3.6-flash-low",
			PrintTimeout:     time.Millisecond,
			WorkingDirectory: t.TempDir(),
		},
	}, func([]byte) error { return nil })
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindTimeout {
		t.Fatalf("Execute() error = %#v, want timeout ExecuteFailure", err)
	}
}

type blockingCommandRunner struct{}

func (blockingCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
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
