package factoryload_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
	configdiagnostics "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/diagnostics"
)

func TestBlockingFactoryLoadFindings_PreservesCanonicalTargets(t *testing.T) {
	err := blockingFactoryLoadTestError()
	loadErr, _ := interfaces.AsBlockingFactoryLoadError(err)
	findings := configdiagnostics.BlockingFactoryLoadFindings(err)
	if len(findings) != len(loadErr.Targets) {
		t.Fatalf("findings = %d, targets = %d, want equivalent coverage", len(findings), len(loadErr.Targets))
	}
}

func TestFactoryConfigValidateRecoveryCommand_UsesSingleValidatePath(t *testing.T) {
	factoryPath := filepath.Join("global-root", "@you", "goal")
	got := factoryload.ConfigValidateRecoveryCommand(factoryPath)
	normalized := strings.ReplaceAll(got, "\\", "/")
	if !strings.Contains(normalized, "@you/goal") {
		t.Fatalf("recovery command = %q, want @you/goal path", got)
	}
	if !strings.HasPrefix(got, "you factory config validate ") {
		t.Fatalf("recovery command = %q, want validate prefix", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("recovery command = %q, want exactly one validate command", got)
	}
}

func TestWrapBlockingFactoryLoadOperatorError_IncludesFindingsAndRecoveryCommand(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, "@you", "goal")
	err := blockingFactoryLoadTestError()

	wrapped := factoryload.WrapOperatorError(factoryPath, err)
	got := wrapped.Error()
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	recovery := factoryload.ConfigValidateRecoveryCommand(factoryPath)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", got)
	}
	if !errors.Is(wrapped, interfaces.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", wrapped)
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorError_IncludesRecoveryForOnDiskFactory(t *testing.T) {
	projectRoot := t.TempDir()
	factoryDir := filepath.Join(projectRoot, "@you", "goal")
	loadErr := blockingFactoryLoadTestError()

	wrapped := factoryload.MaybeFormatOperatorError(loadErr, factoryDir)
	got := wrapped.Error()
	recovery := factoryload.ConfigValidateRecoveryCommand(factoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
}

func TestFailureBaseline_InvalidTopology_MaterializeNamedFactoryFailureRetainsStructuredFindings(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, "@you", "goal")
	err := blockingFactoryLoadTestError()
	wrapped := factoryload.MaybeFormatOperatorError(err, factoryPath)
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, factoryPath)
}

func assertInvalidTopologyMaterializationOperatorDiagnostics(t *testing.T, err error, wantFactoryDir string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected invalid topology materialization or upgrade failure")
	}
	got := err.Error()
	if !strings.Contains(got, "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", got)
	}
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", got)
	}
	wantFactoryDir = strings.TrimSpace(wantFactoryDir)
	if wantFactoryDir == "" {
		t.Fatal("wantFactoryDir is required")
	}
	recovery := factoryload.ConfigValidateRecoveryCommand(wantFactoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	normalized := strings.ReplaceAll(got, "\\", "/")
	if !strings.Contains(normalized, "@you/goal") {
		t.Fatalf("error = %q, want resolved @you/goal factory path", got)
	}
}

func blockingFactoryLoadTestError() error {
	return interfaces.NewBlockingFactoryLoadError(interfaces.ValidationResult{
		Targets: []interfaces.ValidationTarget{{
			Code:     interfaces.ValidationCodeLayoutUnknownNodeReference,
			Severity: interfaces.ValidationSeverityError,
			Message:  "output state missing-plan-state is not declared by Work Type goal",
			Subject: interfaces.ValidationSubject{
				Type:     interfaces.ValidationSubjectTypeWorkstation,
				ID:       "plan-goal",
				Location: interfaces.ValidationSubjectLocationOutputs,
			},
			Path: "/workstations/plan-goal/outputs/0/state",
		}},
	})
}
