package restart_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestartArtifactSetupFailsClosedForRequiredMissingOrInvalidInput(t *testing.T) {
	t.Run("required path missing", func(t *testing.T) {
		identity, err := resolveRestartCLIArtifact("", true)
		if err == nil || !strings.Contains(err.Error(), restartArtifactEnvironment) {
			t.Fatalf("resolve missing required artifact = (%#v, %v), want actionable required-input error", identity, err)
		}
	})

	t.Run("directory path", func(t *testing.T) {
		t.Setenv(restartSourceHeadEnvironment, strings.Repeat("a", 40))
		directory := t.TempDir()
		identity, err := resolveRestartCLIArtifact(directory, true)
		if err == nil || identity != nil {
			t.Fatalf("resolve directory artifact = (%#v, %v), want failure before process setup", identity, err)
		}
	})

	t.Run("non-Go file", func(t *testing.T) {
		t.Setenv(restartSourceHeadEnvironment, strings.Repeat("a", 40))
		path := filepath.Join(t.TempDir(), "not-you.exe")
		if err := os.WriteFile(path, []byte("not a compiled CLI"), 0o600); err != nil {
			t.Fatalf("write invalid artifact fixture: %v", err)
		}
		identity, err := resolveRestartCLIArtifact(path, true)
		if err == nil || identity != nil {
			t.Fatalf("resolve non-Go artifact = (%#v, %v), want failure before process setup", identity, err)
		}
	})
}
