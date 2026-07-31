package support

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestProviderSessionFixtureRootIsOwnedByFunctionalTestSupport(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)

	fixtureRel := ProviderSessionFixturePath("README.md")
	fixtureAbs := filepath.Join(repoRoot, filepath.FromSlash(fixtureRel))
	if _, err := os.Stat(fixtureAbs); err != nil {
		t.Fatalf("functional test fixture root %s: %v", fixtureRel, err)
	}
	wantRoot := filepath.Join(repoRoot, "tests", "functional", "internal", "support", "testdata", "provider-sessions")
	if got := filepath.Dir(fixtureAbs); !samePath(got, wantRoot) {
		t.Fatalf("fixture root = %q, want test-owned root %q", got, wantRoot)
	}
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
