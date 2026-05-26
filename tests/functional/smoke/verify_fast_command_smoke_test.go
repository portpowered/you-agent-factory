package smoke

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":             "@printf '%s\\n' 'stub:typecheck'\n",
		"ui-test":               "@printf '%s\\n' 'stub:ui-test'\n",
		"test":                  "@printf '%s\\n' 'stub:test'\n",
		"ui-install-playwright": "@printf '%s\\n' 'unexpected:ui-install-playwright'\n\t@exit 99\n",
		"ui-integration-test":   "@printf '%s\\n' 'unexpected:ui-integration-test'\n\t@exit 99\n",
		"test-functional-long":  "@printf '%s\\n' 'unexpected:test-functional-long'\n\t@exit 99\n",
		"long-tests":            "@printf '%s\\n' 'unexpected:long-tests'\n\t@exit 99\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-fast")
	if err != nil {
		t.Fatalf("run verify-fast wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running fast verification tier: typecheck + short UI/unit suite + short Go suite",
		"==> dashboard typecheck [make typecheck]",
		"stub:typecheck",
		"==> short UI/unit suite [make ui-test]",
		"stub:ui-test",
		"==> short Go suite [make test]",
		"stub:test",
	)

	for _, unwanted := range []string{
		"unexpected:ui-install-playwright",
		"unexpected:ui-integration-test",
		"unexpected:test-functional-long",
		"unexpected:long-tests",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-fast unexpectedly ran %q:\n%s", unwanted, output)
		}
	}
}

func TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck": "@printf '%s\\n' 'stub:typecheck'\n",
		"ui-test":   "@printf '%s\\n' 'stub:ui-test'\n\t@exit 17\n",
		"test":      "@printf '%s\\n' 'stub:test'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-fast")
	if err == nil {
		t.Fatalf("verify-fast unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: short UI/unit suite [make ui-test] failed. Rerun with: make ui-test") {
		t.Fatalf("verify-fast failure output missing rerun hint:\n%s", output)
	}
	if strings.Contains(output, "stub:test") {
		t.Fatalf("verify-fast continued after the failing owned suite:\n%s", output)
	}
}

func writeVerifyFastWrapperMakefile(t *testing.T, repoRoot string, overrides map[string]string) string {
	t.Helper()

	var body strings.Builder
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
	return path
}

func runMakefileTarget(repoRoot, makefilePath, target string) (string, error) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		return "", err
	}

	cmd := exec.Command(
		makePath,
		"-f", makefilePath,
		fmt.Sprintf("MAKE=%s -f %s", makePath, makefilePath),
		target,
	)
	cmd.Dir = repoRoot

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	return output.String(), err
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
