package runtime_api

import (
	"os/exec"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestRuntimeAPI_CompilesWithFunctionalLongTag(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	cmd := exec.Command("go", "test", "-tags=functionallong", "./tests/functional/runtime_api", "-run", "^$", "-count=0")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile runtime_api with functionallong tag: %v\n%s", err, output)
	}
}
