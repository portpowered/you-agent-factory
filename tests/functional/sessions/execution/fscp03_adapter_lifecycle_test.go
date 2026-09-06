package execution_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFSCP03RetainedOpeningReusesOneProcessAndKeepsHelpInert proves the
// executable spine through root.BuildProcess and Process.Execute: two legacy
// run invocations reuse one process graph, a failed opening does not poison a
// later invocation, and help remains invocation-owned without opening a
// runtime or listener.
func TestFSCP03RetainedOpeningReusesOneProcessAndKeepsHelpInert(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	firstFactory := support.ScaffoldSingleStepFactory(t, "fscp03-first")
	secondFactory := support.ScaffoldSingleStepFactory(t, "fscp03-second")
	var sessionIDs atomic.Int32
	var runtimeIDs atomic.Int32
	var listenerStarts atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			return errors.New("FSCP03 listener must not start for this cell")
		},
		FactorySessionIDGenerator: func() string {
			return fmt.Sprintf("fscp03-session-%d", sessionIDs.Add(1))
		},
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			return fmt.Sprintf("fscp03-runtime-%d", runtimeIDs.Add(1))
		},
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("fscp03 worker COMPLETE"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	if sessionIDs.Load() != 0 || runtimeIDs.Load() != 0 || listenerStarts.Load() != 0 {
		t.Fatalf("construction effects = session:%d runtime:%d listener:%d, want all zero", sessionIDs.Load(), runtimeIDs.Load(), listenerStarts.Load())
	}

	runFactory := func(factoryDir string) {
		t.Helper()
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--dir", factoryDir, "--no-record", "--quiet",
		})
		inputs.Input.Env = isolatedEnvironment(t.TempDir())
		inputs.Input.WorkingDirectory = factoryDir
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("Process.Execute(run --dir %s) error = %v\nstdout=%s\nstderr=%s", factoryDir, err, inputs.Stdout(), inputs.Stderr())
		}
	}

	runFactory(firstFactory)
	firstSessionIDs, firstRuntimeIDs := sessionIDs.Load(), runtimeIDs.Load()
	if firstSessionIDs == 0 || firstRuntimeIDs == 0 {
		t.Fatalf("first run identities = session:%d runtime:%d, want runtime opening through the retained root", firstSessionIDs, firstRuntimeIDs)
	}

	missingFactory := filepath.Join(t.TempDir(), "missing-factory.json")
	failed := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory, "--no-record", "--quiet",
	})
	failed.Input.Env = isolatedEnvironment(t.TempDir())
	failed.Input.WorkingDirectory = filepath.Dir(missingFactory)
	if err := process.Execute(failed.Input); err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("failed Process.Execute() error = %v, want the missing Factory diagnostic", err)
	}
	if sessionIDs.Load() != firstSessionIDs || runtimeIDs.Load() != firstRuntimeIDs {
		t.Fatalf("failed opening allocated identities = session:%d runtime:%d, want no partial session/runtime", sessionIDs.Load(), runtimeIDs.Load())
	}

	runFactory(secondFactory)
	if sessionIDs.Load() <= firstSessionIDs || runtimeIDs.Load() <= firstRuntimeIDs {
		t.Fatalf("second run identities = session:%d runtime:%d, want a new session/runtime on the same process", sessionIDs.Load(), runtimeIDs.Load())
	}

	help := support.FakeInputs(t.Context(), []string{"you", "--help"})
	help.Input.Env = isolatedEnvironment(t.TempDir())
	help.Input.WorkingDirectory = secondFactory
	if err := process.Execute(help.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v", err)
	}
	if !strings.Contains(help.Stdout(), "Usage:") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want public usage output", help.Stdout())
	}
	if listenerStarts.Load() != 0 {
		t.Fatalf("help listener starts = %d, want no runtime transport activation", listenerStarts.Load())
	}
}
