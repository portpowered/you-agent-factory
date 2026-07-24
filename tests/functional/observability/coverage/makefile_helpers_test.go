//go:build !windows

package coverage

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// Drop ambient CI/local Make overrides so contract proofs observe Makefile
	// defaults (or explicit command-line args). Required Backend Functional
	// Coverage exports FUNCTIONAL_TEST_VIZ_DIR into the suite process; without
	// scrubbing, make -n / stubbed default-path assertions inherit that path.
	cmd.Env = scrubMakeOverrideEnv(os.Environ(), functionalCoverageMakeOverrideVars...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	return output.String(), err
}

// functionalCoverageMakeOverrideVars are Make variables the functional viz /
// coverage surface accepts from the environment. Contract and fail-closed
// smokes must not inherit CI values for these when proving defaults or when
// the caller already passes an explicit Make command-line override.
var functionalCoverageMakeOverrideVars = []string{
	"FUNCTIONAL_TEST_VIZ_DIR",
	"FUNCTIONAL_TEST_VIZ_PROFILE",
	"FUNCTIONAL_TEST_VIZ_JSON",
	"FUNCTIONAL_TEST_VIZ_MARKDOWN",
	"GO_FUNCTIONAL_COVERAGE_PROFILE",
	"GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT",
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

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("POSIX shell smoke test requires sh: %v", err)
	}
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
