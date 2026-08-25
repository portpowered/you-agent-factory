package process_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestBuiltCLIRunDirectorySelectionIgnoresOpenStdin proves the descriptor
// lifetime behavior of a separately built CLI process. Both live and replay
// directory-selected runs must finish without an EOF from an unrelated pipe,
// with byte-for-byte equivalent terminal streams to a closed-stdin control.
func TestBuiltCLIRunDirectorySelectionIgnoresOpenStdin(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)
	factoryPath := writeStdinRunFactory(t, session.WorkDir)
	factoryDir := filepath.Dir(factoryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))

	baseArgs := append([]string{}, session.RuntimeLogDirFlags()...)
	baseArgs = append(baseArgs, session.ServerFlags()...)
	recordingPath := filepath.Join(session.WorkDir, "directory-run.replay.json")
	recordArgs := append(append([]string{}, baseArgs...),
		"run", "--dir", factoryDir, "--record", recordingPath, "--quiet",
	)
	if result, err := runBuiltCLIWithClosedStdin(t, binaryPath, session, recordArgs...); err != nil || result.ExitCode != 0 {
		t.Fatalf("create directory-run replay recording: result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(recordingPath); err != nil {
		t.Fatalf("directory-run replay recording %q: %v", recordingPath, err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "live", args: append(append([]string{}, baseArgs...), "run", "--dir", factoryDir, "--no-record", "--quiet")},
		{name: "replay", args: append(append([]string{}, baseArgs...), "run", "--dir", factoryDir, "--replay", recordingPath, "--no-record", "--quiet")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			closed, closedErr := runBuiltCLIWithClosedStdin(t, binaryPath, session, test.args...)
			if closedErr != nil || closed.ExitCode != 0 {
				t.Fatalf("closed-stdin %s run: result=%#v err=%v", test.name, closed, closedErr)
			}

			open, openErr := runBuiltCLIWithOpenStdin(t, binaryPath, session, test.args...)
			if openErr != nil || open.ExitCode != 0 {
				t.Fatalf("open-stdin %s run: result=%#v err=%v", test.name, open, openErr)
			}
			if open != closed {
				t.Fatalf("open-stdin %s result=%#v differs from closed-stdin control=%#v", test.name, open, closed)
			}
		})
	}

	closedHelp, closedHelpErr := runBuiltCLIWithClosedStdin(t, binaryPath, session, "--help")
	if closedHelpErr != nil || closedHelp.ExitCode != 0 {
		t.Fatalf("closed-stdin help: result=%#v err=%v", closedHelp, closedHelpErr)
	}
	openHelp, openHelpErr := runBuiltCLIWithOpenStdin(t, binaryPath, session, "--help")
	if openHelpErr != nil || openHelp.ExitCode != 0 {
		t.Fatalf("open-stdin help: result=%#v err=%v", openHelp, openHelpErr)
	}
	if openHelp != closedHelp {
		t.Fatalf("open-stdin help result=%#v differs from closed-stdin control=%#v", openHelp, closedHelp)
	}
	if !strings.Contains(openHelp.Stdout, "Available Commands:") {
		t.Fatalf("open-stdin help omitted full command listing:\n%s", openHelp.Stdout)
	}
}

func runBuiltCLIWithClosedStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create closed-stdin pipe: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		t.Fatalf("close closed-stdin pipe writer: %v", err)
	}
	return runBuiltCLIWithStdin(t, binaryPath, session, reader, args...)
}

func runBuiltCLIWithOpenStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create open-stdin pipe: %v", err)
	}
	defer writer.Close()
	return runBuiltCLIWithStdin(t, binaryPath, session, reader, args...)
}

func runBuiltCLIWithStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	stdin *os.File,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	defer stdin.Close()

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	command.Stdin = stdin
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI with controlled stdin: %v", err)
	}

	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	var runErr error
	select {
	case runErr = <-waitResult:
	case <-time.After(30 * time.Second):
		// This is only a bounded failure guard for the OS-process test. The
		// assertion is completion while the writer remains open, not a latency
		// threshold.
		_ = command.Process.Kill()
		runErr = <-waitResult
		t.Fatalf("built CLI did not finish with controlled stdin: %v", runErr)
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("built CLI wait: %v", runErr)
		}
		exitCode = exitErr.ExitCode()
	}
	return builtcliacceptance.RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, runErr
}

const (
	stdinSubmitBatchRequestID = "functional-stdin-submit-batch"
	stdinSubmitBatchWorkName  = "stdin-batch-task"
	stdinSubmitBatchWorkType  = "task"
)

// TestRunReadsPromptFromStdin proves you run - consumes piped stdin and
// delivers the exact prompt bytes to the selected worker through the public
// built you CLI process boundary.
func TestRunReadsPromptFromStdin(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	factoryPath := writeStdinRunFactory(t, session.WorkDir)
	mockWorkersPath := writeStdinRunDefaultMockWorkers(t)
	prompt := fmt.Sprintf("functional-stdin-run-café-résumé-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"--quiet",
		"-",
	)

	result, err := session.RunWithStdin(ctx, prompt, args...)
	if err != nil {
		t.Fatalf(
			"you run - via stdin: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			result.Stdout,
			result.Stderr,
		)
	}
	if got := result.Stdout; got != prompt {
		t.Fatalf("worker-bound primary result = %q, want exact stdin prompt %q", got, prompt)
	}
}

// TestSubmitBatchReadsJSONFromStdin proves you submit batch - consumes one
// canonical FACTORY_REQUEST_BATCH document from piped stdin and acknowledges the
// accepted work through the public built you CLI process boundary.
func TestSubmitBatchReadsJSONFromStdin(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	command, lines, scanErr, stderr := startRootProcessServerCommand(t, session, harness)
	stopped := false
	defer func() {
		if !stopped {
			command.Cancel()
			_ = command.Wait()
			waitForScannerCompletion(t, scanErr, "stdin submit server", 5*time.Second)
		}
	}()
	_ = waitForDashboardURL(t, lines, scanErr, stderr, 45*time.Second)

	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Stdin submit batch process"}}]}`,
		stdinSubmitBatchRequestID,
		stdinSubmitBatchWorkName,
		stdinSubmitBatchWorkType,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	args := append([]string{}, session.ServerFlags()...)
	args = append(args, "submit", "batch", "-")

	result, err := session.RunWithStdin(ctx, batchJSON, args...)
	if err != nil {
		t.Fatalf(
			"you submit batch - via stdin: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			result.Stdout,
			result.Stderr,
		)
	}

	output := result.Stdout
	for _, marker := range []string{
		"requestId: " + stdinSubmitBatchRequestID,
		"traceId:",
		"work count: 1",
		stdinSubmitBatchWorkName + " (" + stdinSubmitBatchWorkType + ")",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, output)
		}
	}

	command.Cancel()
	_ = command.Wait()
	stopped = true
	waitForScannerCompletion(t, scanErr, "stdin submit server", 5*time.Second)
}

// TestCLIEmptyRequiredStdinFailsWithoutDispatch proves you run - rejects EOF or
// empty required stdin through the public built you CLI process boundary before
// any worker-bound primary result or other external worker effect attributable
// to the invocation.
func TestCLIEmptyRequiredStdinFailsWithoutDispatch(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	factoryPath := writeStdinRunFactory(t, session.WorkDir)
	mockWorkersPath := writeStdinRunDefaultMockWorkers(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"--quiet",
		"-",
	)

	result, err := session.RunWithStdin(ctx, "", args...)
	if err == nil {
		t.Fatalf(
			"you run - with empty stdin succeeded; want pre-dispatch rejection\nstdout:\n%s\nstderr:\n%s",
			result.Stdout,
			result.Stderr,
		)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want documented validation-failure exit 1", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf(
			"stdout = %q, want empty (no worker-bound primary result before stdin rejection)",
			result.Stdout,
		)
	}
	diagnostic := result.Stderr + "\n" + err.Error()
	if !strings.Contains(diagnostic, "INVOCATION_INPUT_EMPTY") {
		t.Fatalf(
			"empty stdin diagnostic missing INVOCATION_INPUT_EMPTY:\nstdout:\n%s\nstderr:\n%s",
			result.Stdout,
			result.Stderr,
		)
	}
}
