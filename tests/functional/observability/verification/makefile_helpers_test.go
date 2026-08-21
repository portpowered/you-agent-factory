//go:build !windows

package verification

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeVerifyFastWrapperMakefile(t *testing.T, repoRoot string, overrides map[string]string) string {
	t.Helper()
	requirePOSIXShell(t)

	var body strings.Builder
	body.WriteString("SHELL := sh\n")
	body.WriteString(fmt.Sprintf("include %s\n\n", filepath.Join(repoRoot, "Makefile")))
	for target, recipe := range overrides {
		body.WriteString(fmt.Sprintf("%s:\n", target))
		for _, line := range strings.Split(strings.TrimSuffix(recipe, "\n"), "\n") {
			body.WriteString("\t")
			body.WriteString(line)
			body.WriteString("\n")
		}
		body.WriteString("\n")
	}

	path := filepath.Join(t.TempDir(), "verify-fast-wrapper.mk")
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatalf("write wrapper makefile: %v", err)
	}
	return filepath.ToSlash(path)
}

func runMakefileTarget(repoRoot, makefilePath, target string) (string, error) {
	return runMakefileTargetWithArgs(repoRoot, makefilePath, target)
}

func runMakefileTargetDryRun(repoRoot, makefilePath, target string) (string, error) {
	return runMakefileTargetWithArgs(repoRoot, makefilePath, "-n", target)
}

// runMakefileTargetWithPrerequisitesMarkedOld keeps wrapper execution focused
// on the target recipe. GNU Make merges prerequisites from the included rule
// and appended stub, so -o is the test-local seam for neutralizing inherited
// prerequisites during an invocation that must execute the stub.
func runMakefileTargetWithPrerequisitesMarkedOld(repoRoot, makefilePath, target string, prerequisites ...string) (string, error) {
	args := make([]string, 0, len(prerequisites)*2)
	for _, prerequisite := range prerequisites {
		args = append(args, "-o", prerequisite)
	}
	return runMakefileTargetWithArgs(repoRoot, makefilePath, target, args...)
}

func runMakefileTargetWithArgs(repoRoot, makefilePath, target string, args ...string) (string, error) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		return "", err
	}

	makeArgs := []string{
		"-f", makefilePath,
		"SHELL=sh",
		fmt.Sprintf("MAKE=%s -f %s", filepath.ToSlash(makePath), filepath.ToSlash(makefilePath)),
	}
	makeArgs = append(makeArgs, args...)
	makeArgs = append(makeArgs, target)

	cmd := exec.Command(makePath, makeArgs...)
	cmd.Dir = repoRoot
	cmd.Env = scrubMakeOverrideEnv(os.Environ(), functionalCoverageMakeOverrideVars...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	return output.String(), err
}

var functionalCoverageMakeOverrideVars = []string{
	"FUNCTIONAL_TEST_VIZ_DIR",
	"FUNCTIONAL_TEST_VIZ_PROFILE",
	"FUNCTIONAL_TEST_VIZ_JSON",
	"FUNCTIONAL_TEST_VIZ_MARKDOWN",
	"GO_FUNCTIONAL_COVERAGE_PROFILE",
	"GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT",
	"FUNCTIONAL_TEST_JOBS",
}

func scrubMakeOverrideEnv(environ []string, dropNames ...string) []string {
	drop := make(map[string]struct{}, len(dropNames))
	for _, name := range dropNames {
		drop[name] = struct{}{}
	}
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			out = append(out, entry)
			continue
		}
		if _, dropIt := drop[name]; dropIt {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func writeMakeEchoScript(t *testing.T, label string) string {
	t.Helper()
	requirePOSIXShell(t)

	path := filepath.Join(t.TempDir(), label)
	body := fmt.Sprintf("#!/bin/sh\nprintf '%%s:' %q\nprintf '%%s\\n' \"$*\"\n", label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write echo script: %v", err)
	}
	return filepath.ToSlash(path)
}

func writeFunctionallongFixture(t *testing.T, repoRoot string, invalid bool) string {
	t.Helper()

	dir, err := os.MkdirTemp(repoRoot, ".functionallong-gate-")
	if err != nil {
		t.Fatalf("create functionallong fixture directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove functionallong fixture directory: %v", err)
		}
	})

	source := strings.Join([]string{
		"//go:build functionallong",
		"",
		"package functionallongfixture",
		"",
		"func compiledOnly() string {",
		"\treturn \"compiled\"",
		"}",
	}, "\n") + "\n"
	if invalid {
		source += "\nvar _ = functionallongCompileGateIntentionalError\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture_functionallong.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("write functionallong fixture source: %v", err)
	}

	testSource := strings.Join([]string{
		"//go:build functionallong",
		"",
		"package functionallongfixture",
		"",
		"import \"testing\"",
		"",
		"func TestCompileOnlyGateMustNotExecute(t *testing.T) {",
		"\tt.Fatal(\"the functionallong compile gate must not execute tests\")",
		"}",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fixture_functionallong_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write functionallong fixture test: %v", err)
	}

	relativePath, err := filepath.Rel(repoRoot, dir)
	if err != nil {
		t.Fatalf("make functionallong fixture path relative: %v", err)
	}
	return "./" + filepath.ToSlash(relativePath)
}

func writeExecutableScript(t *testing.T, label string, body string) string {
	t.Helper()
	requirePOSIXShell(t)

	path := filepath.Join(t.TempDir(), label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable script: %v", err)
	}
	return filepath.ToSlash(path)
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("POSIX shell smoke test requires sh: %v", err)
	}
}

func runScript(repoRoot string, scriptPath string, env ...string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("sh", filepath.ToSlash(scriptPath))
	} else {
		cmd = exec.Command(scriptPath)
	}
	cmd.Dir = repoRoot
	cmd.Env = append(minimalScriptEnvironment(), env...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	return output.String(), err
}

func minimalScriptEnvironment() []string {
	path, ok := os.LookupEnv("PATH")
	if !ok {
		return []string{}
	}
	return []string{"PATH=" + path}
}

func assertOutputOrder(t *testing.T, output string, markers ...string) {
	t.Helper()

	cursor := 0
	for _, marker := range markers {
		next := strings.Index(output[cursor:], marker)
		if next < 0 {
			t.Fatalf("output missing marker %q:\n%s", marker, output)
		}
		cursor += next + len(marker)
	}
}
