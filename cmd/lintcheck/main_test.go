package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProcessCheckerReusesAndInvalidatesTransitiveInputs(t *testing.T) {
	driver := testDriver(t)
	fixtureRoot := writeCheckerFixture(t, "one")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	environment := map[string]string{"LINT_CHECKER_TEST_VALUE": "forwarded"}

	output, err := runDriver(driver, fixtureRoot, cacheDir, false, environment, "first arg", "second")
	if err != nil {
		t.Fatalf("first checker run: %v; output: %s", err, output)
	}
	if !strings.Contains(output, "version=one args=first arg|second env=forwarded") {
		t.Fatalf("first checker output = %q", output)
	}
	cacheFiles := lintCheckerCacheFiles(t, cacheDir)
	if len(cacheFiles) != 1 {
		t.Fatalf("cached checker files = %v, want one executable", cacheFiles)
	}

	preservedTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(cacheFiles[0], preservedTime, preservedTime); err != nil {
		t.Fatalf("set cache timestamp: %v", err)
	}
	output, err = runDriver(driver, fixtureRoot, cacheDir, false, environment, "first arg", "second")
	if err != nil {
		t.Fatalf("reused checker run: %v; output: %s", err, output)
	}
	info, err := os.Stat(cacheFiles[0])
	if err != nil {
		t.Fatalf("stat reused checker: %v", err)
	}
	if !info.ModTime().Equal(preservedTime) {
		t.Fatalf("reused checker timestamp = %v, want %v", info.ModTime(), preservedTime)
	}

	writeCheckerVersion(t, fixtureRoot, "two")
	output, err = runDriver(driver, fixtureRoot, cacheDir, false, environment, "changed")
	if err != nil {
		t.Fatalf("stale-input checker run: %v; output: %s", err, output)
	}
	if !strings.Contains(output, "version=two args=changed env=forwarded") {
		t.Fatalf("stale-input checker output = %q", output)
	}
	if cacheFiles = lintCheckerCacheFiles(t, cacheDir); len(cacheFiles) != 2 {
		t.Fatalf("cache files after input change = %v, want two fingerprints", cacheFiles)
	}

	environment["LINT_CHECKER_TEST_EXIT"] = "7"
	output, err = runDriver(driver, fixtureRoot, cacheDir, false, environment, "failure")
	assertExitCode(t, err, 7)
	if !strings.Contains(output, "version=two args=failure env=forwarded") {
		t.Fatalf("failed checker output = %q", output)
	}
}

func TestProcessCheckerFallbackUsesGoRunAndPreservesExitStatus(t *testing.T) {
	driver := testDriver(t)
	fixtureRoot := writeCheckerFixture(t, "fallback")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	environment := map[string]string{"LINT_CHECKER_TEST_VALUE": "fallback-env"}

	output, err := runDriver(driver, fixtureRoot, cacheDir, true, environment, "fallback arg")
	if err != nil {
		t.Fatalf("fallback checker run: %v; output: %s", err, output)
	}
	if !strings.Contains(output, "version=fallback args=fallback arg env=fallback-env") {
		t.Fatalf("fallback checker output = %q", output)
	}
	if files := lintCheckerCacheFiles(t, cacheDir); len(files) != 0 {
		t.Fatalf("fallback cache files = %v, want none", files)
	}

	environment["LINT_CHECKER_TEST_EXIT"] = "9"
	output, err = runDriver(driver, fixtureRoot, cacheDir, true, environment, "fallback failure")
	assertExitCode(t, err, 9)
	if !strings.Contains(output, "version=fallback args=fallback failure") {
		t.Fatalf("fallback failure output = %q", output)
	}
}

func TestProcessCheckerCompileFailuresRemainVisible(t *testing.T) {
	driver := testDriver(t)
	fixtureRoot := writeCheckerFixture(t, "broken")
	checkerPath := filepath.Join(fixtureRoot, "checker", "main.go")
	if err := os.WriteFile(checkerPath, []byte("package main\nfunc main(\n"), 0o644); err != nil {
		t.Fatalf("write broken checker: %v", err)
	}
	output, err := runDriver(driver, fixtureRoot, filepath.Join(t.TempDir(), "cache"), false, nil)
	if err == nil {
		t.Fatalf("broken checker succeeded; output: %s", output)
	}
	if !strings.Contains(output, "main.go") {
		t.Fatalf("compile failure output = %q, want source location", output)
	}
}

func testDriver(t *testing.T) string {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("lint checker process test requires Go: %v", err)
	}
	return buildDriver(t, goTool)
}

func buildDriver(t *testing.T, goTool string) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve lint checker test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
	driver := filepath.Join(t.TempDir(), "lintcheck"+executableSuffix())
	command := exec.Command(goTool, "build", "-o", driver, "./cmd/lintcheck")
	command.Dir = repoRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build lint checker driver: %v; output: %s", err, output)
	}
	return driver
}

func writeCheckerFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module lintfixture\n\ngo 1.25.0\n")
	writeFixtureFile(t, root, "version/value.go", fmt.Sprintf("package version\n\nconst Value = %q\n", version))
	writeFixtureFile(t, root, "checker/main.go", `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"lintfixture/version"
)

func main() {
	fmt.Printf("version=%s args=%s env=%s\n", version.Value, strings.Join(os.Args[1:], "|"), os.Getenv("LINT_CHECKER_TEST_VALUE"))
	if value := os.Getenv("LINT_CHECKER_TEST_EXIT"); value != "" {
		code, err := strconv.Atoi(value)
		if err != nil {
			panic(err)
		}
		os.Exit(code)
	}
}
`)
	return root
}

func writeCheckerVersion(t *testing.T, root, version string) {
	t.Helper()
	writeFixtureFile(t, root, "version/value.go", fmt.Sprintf("package version\n\nconst Value = %q\n", version))
}

func writeFixtureFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file %s: %v", relativePath, err)
	}
}

func runDriver(driver, fixtureRoot, cacheDir string, fallback bool, environment map[string]string, args ...string) (string, error) {
	commandArgs := []string{"-cache-dir", cacheDir, "-go", "go", "-package", "./checker"}
	if fallback {
		commandArgs = append(commandArgs, "-fallback")
	}
	commandArgs = append(commandArgs, "--")
	commandArgs = append(commandArgs, args...)
	command := exec.Command(driver, commandArgs...)
	command.Dir = fixtureRoot
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	command.Env = append(os.Environ(), command.Env...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.String(), err
}

func lintCheckerCacheFiles(t *testing.T, cacheDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(cacheDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read lint checker cache: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, filepath.Join(cacheDir, entry.Name()))
		}
	}
	return files
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("checker succeeded, want exit code %d", want)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("checker error = %v, want process exit", err)
	}
	if got := exitErr.ExitCode(); got != want {
		t.Fatalf("checker exit code = %d, want %d", got, want)
	}
}

func TestParseConfigRejectsMissingPackage(t *testing.T) {
	if _, err := parseConfig([]string{"-go", "go"}, &bytes.Buffer{}); err == nil {
		t.Fatal("parseConfig succeeded without checker package")
	}
}
