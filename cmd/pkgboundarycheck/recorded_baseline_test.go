package main

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestRunRejectsInvalidExplicitBaseRef(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/work/committed.go", "work", repositoryImportPrefix+"pkg/transports/http")
	commitRecordedBoundaryFixture(t, repoRoot)

	err := run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "missing-base-ref"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != `package-boundary base ref "missing-base-ref" could not be resolved` {
		t.Fatalf("run() error = %v, want invalid explicit base-ref error", err)
	}
}

func commitRecordedBoundaryFixture(t *testing.T, repoRoot string) {
	t.Helper()
	runGitFixtureCommand(t, repoRoot, "init", "-q")
	runGitFixtureCommand(t, repoRoot, "config", "user.email", "pkg-boundary-test@example.invalid")
	runGitFixtureCommand(t, repoRoot, "config", "user.name", "pkg-boundary test")
	runGitFixtureCommand(t, repoRoot, "add", ".")
	runGitFixtureCommand(t, repoRoot, "commit", "-qm", "record package-boundary fixture")
}

func runGitFixtureCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func TestRecordedBaselineFingerprintSurvivesLineMotion(t *testing.T) {
	before := serviceConstructionFinding{
		owner:      "pkg/services/factory_definitions",
		importPath: "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire",
		symbol:     "NewService",
		filePath:   "tests/functional/factory/definitions/execution_catalog_test.go",
		line:       348,
		count:      1,
	}
	moved := before
	moved.line = 386

	if got, want := boundaryFindingFingerprint("service-construction", moved), boundaryFindingFingerprint("service-construction", before); got != want {
		t.Fatalf("moving a recorded finding to a new line changed its fingerprint:\n got %s\nwant %s", got, want)
	}
}

func TestRecordedBaselineFingerprintStillSeparatesRealGrowth(t *testing.T) {
	base := serviceConstructionFinding{
		owner:      "pkg/services/work",
		importPath: "github.com/portpowered/infinite-you/pkg/services/work/wire",
		symbol:     "NewRuntimeService",
		filePath:   "pkg/services/factory_runtime/internal/runtime/helpers_test.go",
		line:       105,
		count:      1,
	}
	baseFingerprint := boundaryFindingFingerprint("service-construction", base)

	extraSelection := base
	extraSelection.count = 2
	if boundaryFindingFingerprint("service-construction", extraSelection) == baseFingerprint {
		t.Fatal("an additional construction selection must not reuse the recorded fingerprint")
	}

	otherFile := base
	otherFile.filePath = "pkg/services/factory_runtime/internal/runtime/other_test.go"
	if boundaryFindingFingerprint("service-construction", otherFile) == baseFingerprint {
		t.Fatal("the same construction in a different file must not reuse the recorded fingerprint")
	}

	otherSymbol := base
	otherSymbol.symbol = "NewProjectionService"
	if boundaryFindingFingerprint("service-construction", otherSymbol) == baseFingerprint {
		t.Fatal("a different constructed symbol must not reuse the recorded fingerprint")
	}

	if boundaryFindingFingerprint("test-service-import", base) == baseFingerprint {
		t.Fatal("a different finding category must not reuse the recorded fingerprint")
	}
}
