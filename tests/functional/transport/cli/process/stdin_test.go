package process_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

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

	stopped = true
	command.Cancel()
	_ = command.Wait()
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
