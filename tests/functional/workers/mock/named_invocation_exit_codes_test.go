package mock

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const characterizedNamedFactory = "@you/goal"

// TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot pins the existing
// one-shot process contract before batch reporting changes. The test uses the
// compiled executable and serialized mock workers because injected edges cannot
// cross that OS process boundary.
func TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildContext, testutil.MustRepoRoot(t))

	t.Run("success preserves primary result", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-named-success")
		mockWorkersPath := writeAcceptingGoalMockWorkers(t)

		result, err := runBuiltYouBinary(ctx, binaryPath, session, characterizedNamedRunArgs(
			session,
			mockWorkersPath,
			"compiled named success",
			"--quiet",
		)...)
		if err != nil {
			t.Fatalf("compiled you run --named success: %v\nstdout:\n%s\nstderr:\n%s", err, result.Stdout, result.Stderr)
		}
		if result.ExitCode != 0 {
			t.Fatalf("success exit code = %d, want 0; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
		}
		if result.Stdout != successStdoutPrimaryResult {
			t.Fatalf("success stdout = %q, want established primary result %q", result.Stdout, successStdoutPrimaryResult)
		}
		if result.Stderr != "" {
			t.Fatalf("success stderr = %q, want empty", result.Stderr)
		}
	})

	t.Run("terminal failure preserves human detail", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-named-human-failure")
		mockWorkersPath := writeRejectingGoalMockWorkers(t)

		result, err := runBuiltYouBinary(ctx, binaryPath, session, characterizedNamedRunArgs(
			session,
			mockWorkersPath,
			"compiled named human failure",
			"--output", "response-stream",
		)...)
		if err == nil {
			t.Fatalf("compiled you run --named human failure succeeded: %#v", result)
		}
		if result.ExitCode == 0 {
			t.Fatalf("human failure exit code = %d, want non-zero", result.ExitCode)
		}
		for _, want := range []string{"status: FAILED", "workName: ", "workState: goal:failed"} {
			if !strings.Contains(result.Stdout, want) {
				t.Fatalf("human failure stdout missing %q:\n%s", want, result.Stdout)
			}
		}
		if !hasNonEmptyLabeledValue(result.Stdout, "workName: ") {
			t.Fatalf("human failure stdout has an empty Work name:\n%s", result.Stdout)
		}
	})

	t.Run("terminal failure preserves JSON detail", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-named-json-failure")
		mockWorkersPath := writeRejectingGoalMockWorkers(t)

		args := characterizedNamedRunArgs(
			session,
			mockWorkersPath,
			"compiled named JSON failure",
			"--output", "response-stream",
		)
		args = append([]string{"--json"}, args...)
		result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
		if err == nil {
			t.Fatalf("compiled you run --named JSON failure succeeded: %#v", result)
		}
		if result.ExitCode == 0 {
			t.Fatalf("JSON failure exit code = %d, want non-zero", result.ExitCode)
		}

		response := decodeNamedInvocationResult(t, result.Stdout)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("JSON failure status = %q, want FAILED", response.Status)
		}
		if response.WorkName == nil || strings.TrimSpace(*response.WorkName) == "" {
			t.Fatalf("JSON failure Work name = %#v, want non-empty", response.WorkName)
		}
		if response.WorkState == nil || *response.WorkState != "goal:failed" {
			t.Fatalf("JSON failure Work state = %#v, want goal:failed", response.WorkState)
		}
	})
}

func characterizedNamedRunArgs(
	session *builtcliacceptance.Session,
	mockWorkersPath string,
	prompt string,
	extra ...string,
) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", characterizedNamedFactory,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
	)
	args = append(args, extra...)
	return append(args, prompt)
}

func decodeNamedInvocationResult(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		t.Fatal("JSON invocation stdout is empty")
	}
	var terminal factoryapi.InvocationResponse
	terminalRecords := 0
	for lineNumber, line := range strings.Split(trimmed, "\n") {
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON invocation stdout line %d: %v\nstdout:\n%s", lineNumber+1, err, stdout)
		}
		var recordType string
		if err := json.Unmarshal(record["recordType"], &recordType); err != nil {
			t.Fatalf("decode JSON invocation stdout record type on line %d: %v", lineNumber+1, err)
		}
		if recordType != "invocation_result" {
			continue
		}
		if err := json.Unmarshal(record["response"], &terminal); err != nil {
			t.Fatalf("decode JSON invocation terminal response: %v", err)
		}
		terminalRecords++
	}
	if terminalRecords != 1 {
		t.Fatalf("terminal invocation result records = %d, want one; stdout:\n%s", terminalRecords, stdout)
	}
	return terminal
}

func hasNonEmptyLabeledValue(output, label string) bool {
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, label); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
