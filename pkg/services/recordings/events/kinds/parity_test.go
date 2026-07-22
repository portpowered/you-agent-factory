package factoryeventkinds

import (
	"os"
	"strings"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateFactoryEventKindParity_CurrentInventoryMatchesOpenAPI(t *testing.T) {
	rawMapping, _, enumValues := loadBundledFactoryEventDiscriminatorContract(t)
	mapping, err := ParseFactoryEventTypePayloadMapping(rawMapping)
	if err != nil {
		t.Fatalf("parse factory event type payload mapping: %v", err)
	}

	openAPIMappingKinds := make([]factorycontracts.FactoryEventType, 0, len(mapping))
	for _, entry := range mapping {
		openAPIMappingKinds = append(openAPIMappingKinds, entry.EventType)
	}

	err = ValidateFactoryEventKindParity(FactoryEventKindParityInput{
		RuntimeKinds:        PublicEmittableFactoryEventKinds(),
		ContractOnlyKinds:   ContractOnlyFactoryEventKinds(),
		OpenAPIMappingKinds: openAPIMappingKinds,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(enumValues) != len(openAPIMappingKinds) {
		t.Fatalf("FactoryEventType enum length = %d, mapping length = %d", len(enumValues), len(openAPIMappingKinds))
	}
}

func TestValidateBundledFactoryEventKindParity_ReportsZeroDrift(t *testing.T) {
	openAPIPath := bundledOpenAPIPath(t)
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read bundled openapi contract %s: %v", openAPIPath, err)
	}

	if err := ValidateBundledFactoryEventKindParity(data); err != nil {
		t.Fatal(err)
	}

	input, err := LoadFactoryEventKindParityInputFromOpenAPIYAML(data)
	if err != nil {
		t.Fatalf("load parity input: %v", err)
	}
	drift := CompareFactoryEventKindParity(input)
	if len(drift.RuntimeOnlyKinds) != 0 {
		t.Fatalf("runtime-only drift = %#v, want none after gap closure", drift.RuntimeOnlyKinds)
	}
	if len(drift.ContractOnlyKinds) != 0 {
		t.Fatalf("contract-only drift = %#v, want none after gap closure", drift.ContractOnlyKinds)
	}
}

func TestCompareFactoryEventKindParity_NamesRuntimeOnlyKind(t *testing.T) {
	drift := CompareFactoryEventKindParity(FactoryEventKindParityInput{
		RuntimeKinds: []PublicEmittableKind{
			{Kind: factorycontracts.FactoryEventTypeRunRequest, EmissionEvidence: "test"},
			{Kind: factorycontracts.FactoryEventTypeWorkRequest, EmissionEvidence: "test"},
		},
		OpenAPIMappingKinds: []factorycontracts.FactoryEventType{
			factorycontracts.FactoryEventTypeRunRequest,
		},
	})

	if len(drift.RuntimeOnlyKinds) != 1 || drift.RuntimeOnlyKinds[0] != factorycontracts.FactoryEventTypeWorkRequest {
		t.Fatalf("runtime-only drift = %#v, want [WORK_REQUEST]", drift.RuntimeOnlyKinds)
	}
	if len(drift.ContractOnlyKinds) != 0 {
		t.Fatalf("contract-only drift = %#v, want none", drift.ContractOnlyKinds)
	}

	err := drift.Error()
	if !strings.Contains(err, "runtime-only factory event kinds") || !strings.Contains(err, "WORK_REQUEST") {
		t.Fatalf("drift error = %q, want runtime-only WORK_REQUEST naming", err)
	}
}

func TestCompareFactoryEventKindParity_NamesContractOnlyKind(t *testing.T) {
	drift := CompareFactoryEventKindParity(FactoryEventKindParityInput{
		RuntimeKinds: []PublicEmittableKind{
			{Kind: factorycontracts.FactoryEventTypeRunRequest, EmissionEvidence: "test"},
		},
		OpenAPIMappingKinds: []factorycontracts.FactoryEventType{
			factorycontracts.FactoryEventTypeRunRequest,
			factorycontracts.FactoryEventTypeFactoryChange,
		},
	})

	if len(drift.ContractOnlyKinds) != 1 || drift.ContractOnlyKinds[0] != factorycontracts.FactoryEventTypeFactoryChange {
		t.Fatalf("contract-only drift = %#v, want [FACTORY_CHANGE]", drift.ContractOnlyKinds)
	}

	err := drift.Error()
	if !strings.Contains(err, "contract-only factory event kinds") || !strings.Contains(err, "FACTORY_CHANGE") {
		t.Fatalf("drift error = %q, want contract-only FACTORY_CHANGE naming", err)
	}
}

func TestCompareFactoryEventKindParity_ClassifiedContractOnlyKindsAreNotUnexplainedDrift(t *testing.T) {
	drift := CompareFactoryEventKindParity(FactoryEventKindParityInput{
		RuntimeKinds: PublicEmittableFactoryEventKinds(),
		ContractOnlyKinds: []ContractOnlyKind{
			{
				Kind:     factorycontracts.FactoryEventTypeJavaScriptPhaseChange,
				Evidence: "classified for test",
			},
		},
		OpenAPIMappingKinds: []factorycontracts.FactoryEventType{
			factorycontracts.FactoryEventTypeRunRequest,
			factorycontracts.FactoryEventTypeJavaScriptPhaseChange,
		},
	})

	if len(drift.ContractOnlyKinds) != 0 {
		t.Fatalf("classified contract-only drift = %#v, want none", drift.ContractOnlyKinds)
	}
}

func TestContractOnlyFactoryEventKinds_HasEvidenceForEveryEntry(t *testing.T) {
	contractOnly := ContractOnlyFactoryEventKinds()
	seen := make(map[factorycontracts.FactoryEventType]struct{}, len(contractOnly))
	for _, entry := range contractOnly {
		if strings.TrimSpace(string(entry.Kind)) == "" {
			t.Fatal("contract-only inventory entry missing kind")
		}
		if strings.TrimSpace(entry.Evidence) == "" {
			t.Fatalf("contract-only kind %q missing evidence", entry.Kind)
		}
		if _, ok := seen[entry.Kind]; ok {
			t.Fatalf("duplicate contract-only inventory kind %q", entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
	}
}

func TestExcludedAndContractOnlyKinds_AreNotSilentOmissionsInParity(t *testing.T) {
	_, _, enumValues := loadBundledFactoryEventDiscriminatorContract(t)
	enumSet := make(map[factorycontracts.FactoryEventType]struct{}, len(enumValues))
	for _, eventType := range enumValues {
		enumSet[eventType] = struct{}{}
	}

	runtimeSet := make(map[factorycontracts.FactoryEventType]struct{}, len(PublicEmittableFactoryEventKinds()))
	for _, entry := range PublicEmittableFactoryEventKinds() {
		runtimeSet[entry.Kind] = struct{}{}
	}

	contractOnlySet := make(map[factorycontracts.FactoryEventType]struct{}, len(ContractOnlyFactoryEventKinds()))
	for _, entry := range ContractOnlyFactoryEventKinds() {
		contractOnlySet[entry.Kind] = struct{}{}
		if _, ok := enumSet[entry.Kind]; !ok {
			t.Fatalf("classified contract-only kind %q is missing from FactoryEventType enum", entry.Kind)
		}
		if _, ok := runtimeSet[entry.Kind]; ok {
			t.Fatalf("classified contract-only kind %q is also in the runtime public inventory", entry.Kind)
		}
	}

	for _, entry := range ExcludedNonPublicFactoryEventKinds() {
		if entry.Category != "retired-factory-event-vocabulary" {
			continue
		}
		retiredKind := factorycontracts.FactoryEventType(entry.Name)
		if _, ok := enumSet[retiredKind]; ok {
			t.Fatalf("retired excluded kind %q still appears in FactoryEventType enum", entry.Name)
		}
		if _, ok := runtimeSet[retiredKind]; ok {
			t.Fatalf("retired excluded kind %q appears in runtime public inventory", entry.Name)
		}
	}

	for eventType := range enumSet {
		if _, inRuntime := runtimeSet[eventType]; inRuntime {
			continue
		}
		if _, classified := contractOnlySet[eventType]; classified {
			continue
		}
		t.Fatalf("FactoryEventType %q is neither runtime-emittable nor classified contract-only", eventType)
	}
}
