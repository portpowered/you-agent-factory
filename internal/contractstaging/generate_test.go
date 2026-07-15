package contractstaging_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestRepositoryGenerateLeavesOwnedStagingCleanOnSecondRun(t *testing.T) {
	defer contractstaging.LockRepositoryStagingForTest()()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	protected := []string{
		contractstaging.CanonicalOpenAPIPath,
		"api/openapi-main.yaml",
		"pkg/transports/http/generated/server.gen.go",
		"pkg/transports/http/client/client.gen.go",
		"ui/src/api/generated/openapi.ts",
	}
	beforeProtected := fileDigests(t, repositoryRoot, protected)
	beforeStaging := fileDigests(t, repositoryRoot, contractstaging.AllowedArtifacts())

	drift, err := contractstaging.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("precondition Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("precondition Check() drift = %#v, want clean staging before regeneration test", drift)
	}

	if err := contractstaging.Generate(repositoryRoot); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	afterFirst := fileDigests(t, repositoryRoot, contractstaging.AllowedArtifacts())
	drift, err = contractstaging.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("Check() after first Generate() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() after first Generate() drift = %#v, want none", drift)
	}

	if err := contractstaging.Generate(repositoryRoot); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	afterSecond := fileDigests(t, repositoryRoot, contractstaging.AllowedArtifacts())
	drift, err = contractstaging.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("Check() after second Generate() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() after second Generate() drift = %#v, want none", drift)
	}

	if !reflect.DeepEqual(beforeStaging, afterFirst) {
		t.Fatalf("first Generate() changed owned staging digests:\nbefore=%x\nafter=%x", beforeStaging, afterFirst)
	}
	if !reflect.DeepEqual(afterFirst, afterSecond) {
		t.Fatalf("second Generate() changed owned staging digests:\nfirst=%x\nsecond=%x", afterFirst, afterSecond)
	}
	if afterProtected := fileDigests(t, repositoryRoot, protected); !reflect.DeepEqual(beforeProtected, afterProtected) {
		t.Fatalf("Generate() changed protected local-source files:\nbefore=%x\nafter=%x", beforeProtected, afterProtected)
	}
}

func fileDigests(t *testing.T, root string, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	digests := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		digests[path] = sha256.Sum256(content)
	}
	return digests
}
