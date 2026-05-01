package support

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

func AgentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testutil.MustRepoPath(t, rel)
}

func LegacyFixtureDir(t *testing.T, name string) string {
	t.Helper()
	return testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", name))
}

func ClearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}
