//go:build !windows

package verification

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFmtCheckReportsOnlyRealDrift proves the gate reports a misformatted
// tracked file without rewriting it, then passes silently after that file is
// formatted. The fixture path deliberately contains whitespace so the test
// also exercises the NUL-delimited tracked-file boundary.
func TestFmtCheckReportsOnlyRealDrift(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	fixtureRoot := t.TempDir()

	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatalf("read repository Makefile: %v", err)
	}
	makefilePath := filepath.Join(fixtureRoot, "Makefile")
	if err := os.WriteFile(makefilePath, makefile, 0o644); err != nil {
		t.Fatalf("write fixture Makefile: %v", err)
	}

	relativeFixturePath := filepath.ToSlash(filepath.Join("pkg", "format fixture", "fixture.go"))
	fixturePath := filepath.Join(fixtureRoot, filepath.FromSlash(relativeFixturePath))
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	misformatted := []byte("package fmtcheckfixture\n\nfunc unformatted( ) {}\n")
	if err := os.WriteFile(fixturePath, misformatted, 0o644); err != nil {
		t.Fatalf("write misformatted fixture: %v", err)
	}

	runFmtCheckGitCommand(t, fixtureRoot, "init", "-q")
	runFmtCheckGitCommand(t, fixtureRoot, "add", "--", "Makefile", relativeFixturePath)

	before, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read misformatted fixture before check: %v", err)
	}
	output, err := runMakefileTarget(fixtureRoot, makefilePath, "fmt-check")
	if err == nil {
		t.Fatalf("fmt-check unexpectedly accepted misformatted fixture:\n%s", output)
	}
	if !strings.Contains(output, relativeFixturePath) {
		t.Fatalf("fmt-check failure did not report %q:\n%s", relativeFixturePath, output)
	}
	after, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read misformatted fixture after check: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("fmt-check rewrote the misformatted fixture: before=%q after=%q", before, after)
	}

	formatted := []byte("package fmtcheckfixture\n\nfunc unformatted() {}\n")
	if err := os.WriteFile(fixturePath, formatted, 0o644); err != nil {
		t.Fatalf("write formatted fixture: %v", err)
	}
	output, err = runMakefileTarget(fixtureRoot, makefilePath, "fmt-check")
	if err != nil {
		t.Fatalf("fmt-check rejected formatted fixture: %v\n%s", err, output)
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("fmt-check emitted output for formatted fixture:\n%s", output)
	}
}

func runFmtCheckGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output.String())
	}
}
