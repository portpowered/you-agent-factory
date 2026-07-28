package factorydefinitions_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// TestWorkerWorkstationCompatibilityContract_AcceptsAndRejectsThroughServiceRoot
// proves worker/workstation taxonomy compatibility helpers remain reachable
// through only the Factory Definitions service root import path.
func TestWorkerWorkstationCompatibilityContract_AcceptsAndRejectsThroughServiceRoot(t *testing.T) {
	t.Parallel()

	if !factorydefinitions.CompatibleWorkerWorkstationBehavior(
		factorydefinitions.WorkerTypeInference,
		factorydefinitions.WorkstationTypeInference,
		factorydefinitions.WorkstationKindStandard,
	) {
		t.Fatal("inference worker with inference workstation must remain compatible")
	}
	if factorydefinitions.CompatibleWorkerWorkstationBehavior(
		factorydefinitions.WorkerTypeInference,
		factorydefinitions.WorkstationTypeAgent,
		factorydefinitions.WorkstationKindStandard,
	) {
		t.Fatal("inference worker with agent workstation must remain incompatible")
	}

	if !factorydefinitions.RequiresWorkerWorkstationBehaviorCompatibility(
		factorydefinitions.WorkstationTypeAgent,
		factorydefinitions.WorkstationKindStandard,
		"executor",
	) {
		t.Fatal("agent workstation must require worker compatibility checks")
	}
	if !factorydefinitions.ExemptFromWorkerWorkstationCompatibility(factorydefinitions.Workstation{
		Type: factorydefinitions.WorkstationTypeClassify,
	}) {
		t.Fatal("classify workstation must remain exempt from compatibility checks")
	}

	if !factorydefinitions.WorkerMatchesWorkstationBehavior(
		factorydefinitions.WorkerTypeScript,
		factorydefinitions.Workstation{
			Type: factorydefinitions.WorkstationTypeScript,
			Kind: factorydefinitions.WorkstationKindStandard,
		},
	) {
		t.Fatal("script worker with script workstation must remain compatible")
	}
}
