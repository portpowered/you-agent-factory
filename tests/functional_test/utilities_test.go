package functional_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func agentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testutil.MustRepoPath(t, rel)
}

func clearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}
