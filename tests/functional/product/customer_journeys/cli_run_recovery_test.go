package customer_journeys_test

import (
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCLIRunRecoversAfterMissingFactory(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	firstFactory := support.ScaffoldSingleStepFactory(t, "first-run")
	secondFactory := support.ScaffoldSingleStepFactory(t, "second-run")
	home := t.TempDir()
	env := append(isolatedEnvironment(home), runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, "models"))
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("worker COMPLETE"),
	})
	runFactory := func(factoryDir string) {
		t.Helper()
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--dir", factoryDir, "--no-record", "--quiet",
		})
		inputs.Input.Env, inputs.Input.WorkingDirectory = env, factoryDir
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("run --dir %s: %v\nstdout=%s\nstderr=%s", factoryDir, err, inputs.Stdout(), inputs.Stderr())
		}
	}

	runFactory(firstFactory)
	missingFactory := filepath.Join(t.TempDir(), "missing-factory.json")
	failed := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory, "--no-record", "--quiet",
	})
	failed.Input.Env, failed.Input.WorkingDirectory = env, filepath.Dir(missingFactory)
	if err := process.Execute(failed.Input); err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("missing Factory diagnostic = %v", err)
	}
	runFactory(secondFactory)

	help := support.FakeInputs(t.Context(), []string{"you", "--help"})
	help.Input.Env, help.Input.WorkingDirectory = env, secondFactory
	if err := process.Execute(help.Input); err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(help.Stdout(), "Usage:") {
		t.Fatalf("help output = %q, want public usage", help.Stdout())
	}
}
