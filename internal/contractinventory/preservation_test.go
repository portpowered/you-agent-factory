package contractinventory

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

var protectedRESTContractPrefixes = []string{
	"api/openapi-main.yaml",
	"api/components/",
	"api/openapi.yaml",
	"pkg/api/",
	"pkg/generatedclient/",
	"ui/src/api/generated/",
}

func TestInventoryLane_ProtectedRESTContractSurfacesUnchanged(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Dir(testpath.MustRepoPathFromCaller(t, 0, "go.mod"))
	mergeBase, ok := resolveMergeBase(repoRoot)
	if !ok {
		t.Skip("main branch unavailable for preservation check")
	}

	changed, err := gitOutput(repoRoot, "diff", "--name-only", mergeBase, "HEAD")
	if err != nil {
		t.Fatalf("git diff --name-only: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(changed), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		normalized := filepath.ToSlash(line)
		for _, prefix := range protectedRESTContractPrefixes {
			if normalized == prefix || strings.HasPrefix(normalized, prefix) {
				t.Fatalf("inventory lane modified protected REST contract surface %q", normalized)
			}
		}
	}
}

func resolveMergeBase(repoRoot string) (string, bool) {
	for _, baseRef := range []string{"origin/main", "remotes/origin/main", "main"} {
		mergeBase, err := gitOutput(repoRoot, "merge-base", baseRef, "HEAD")
		if err == nil && mergeBase != "" {
			return mergeBase, true
		}
	}
	return "", false
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	trimmed := bytes.TrimSpace(out)
	if err != nil {
		if len(trimmed) == 0 {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, trimmed)
	}
	return string(trimmed), nil
}
