//go:build !windows

package verification

import (
	"strings"
	"testing"
)

func TestRunScriptEnvironment_IsolatesAmbientContractVariables(t *testing.T) {
	repoRoot := t.TempDir()
	scriptPath := writeExecutableScript(t, "print-script-environment", `#!/bin/sh
printf 'parent-only=%s\n' "${FUNCTIONAL_PARENT_ONLY_SENTINEL:-unset}"
printf 'artifact-root=%s\n' "${ARTIFACT_ROOT:-unset}"
printf 'github-run-id=%s\n' "${GITHUB_RUN_ID:-unset}"
printf 'short=%s\n' "${FUNCTIONAL_SHORT:-unset}"
printf 'explicit=%s\n' "${FUNCTIONAL_EXPLICIT_SENTINEL:-unset}"
`)

	for _, ambientShort := range []string{"true", "false"} {
		t.Run("ambient FUNCTIONAL_SHORT="+ambientShort, func(t *testing.T) {
			t.Setenv("FUNCTIONAL_PARENT_ONLY_SENTINEL", "ambient-only")
			t.Setenv("ARTIFACT_ROOT", "ambient-artifact")
			t.Setenv("GITHUB_RUN_ID", "ambient-github")
			t.Setenv("FUNCTIONAL_SHORT", ambientShort)

			output, err := runScript(
				repoRoot,
				scriptPath,
				"FUNCTIONAL_EXPLICIT_SENTINEL=explicit",
			)
			if err != nil {
				t.Fatalf("run environment probe: %v\n%s", err, output)
			}

			for _, expected := range []string{
				"parent-only=unset",
				"artifact-root=unset",
				"github-run-id=unset",
				"short=unset",
				"explicit=explicit",
			} {
				if !strings.Contains(output, expected) {
					t.Fatalf("environment probe missing %q:\n%s", expected, output)
				}
			}
		})
	}
}
