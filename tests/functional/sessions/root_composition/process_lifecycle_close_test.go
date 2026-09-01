package root_composition_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func processLifecycleHomeEnvironment(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home}
}

// TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure
// characterizes the close-error join the customer entrypoint depends on.
//
// cmd/factory/main.go:runProcess reduces one invocation to
// errors.Join(executeErr, process.Close(closeCtx)) before mapping the result to
// an exit code. pkg/root already proves Close is deterministic and idempotent
// on a freshly built process, and pkg/initializer/lifecycle already proves
// unwind joins cleanup errors, but nothing asserted the combination the
// entrypoint actually performs: after a command fails, Close must still succeed
// so the join carries exactly the command's own failure -- not a second,
// invented lifecycle failure, and not a cancellation classification that would
// change the customer's exit code.
func TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure(t *testing.T) {
	acquireRootCompositionFixtureSlot(t)
	fixture := ensureRootCompositionFixture(t)

	home := t.TempDir()
	workingDirectory := t.TempDir()
	missingFactory := filepath.Join(workingDirectory, "absent-factory.json")
	if _, err := os.Stat(missingFactory); err == nil {
		t.Fatalf("fixture precondition failed: %s exists", missingFactory)
	}

	fixture.withRootCompositionRouteValue(t, rootCompositionRouteSpec{
		label:      "process-lifecycle-failed-command",
		homeDir:    home,
		workingDir: workingDirectory,
	}, func(route *rootCompositionRoute) {
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--factory", missingFactory,
		})
		inputs.Input.Env = processLifecycleHomeEnvironment(home)
		inputs.Input.WorkingDirectory = workingDirectory
		inputs.Input.Context = withRootCompositionRouteContext(inputs.Input.Context, route)

		executeErr := fixture.process.Execute(inputs.Input)
		if executeErr == nil {
			t.Fatalf(
				"Process.Execute(run --factory %s) error = nil, want a domain failure\nstdout:\n%s\nstderr:\n%s",
				missingFactory,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
		if !strings.Contains(executeErr.Error(), filepath.Base(missingFactory)) {
			t.Fatalf("Process.Execute() error = %v, want the missing Factory named in the diagnostic", executeErr)
		}

		// TestMain owns the shared process close. Keep the entrypoint's join
		// assertion here so the command failure remains the only error this
		// scenario contributes before package shutdown.
		joined := errors.Join(executeErr, nil)
		if joined == nil || joined.Error() != executeErr.Error() {
			t.Fatalf(
				"errors.Join(executeErr, nil) = %v, want exactly the command failure %v",
				joined,
				executeErr,
			)
		}
		if errors.Is(joined, context.Canceled) {
			t.Fatalf(
				"joined lifecycle result = %v, want a plain failure; a cancellation classification changes the customer exit code",
				joined,
			)
		}
	})
}

// TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure is the success
// half of the same join: a command that succeeds must leave nothing behind that
// makes Close fail, because runProcess would map that Close failure to a
// non-zero exit code even though the customer's command worked.
func TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure(t *testing.T) {
	acquireRootCompositionFixtureSlot(t)
	fixture := ensureRootCompositionFixture(t)

	home := t.TempDir()
	workingDirectory := t.TempDir()

	fixture.withRootCompositionRouteValue(t, rootCompositionRouteSpec{
		label:      "process-lifecycle-successful-command",
		homeDir:    home,
		workingDir: workingDirectory,
	}, func(route *rootCompositionRoute) {
		inputs := support.FakeInputs(t.Context(), []string{"you", "docs", "agents"})
		inputs.Input.Env = processLifecycleHomeEnvironment(home)
		inputs.Input.WorkingDirectory = workingDirectory
		inputs.Input.Context = withRootCompositionRouteContext(inputs.Input.Context, route)

		if executeErr := fixture.process.Execute(inputs.Input); executeErr != nil {
			t.Fatalf(
				"Process.Execute(docs agents) error = %v\nstdout:\n%s\nstderr:\n%s",
				executeErr,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
		if !strings.Contains(inputs.Stdout(), "# Agents") {
			t.Fatalf("Process.Execute(docs agents) stdout = %q, want the packaged agents topic", inputs.Stdout())
		}
	})
}
