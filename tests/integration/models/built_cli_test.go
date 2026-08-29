package models_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

var story001Binary struct {
	once    sync.Once
	tempDir string
	path    string
	err     error
	output  []byte
}

// TestMain owns the reusable delivered CLI artifact for this package. The
// characterization cases intentionally start several child processes, so the
// Go build is performed once rather than once per process-level observation.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if story001Binary.tempDir != "" {
		if err := os.RemoveAll(story001Binary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove story-001 CLI directory %s: %v\n", story001Binary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func buildStory001Binary(t testing.TB) string {
	t.Helper()
	story001Binary.once.Do(func() {
		story001Binary.tempDir, story001Binary.err = os.MkdirTemp("", "you-localmodels-story-001-")
		if story001Binary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		story001Binary.path = filepath.Join(story001Binary.tempDir, binaryName)
		command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", story001Binary.path, "./cmd/factory")
		command.Dir = testutil.MustRepoRoot(t)
		story001Binary.output, story001Binary.err = command.CombinedOutput()
	})
	if story001Binary.err != nil {
		t.Fatalf("build delivered you binary: %v\n%s", story001Binary.err, story001Binary.output)
	}
	return story001Binary.path
}

func story001EnvironmentWithBrowserStub(t testing.TB, home, cache, endpoint string) []string {
	t.Helper()
	environment := story001Environment(home, cache, endpoint)
	binDir := filepath.Join(home, "story-001-browser-stub")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create story-001 browser stub directory: %v", err)
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("locate Go executable for story-001 browser stub: %v", err)
	}
	stubName := "xdg-open"
	if runtime.GOOS == "windows" {
		stubName = "rundll32.exe"
	} else if runtime.GOOS == "darwin" {
		stubName = "open"
	}
	stubPath := filepath.Join(binDir, stubName)
	goBinary, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("read Go executable for story-001 browser stub: %v", err)
	}
	if err := os.WriteFile(stubPath, goBinary, 0o755); err != nil {
		t.Fatalf("install story-001 browser stub: %v", err)
	}
	return prependStory001Path(environment, binDir)
}

func prependStory001Path(environment []string, directory string) []string {
	pathValue := ""
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			pathValue = value
			continue
		}
		filtered = append(filtered, entry)
	}
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	return append(filtered, "PATH="+directory+string(os.PathListSeparator)+pathValue)
}
