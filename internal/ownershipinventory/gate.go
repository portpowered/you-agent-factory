package ownershipinventory

import "fmt"

// VerificationGateReport is the combined ownership-inventory and path-lease
// freeze gate used by PSS-F01 story 007. Explicit proof flags surface the
// acceptance properties reviewers need without inspecting nested reports.
type VerificationGateReport struct {
	Completeness               bool
	StableSortOrder            bool
	RequiredRationaleFields    bool
	EdgeClassifications        bool
	NamedOwnerCoverage         bool
	ProcessEdgesException      bool
	NonOverlappingActiveLeases bool
	InventoryOK                bool
	PathLeaseOK                bool
	Inventory                  Report
	PathLease                  PathLeaseFreezeReport
}

// OK reports whether every required freeze property was proved.
func (r VerificationGateReport) OK() bool {
	return r.Completeness &&
		r.StableSortOrder &&
		r.RequiredRationaleFields &&
		r.EdgeClassifications &&
		r.NamedOwnerCoverage &&
		r.ProcessEdgesException &&
		r.NonOverlappingActiveLeases &&
		r.InventoryOK &&
		r.PathLeaseOK
}

// VerifyFreeze loads the frozen ownership inventory and path-lease freeze for
// root and returns the combined verification-gate report.
func VerifyFreeze(root string) (VerificationGateReport, error) {
	inventoryReport, err := Validate(root)
	if err != nil {
		return VerificationGateReport{}, err
	}
	freeze, err := LoadPathLeaseFreeze(root)
	if err != nil {
		return VerificationGateReport{}, fmt.Errorf("load path-lease freeze: %w", err)
	}
	return composeVerificationGate(inventoryReport, ValidatePathLeaseFreeze(freeze)), nil
}

// VerifyFreezeArtifacts evaluates pre-loaded freeze artifacts against an
// explicit production package list. Edge-coverage reconciliation is skipped;
// use VerifyFreeze for full repository-root gating.
func VerifyFreezeArtifacts(inventory Inventory, freeze PathLeaseFreeze, packages []string) VerificationGateReport {
	return composeVerificationGate(ValidateInventory(inventory, packages), ValidatePathLeaseFreeze(freeze))
}

func composeVerificationGate(inventory Report, pathLease PathLeaseFreezeReport) VerificationGateReport {
	return VerificationGateReport{
		Completeness: completenessProved(inventory),
		StableSortOrder: !inventory.UnstableSort &&
			!inventory.UnstableRationaleSort &&
			!inventory.UnstableResponsibilitySort &&
			!inventory.UnstableEdgeSort &&
			!inventory.UnstableNamedOwnerSort &&
			!inventory.UnstableMisplacedGuardSort &&
			!inventory.UnstablePublicSurfaceSort &&
			!inventory.UnstableOwnedRoleSort &&
			!pathLease.UnstablePacketSort,
		RequiredRationaleFields: len(inventory.MissingOwnerRationales) == 0 &&
			len(inventory.MissingNestedRationales) == 0 &&
			len(inventory.InvalidRationaleFields) == 0,
		EdgeClassifications: !inventory.MissingCrossServiceEdgeTable &&
			len(inventory.MissingCrossServiceEdges) == 0 &&
			len(inventory.UnexpectedCrossServiceEdges) == 0 &&
			len(inventory.InvalidEdgeClassifications) == 0,
		NamedOwnerCoverage: len(inventory.MissingNamedOwners) == 0 &&
			len(inventory.UnconfirmedNamedOwners) == 0 &&
			len(inventory.InvalidNamedOwnerMaps) == 0,
		ProcessEdgesException:      !inventory.MissingProcessEdgesException,
		NonOverlappingActiveLeases: !pathLease.OverlappingActiveLeases && pathLease.ManifestError == "",
		InventoryOK:                inventory.OK(),
		PathLeaseOK:                pathLease.OK(),
		Inventory:                  inventory,
		PathLease:                  pathLease,
	}
}

func completenessProved(inventory Report) bool {
	return len(inventory.MissingPackages) == 0 &&
		len(inventory.UnexpectedPackages) == 0 &&
		len(inventory.DuplicatePackages) == 0 &&
		len(inventory.InvalidMappings) == 0 &&
		len(inventory.MissingSeedServices) == 0 &&
		len(inventory.MissingAdditionalRoots) == 0 &&
		len(inventory.MissingResponsibilityClusters) == 0 &&
		len(inventory.MissingMisplacedGuards) == 0 &&
		len(inventory.InvalidMisplacedGuards) == 0 &&
		len(inventory.MissingPublicSurfaces) == 0 &&
		len(inventory.InvalidPublicSurfaces) == 0 &&
		len(inventory.MissingOwnedRoles) == 0 &&
		len(inventory.InvalidOwnedRoles) == 0
}
