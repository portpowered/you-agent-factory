package workersessions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Remote clients share initialized operator storage. Each connection still owns
// its context, pipes and working directory; real hosts retain their own stores.
type selectedHostClientProcess struct {
	support.Process
	environment []string
}

func (process selectedHostClientProcess) Execute(input root.Input) error {
	input.Env = append([]string(nil), process.environment...)
	return process.Process.Execute(input)
}

func newSelectedHostClientProcess(t *testing.T) support.Process {
	t.Helper()
	home := t.TempDir()
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "HOME="+home, "USERPROFILE="+home,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, "models"))
	process := selectedHostClientProcess{
		Process:     support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: subagentScenarioRunner{t: t}}),
		environment: environment,
	}
	// Finish bootstrap before any parallel connection can use this home.
	inputs := support.FakeInputs(t.Context(), []string{"you", "init", "--provider", "codex"})
	inputs.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("initialize remote-client home: %v\n%s", err, inputs.Stderr())
	}
	return process
}
