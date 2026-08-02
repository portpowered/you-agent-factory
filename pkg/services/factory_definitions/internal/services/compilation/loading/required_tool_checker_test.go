package loading

import (
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPathRequiredToolCheckerReportsMissingCommand(t *testing.T) {
	t.Parallel()

	const command = "portos-required-tool-that-does-not-exist"
	checker, err := NewPathRequiredToolChecker(
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("construct checker: %v", err)
	}
	result := checker.Check(
		factorydefinitions.RequiredToolConfig{
			Name:    "missing helper",
			Command: command,
		},
	)

	if result.FailureKind != factorydefinitions.RequiredToolFailureKindMissing {
		t.Fatalf("failure kind = %q, want %q", result.FailureKind, factorydefinitions.RequiredToolFailureKindMissing)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), command) {
		t.Fatalf("error = %v, want missing command context", result.Err)
	}
}

func TestPathRequiredToolCheckerResolvesToolAndRunsVersionProbe(t *testing.T) {
	t.Parallel()

	var probedPath string
	var probedArgs []string
	checker, err := NewPathRequiredToolChecker(
		func(command string) (string, error) { return "/tools/" + command, nil },
		func(path string, args ...string) ([]byte, error) {
			probedPath = path
			probedArgs = append([]string(nil), args...)
			return []byte("tool version 1"), nil
		},
	)
	if err != nil {
		t.Fatalf("construct checker: %v", err)
	}
	result := checker.Check(
		factorydefinitions.RequiredToolConfig{
			Name:        "toolchain",
			Command:     "tool",
			VersionArgs: []string{"version"},
		},
	)

	if result.Err != nil {
		t.Fatalf("check toolchain: %v", result.Err)
	}
	if result.ResolvedPath != "/tools/tool" {
		t.Fatalf("resolved path = %q, want /tools/tool", result.ResolvedPath)
	}
	if probedPath != result.ResolvedPath || len(probedArgs) != 1 || probedArgs[0] != "version" {
		t.Fatalf("version probe = %q %#v", probedPath, probedArgs)
	}
}

func TestNewPathRequiredToolCheckerRejectsMissingEffects(t *testing.T) {
	t.Parallel()

	probe := RequiredToolVersionProbe(
		func(string, ...string) ([]byte, error) { return nil, nil },
	)
	lookup := RequiredToolPathLookup(
		func(string) (string, error) { return "", nil },
	)
	if _, err := NewPathRequiredToolChecker(nil, probe); err == nil {
		t.Fatal("expected missing path lookup to be rejected")
	}
	if _, err := NewPathRequiredToolChecker(lookup, nil); err == nil {
		t.Fatal("expected missing version probe to be rejected")
	}
}

func TestPathRequiredToolCheckerReportsVersionProbeOutput(t *testing.T) {
	t.Parallel()

	checker, err := NewPathRequiredToolChecker(
		func(string) (string, error) { return "/tools/tool", nil },
		func(string, ...string) ([]byte, error) {
			return []byte("unsupported option\n"), errors.New("exit status 2")
		},
	)
	if err != nil {
		t.Fatalf("construct checker: %v", err)
	}
	result := checker.Check(factorydefinitions.RequiredToolConfig{
		Name:        "Tool",
		Command:     "tool",
		VersionArgs: []string{"--version"},
	})
	if result.FailureKind != factorydefinitions.RequiredToolFailureKindVersionProbe {
		t.Fatalf("failure kind = %q, want version probe", result.FailureKind)
	}
	if result.ResolvedPath != "/tools/tool" || result.Err == nil ||
		!strings.Contains(result.Err.Error(), "unsupported option") ||
		!strings.Contains(result.Err.Error(), "exit status 2") {
		t.Fatalf("version failure = (%q, %v), want resolved path and probe context", result.ResolvedPath, result.Err)
	}
}
