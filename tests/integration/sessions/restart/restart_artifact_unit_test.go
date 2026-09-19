package restart_test

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestRestartArtifactPlatformMismatchFailsPreflightBeforeFixtureOrProcessSetup(t *testing.T) {
	otherGOOS := "linux"
	if runtime.GOOS == otherGOOS {
		otherGOOS = "windows"
	}
	otherGOARCH := "amd64"
	if runtime.GOARCH == otherGOARCH {
		otherGOARCH = "arm64"
	}

	tests := []struct {
		name        string
		buildGOOS   string
		buildGOARCH string
	}{
		{name: "matching artifact", buildGOOS: runtime.GOOS, buildGOARCH: runtime.GOARCH},
		{name: "different operating system", buildGOOS: otherGOOS, buildGOARCH: runtime.GOARCH},
		{name: "different architecture", buildGOOS: runtime.GOOS, buildGOARCH: otherGOARCH},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			identity, err := resolveRestartCLIArtifactForTarget(
				"caller-built-cli",
				true,
				strings.Repeat("a", 40),
				runtime.GOOS,
				runtime.GOARCH,
				func(_, _ string) (restartCLIArtifactIdentity, error) {
					return restartCLIArtifactIdentity{BuildGOOS: test.buildGOOS, BuildGOARCH: test.buildGOARCH}, nil
				},
			)
			mismatch := test.buildGOOS != runtime.GOOS || test.buildGOARCH != runtime.GOARCH
			if mismatch && (err == nil || identity != nil || !strings.Contains(err.Error(), "incompatible with integration test host")) {
				t.Fatalf("resolve target %s/%s = (%#v, %v), want preflight incompatibility before setup", test.buildGOOS, test.buildGOARCH, identity, err)
			}
			if !mismatch && err != nil {
				t.Fatalf("resolve matching target %s/%s: %v", test.buildGOOS, test.buildGOARCH, err)
			}
			if !mismatch && identity == nil {
				t.Fatalf("resolve matching target %s/%s returned no artifact identity", test.buildGOOS, test.buildGOARCH)
			}
		})
	}
}
