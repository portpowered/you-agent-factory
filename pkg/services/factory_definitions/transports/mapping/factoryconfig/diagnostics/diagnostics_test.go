package diagnostics

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestTopologyFindingsMapsCanonicalRepresentation(t *testing.T) {
	t.Parallel()

	findings := TopologyFindings([]factorydefinitions.TopologyFinding{{
		Severity: factorydefinitions.ValidationSeverityWarning,
		Path:     "workstations[0].worker",
		Message:  "worker is deprecated",
		Rule:     "worker-deprecated",
	}})

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0] != (Finding{
		Severity: SeverityWarning,
		Path:     "workstations[0].worker",
		Message:  "worker is deprecated",
		Rule:     "worker-deprecated",
	}) {
		t.Fatalf("finding = %#v", findings[0])
	}
}

func TestFactoryDefinitionFindingsUsesSubjectWhenCanonicalPathIsEmpty(t *testing.T) {
	t.Parallel()

	findings := FactoryDefinitionFindings([]factorydefinitions.ValidationTarget{{
		Code:     "factory.reference",
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  "reference is invalid",
		Subject: factorydefinitions.ValidationSubject{
			ID: "worker-a",
		},
	}})

	if len(findings) != 1 || findings[0].Path != "worker-a" {
		t.Fatalf("findings = %#v, want subject-backed path", findings)
	}
}
