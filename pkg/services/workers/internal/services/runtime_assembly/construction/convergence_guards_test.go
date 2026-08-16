package construction

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

func TestServiceCopiesRetainConvergedRunnerDependencies(t *testing.T) {
	t.Parallel()

	if got := (*Service)(nil).WithRunWorktree("worktree"); got != nil {
		t.Fatalf("nil Service.WithRunWorktree() = %#v, want nil", got)
	}
	if got := (*Service)(nil).WithRunReasoningEffort("high"); got != nil {
		t.Fatalf("nil Service.WithRunReasoningEffort() = %#v, want nil", got)
	}
	service := New(
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	configured := service.
		WithRunWorktree("/tmp/worktree").
		WithRunReasoningEffort("high")
	if configured == service {
		t.Fatal("service copy mutated the original")
	}
	if configured.runWorktree != "/tmp/worktree" || configured.runReasoningEffort != "high" {
		t.Fatalf("run-scoped options = %q/%q, want retained options", configured.runWorktree, configured.runReasoningEffort)
	}
	rebuilt := configured.WithExecutionFactories(nil)
	if rebuilt == configured {
		t.Fatal("WithExecutionFactories() mutated the configured service")
	}
}

func TestServiceBuildLogicalUsesConfiguredConstructionDependencies(t *testing.T) {
	service := New(
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	result := service.BuildLogical(
		runtimefixtures.RuntimeConfigLookupFixture{},
		"logical",
		workers.RunnerIDCodex,
		nil,
		logging.NoopLogger{},
		testClock,
		os.Environ,
		os.Getwd,
	)
	if result.Dispatch == nil || result.Direct != nil {
		t.Fatalf("BuildLogical() = %#v, want dispatch-only result", result)
	}
}
