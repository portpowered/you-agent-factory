package factorydefinitions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const delDefResidualPrestartGateManifestRel = "docs/internal/processes/del-def-residual-prestart-gates.json"

type delDefResidualPrestartGateManifest struct {
	DeletionHoldActive   bool                       `json:"deletion_hold_active"`
	DeletionHoldReason   string                     `json:"deletion_hold_reason"`
	Gates                map[string]delDefResidualGate `json:"gates"`
}

type delDefResidualGate struct {
	FactoryComplete bool     `json:"factory_complete"`
	MergedPR        int      `json:"merged_pr"`
	OpenPR          int      `json:"open_pr"`
	ObservableProof []string `json:"observable_proof"`
	OverlappingDeletionLeasePaths []string `json:"overlapping_deletion_lease_paths"`
}

// DEL-DEF-RESIDUAL story 001 confirms residual fold prerequisites and the
// DEL-DEF serialization gate before leased residual deletion or baseline
// burn-down begins.

func TestDelDefResidualPrerequisiteGate_ResidualFoldPacketsFactoryComplete(t *testing.T) {
	t.Parallel()

	manifest := loadDelDefResidualPrestartGateManifest(t)
	root := delDefResidualRepoRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "factory_definitions")

	foldGates := []struct {
		gateID       string
		internalRel  string
	}{
		{"CLN-DEF-FOLD-CATALOG", "internal/services/catalog"},
		{"CLN-DEF-FOLD-COMPILATION", "internal/services/compilation"},
		{"CLN-DEF-FOLD-COMPOSITION", "internal/lifecycle"},
		{"CLN-DEF-FOLD-VALIDATION", "internal/services/validation"},
		{"CLN-DEF-FOLD-SNAPSHOTS", "internal/services/snapshots_portability"},
		{"CLN-DEF-FOLD-DISTRIBUTION", "internal/services/distribution"},
		{"CLN-DEF-FOLD-INVOCATION-POLICY", "internal/services/invocation_policy"},
	}

	for _, foldGate := range foldGates {
		foldGate := foldGate
		t.Run(foldGate.gateID, func(t *testing.T) {
			t.Parallel()
			gate, ok := manifest.Gates[foldGate.gateID]
			if !ok {
				t.Fatalf("manifest missing gate %s", foldGate.gateID)
			}
			if !gate.FactoryComplete {
				t.Fatalf("%s gate must be Factory-complete before residual deletion begins", foldGate.gateID)
			}
			if gate.MergedPR <= 0 {
				t.Fatalf("%s gate must record the merged PR number", foldGate.gateID)
			}
			internalPath := filepath.Join(serviceRoot, foldGate.internalRel)
			if _, err := os.Stat(internalPath); err != nil {
				t.Fatalf("%s observable proof missing: %s must exist after fold: %v", foldGate.gateID, internalPath, err)
			}
		})
	}
}

func TestDelDefResidualPrerequisiteGate_InvDefInvocationPolicyFactoryComplete(t *testing.T) {
	t.Parallel()

	manifest := loadDelDefResidualPrestartGateManifest(t)
	gate, ok := manifest.Gates["INV-DEF-INVOCATION-POLICY"]
	if !ok {
		t.Fatal("manifest missing INV-DEF-INVOCATION-POLICY gate")
	}
	if !gate.FactoryComplete {
		t.Fatal("INV-DEF-INVOCATION-POLICY gate must be Factory-complete before residual deletion begins")
	}
	if gate.MergedPR <= 0 {
		t.Fatal("INV-DEF-INVOCATION-POLICY gate must record the merged PR number")
	}

	root := delDefResidualRepoRoot(t)
	invocationPolicyRoot := filepath.Join(
		root,
		"pkg",
		"services",
		"factory_definitions",
		"internal",
		"services",
		"invocation_policy",
	)
	if _, err := os.Stat(invocationPolicyRoot); err != nil {
		t.Fatalf("internal/services/invocation_policy must exist after INV-DEF: %v", err)
	}
	wireProof := filepath.Join(
		root,
		"pkg",
		"services",
		"factory_definitions",
		"wire",
		"invocation_policy_test.go",
	)
	if _, err := os.Stat(wireProof); err != nil {
		t.Fatalf("wire/invocation_policy_test.go must exist as INV-DEF observable proof: %v", err)
	}
}

func TestDelDefResidualPrerequisiteGate_DelDefSerializationHoldActive(t *testing.T) {
	t.Parallel()

	manifest := loadDelDefResidualPrestartGateManifest(t)
	delDefGate, ok := manifest.Gates["DEL-DEF"]
	if !ok {
		t.Fatal("manifest missing DEL-DEF serialization gate")
	}

	if delDefGate.FactoryComplete {
		if manifest.DeletionHoldActive {
			t.Fatal("deletion_hold_active must be false once DEL-DEF is Factory-complete")
		}
		return
	}

	if !manifest.DeletionHoldActive {
		t.Fatal("deletion_hold_active must be true while DEL-DEF owns overlapping deletion leases")
	}
	if manifest.DeletionHoldReason == "" {
		t.Fatal("deletion_hold_reason must document why residual deletion is held")
	}
	if delDefGate.OpenPR <= 0 {
		t.Fatal("DEL-DEF gate must record the open PR number while in-flight")
	}
	if len(delDefGate.OverlappingDeletionLeasePaths) == 0 {
		t.Fatal("DEL-DEF gate must list overlapping deletion lease paths")
	}

	root := delDefResidualRepoRoot(t)
	for _, leasePath := range delDefGate.OverlappingDeletionLeasePaths {
		leasePath := leasePath
		t.Run(leasePath, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(root, leasePath)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf(
					"DEL-DEF still owns %s; path must remain present until DEL-DEF merges: %v",
					leasePath,
					err,
				)
			}
		})
	}
}

func TestDelDefResidualPrerequisiteGate_TransitionalResidualPackagesStillPresent(t *testing.T) {
	t.Parallel()

	manifest := loadDelDefResidualPrestartGateManifest(t)
	if !manifest.DeletionHoldActive {
		t.Skip("residual transitional packages may be deleted once deletion_hold_active is false")
	}

	root := delDefResidualRepoRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "factory_definitions")
	for _, relativeDir := range residualTransitionalPublicDirsHeldUntilDeletion() {
		relativeDir := relativeDir
		t.Run(relativeDir, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(serviceRoot, relativeDir)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf(
					"transitional residual public package %s must remain present while deletion_hold_active: %v",
					relativeDir,
					err,
				)
			}
		})
	}
}

func residualTransitionalPublicDirsHeldUntilDeletion() []string {
	return []string{
		"namedpaths",
		"namedfactories",
		"persistence",
		"resource",
		"loading",
		"loadedsource",
		"runtimeconfig",
		"validation",
		"workers",
		"namevalue",
		"portableconfig",
		"snapshotcapture",
		"editable",
		"replayconfig",
		"packages",
		"packagedinstallation",
		"decisionenvelope",
		"invocationinterpolation",
		"invocationoutput",
		"invocationworktype",
		"quorumpolicy",
		"workpropagation",
		"workstationexecution",
		"ttsobservability",
	}
}

func loadDelDefResidualPrestartGateManifest(t *testing.T) delDefResidualPrestartGateManifest {
	t.Helper()

	root := delDefResidualRepoRoot(t)
	path := filepath.Join(root, delDefResidualPrestartGateManifestRel)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pre-start gate manifest %s: %v", path, err)
	}
	var manifest delDefResidualPrestartGateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode pre-start gate manifest: %v", err)
	}
	return manifest
}

func delDefResidualRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	return root
}
