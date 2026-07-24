package ownershipinventory

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Validate loads the effective ownership inventory for root and checks it
// against production packages under pkg.
func Validate(root string) (Report, error) {
	inventory, reused, err := LoadEffective(root)
	if err != nil {
		return Report{}, err
	}
	packages, err := ListProductionPackages(root)
	if err != nil {
		return Report{}, err
	}
	discovered, err := DiscoverCrossServiceEdges(root, inventory.Packages)
	if err != nil {
		return Report{}, err
	}
	report := ValidateInventory(inventory, packages)
	validateCrossServiceEdgeCoverage(inventory, discovered, &report)
	report.ReusedFND01Seed = reused
	return report, nil
}

// ValidateInventory checks freeze properties against an explicit package list.
func ValidateInventory(inventory Inventory, packages []string) Report {
	report := Report{}
	if inventory.SortKey != SortKeyDescription {
		report.InvalidMappings = append(report.InvalidMappings, "sortKey must document packagePath ascending byte order")
	}
	if !slices.IsSorted(packagePaths(inventory.Packages)) {
		report.UnstableSort = true
	}

	seen := map[string]int{}
	for _, row := range inventory.Packages {
		seen[row.PackagePath]++
		if msg := validateRow(row); msg != "" {
			report.InvalidMappings = append(report.InvalidMappings, msg)
		}
	}
	for packagePath, count := range seen {
		if count > 1 {
			report.DuplicatePackages = append(report.DuplicatePackages, packagePath)
		}
	}
	slices.Sort(report.DuplicatePackages)
	slices.Sort(report.InvalidMappings)

	expected := map[string]struct{}{}
	for _, packagePath := range packages {
		expected[packagePath] = struct{}{}
		if seen[packagePath] == 0 {
			report.MissingPackages = append(report.MissingPackages, packagePath)
		}
	}
	slices.Sort(report.MissingPackages)

	for packagePath := range seen {
		if _, ok := expected[packagePath]; !ok {
			report.UnexpectedPackages = append(report.UnexpectedPackages, packagePath)
		}
	}
	slices.Sort(report.UnexpectedPackages)

	if !hasProcessEdgesException(inventory) {
		report.MissingProcessEdgesException = true
	}

	seedNames := map[string]struct{}{}
	for _, seed := range inventory.SeedServices {
		seedNames[seed.Name] = struct{}{}
		if !isKnownDestination(seed.Destination) || seed.Destination == DestinationDeletionQueue {
			report.InvalidMappings = append(report.InvalidMappings, fmt.Sprintf("seed service %q has invalid destination %q", seed.Name, seed.Destination))
		}
	}
	for _, seed := range StructuresSeedServices {
		if _, ok := seedNames[seed.Name]; !ok {
			report.MissingSeedServices = append(report.MissingSeedServices, seed.Name)
		}
	}
	slices.Sort(report.MissingSeedServices)

	rootSet := map[string]struct{}{}
	for _, root := range inventory.AdditionalCurrentRoots {
		rootSet[root] = struct{}{}
	}
	for _, root := range AdditionalCurrentRoots {
		if _, ok := rootSet[root]; !ok {
			report.MissingAdditionalRoots = append(report.MissingAdditionalRoots, root)
		}
	}
	slices.Sort(report.MissingAdditionalRoots)

	validateRationales(inventory, &report)
	validateResponsibilityClusters(inventory, &report)
	validateCrossServiceEdges(inventory, &report)
	validateNamedOwners(inventory, &report)

	return report
}

func validateNamedOwners(inventory Inventory, report *Report) {
	if !slices.IsSortedFunc(inventory.NamedOwnerConfirmations, compareNamedOwnerConfirmations) {
		report.UnstableNamedOwnerSort = true
	}

	byOwner := map[string]NamedOwnerConfirmation{}
	for _, confirmation := range inventory.NamedOwnerConfirmations {
		byOwner[confirmation.Owner] = confirmation
		if msg := validateNamedOwnerConfirmation(confirmation, inventory.Packages); msg != "" {
			report.InvalidNamedOwnerMaps = append(report.InvalidNamedOwnerMaps, msg)
		}
		if confirmation.Status != NamedOwnerStatusConfirmed ||
			strings.Contains(strings.ToLower(confirmation.Status), "discovery") ||
			strings.Contains(strings.ToLower(confirmation.Status), "decomposition") {
			report.UnconfirmedNamedOwners = append(report.UnconfirmedNamedOwners, confirmation.Owner)
		}
		if !slices.Contains(ProductOwners, confirmation.Owner) {
			report.InvalidNamedOwnerMaps = append(
				report.InvalidNamedOwnerMaps,
				confirmation.Owner+": introduces alternate top-level owner outside the committed 13-owner tree",
			)
		}
	}

	for _, owner := range RequiredNamedOwners {
		confirmation, ok := byOwner[owner]
		if !ok {
			report.MissingNamedOwners = append(report.MissingNamedOwners, owner)
			continue
		}
		wantNested := NamedOwnerNestedSubservices[owner]
		if !slices.Equal(confirmation.NestedSubservices, wantNested) {
			report.InvalidNamedOwnerMaps = append(
				report.InvalidNamedOwnerMaps,
				owner+": nested-subservice map does not match committed target tree",
			)
		}
		if strings.TrimSpace(confirmation.TargetPath) == "" {
			report.InvalidNamedOwnerMaps = append(report.InvalidNamedOwnerMaps, owner+": missing targetPath")
		}
	}

	slices.Sort(report.MissingNamedOwners)
	slices.Sort(report.UnconfirmedNamedOwners)
	slices.Sort(report.InvalidNamedOwnerMaps)
	report.InvalidNamedOwnerMaps = slices.Compact(report.InvalidNamedOwnerMaps)
}

func validateNamedOwnerConfirmation(confirmation NamedOwnerConfirmation, packages []PackageRow) string {
	if strings.TrimSpace(confirmation.Owner) == "" {
		return "named owner confirmation missing owner"
	}
	if strings.TrimSpace(confirmation.DisplayName) == "" {
		return confirmation.Owner + ": missing displayName"
	}
	if strings.TrimSpace(confirmation.TargetPath) == "" {
		return confirmation.Owner + ": missing targetPath"
	}
	if strings.TrimSpace(confirmation.Status) == "" {
		return confirmation.Owner + ": missing status"
	}
	if confirmation.NestedSubservices == nil {
		return confirmation.Owner + ": nestedSubservices must be present (use empty list when none)"
	}
	if !slices.IsSorted(confirmation.NestedSubservices) {
		return confirmation.Owner + ": nestedSubservices must be stable-sorted"
	}
	for _, serviceID := range confirmation.NestedSubservices {
		if !strings.HasPrefix(serviceID, confirmation.Owner+"/") {
			return confirmation.Owner + ": nested subservice " + serviceID + " is outside owner"
		}
	}
	if strings.TrimSpace(confirmation.Note) == "" {
		return confirmation.Owner + ": missing note"
	}
	for _, rule := range confirmation.ResidualPackageRules {
		if msg := validateResidualPackageRule(confirmation.Owner, rule, packages); msg != "" {
			return msg
		}
	}
	return ""
}

func validateResidualPackageRule(owner string, rule ResidualPackageRule, packages []PackageRow) string {
	if strings.TrimSpace(rule.PackagePrefix) == "" {
		return owner + ": residual rule missing packagePrefix"
	}
	if strings.TrimSpace(rule.Destination) == "" {
		return owner + ": residual rule " + rule.PackagePrefix + " missing destination"
	}
	if !isKnownDestination(rule.Destination) {
		return owner + ": residual rule " + rule.PackagePrefix + " destination outside closed vocabulary"
	}
	if rule.Disposition != DispositionRetain && rule.Disposition != DispositionMove && rule.Disposition != DispositionDelete {
		return owner + ": residual rule " + rule.PackagePrefix + " has unknown disposition"
	}
	if strings.TrimSpace(rule.Note) == "" {
		return owner + ": residual rule " + rule.PackagePrefix + " missing note"
	}

	matched := 0
	for _, row := range packages {
		if !packageMatchesResidualPrefix(row.PackagePath, rule.PackagePrefix) {
			continue
		}
		matched++
		if row.Destination != rule.Destination || row.Disposition != rule.Disposition {
			return owner + ": residual rule " + rule.PackagePrefix + " mismatches package " + row.PackagePath
		}
	}
	if matched == 0 {
		return owner + ": residual rule " + rule.PackagePrefix + " matches no inventoried packages"
	}
	return ""
}

func validateCrossServiceEdges(inventory Inventory, report *Report) {
	if len(inventory.CrossServiceEdges) == 0 {
		report.MissingCrossServiceEdgeTable = true
		return
	}
	if !slices.IsSortedFunc(inventory.CrossServiceEdges, compareCrossServiceEdges) {
		report.UnstableEdgeSort = true
	}
	for _, edge := range inventory.CrossServiceEdges {
		if msg := validateCrossServiceEdge(edge); msg != "" {
			report.InvalidEdgeClassifications = append(report.InvalidEdgeClassifications, msg)
		}
	}
	slices.Sort(report.InvalidEdgeClassifications)
}

func validateCrossServiceEdgeCoverage(inventory Inventory, discovered []CrossServiceEdge, report *Report) {
	// Incomplete fixture trees may contain package stubs without real imports.
	// Skip coverage reconciliation when discovery finds nothing.
	if len(discovered) == 0 {
		return
	}
	inventoried := map[string]CrossServiceEdge{}
	for _, edge := range inventory.CrossServiceEdges {
		inventoried[edgePairKey(edge.FromOwner, edge.ToOwner)] = edge
	}
	discoveredKeys := map[string]struct{}{}
	for _, edge := range discovered {
		key := edgePairKey(edge.FromOwner, edge.ToOwner)
		discoveredKeys[key] = struct{}{}
		if _, ok := inventoried[key]; !ok {
			report.MissingCrossServiceEdges = append(report.MissingCrossServiceEdges, key)
		}
	}
	for key := range inventoried {
		if _, ok := discoveredKeys[key]; !ok {
			report.UnexpectedCrossServiceEdges = append(report.UnexpectedCrossServiceEdges, key)
		}
	}
	slices.Sort(report.MissingCrossServiceEdges)
	slices.Sort(report.UnexpectedCrossServiceEdges)
}

func validateCrossServiceEdge(edge CrossServiceEdge) string {
	key := edgePairKey(edge.FromOwner, edge.ToOwner)
	if strings.TrimSpace(edge.FromOwner) == "" || strings.TrimSpace(edge.ToOwner) == "" {
		return key + ": missing fromOwner/toOwner"
	}
	if edge.FromOwner == edge.ToOwner {
		return key + ": edge is not cross-owner"
	}
	if strings.TrimSpace(edge.Class) == "" {
		return key + ": missing class"
	}
	if !isAllowedEdgeClass(edge.Class) {
		return key + ": unknown class " + strconv.Quote(edge.Class)
	}
	involvesProcessEdges := edge.FromOwner == DestinationEdges || edge.ToOwner == DestinationEdges
	if involvesProcessEdges {
		if !edge.ArchitectureException {
			return key + ": Process Edges edge must set architectureException"
		}
		if edge.Class != EdgeClassConstruction && edge.Class != EdgeClassExternalEffect {
			return key + ": Process Edges edge class must be construction or external_effect"
		}
	} else if edge.ArchitectureException {
		return key + ": architectureException reserved for Process Edges edges"
	}
	if strings.TrimSpace(edge.Evidence) == "" {
		return key + ": missing evidence"
	}
	return ""
}

func validateRationales(inventory Inventory, report *Report) {
	if !slices.IsSortedFunc(inventory.OwnerRationales, func(a, b OwnerRationaleCard) int {
		return strings.Compare(a.ServiceID, b.ServiceID)
	}) {
		report.UnstableRationaleSort = true
	}

	byID := map[string]OwnerRationaleCard{}
	for _, card := range inventory.OwnerRationales {
		byID[card.ServiceID] = card
		if msg := validateRationaleCard(card); msg != "" {
			report.InvalidRationaleFields = append(report.InvalidRationaleFields, msg)
		}
	}

	for _, owner := range ProductOwners {
		card, ok := byID[owner]
		if !ok || card.Kind != RationaleKindTopLevel || card.Owner != owner {
			report.MissingOwnerRationales = append(report.MissingOwnerRationales, owner)
		}
	}
	slices.Sort(report.MissingOwnerRationales)

	for _, serviceID := range CommittedNestedServiceIDs {
		card, ok := byID[serviceID]
		if !ok || card.Kind != RationaleKindNested {
			report.MissingNestedRationales = append(report.MissingNestedRationales, serviceID)
		}
	}
	slices.Sort(report.MissingNestedRationales)
	slices.Sort(report.InvalidRationaleFields)
}

func validateResponsibilityClusters(inventory Inventory, report *Report) {
	if !slices.IsSortedFunc(inventory.ResponsibilityClusters, func(a, b ResponsibilityCluster) int {
		if cmp := strings.Compare(a.Owner, b.Owner); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ClusterID, b.ClusterID)
	}) {
		report.UnstableResponsibilitySort = true
	}

	seen := map[string]ResponsibilityCluster{}
	for _, cluster := range inventory.ResponsibilityClusters {
		key := cluster.Owner + "/" + cluster.ClusterID
		seen[key] = cluster
		if strings.TrimSpace(cluster.Owner) == "" ||
			strings.TrimSpace(cluster.ClusterID) == "" ||
			strings.TrimSpace(cluster.Name) == "" ||
			strings.TrimSpace(cluster.Note) == "" {
			report.MissingResponsibilityClusters = append(report.MissingResponsibilityClusters, key)
		}
	}
	for _, key := range CommittedResponsibilityClusterIDs {
		if _, ok := seen[key]; !ok {
			report.MissingResponsibilityClusters = append(report.MissingResponsibilityClusters, key)
		}
	}
	slices.Sort(report.MissingResponsibilityClusters)
	report.MissingResponsibilityClusters = slices.Compact(report.MissingResponsibilityClusters)
}

func validateRationaleCard(card OwnerRationaleCard) string {
	if strings.TrimSpace(card.ServiceID) == "" {
		return "rationale card missing serviceId"
	}
	switch card.Kind {
	case RationaleKindTopLevel, RationaleKindNested:
	default:
		return card.ServiceID + ": unknown rationale kind " + strconv.Quote(card.Kind)
	}
	if strings.TrimSpace(card.Owner) == "" {
		return card.ServiceID + ": missing owner"
	}
	if strings.TrimSpace(card.TargetPath) == "" {
		return card.ServiceID + ": missing targetPath"
	}
	if card.Kind == RationaleKindNested && strings.TrimSpace(card.ParentServiceID) == "" {
		return card.ServiceID + ": nested rationale missing parentServiceId"
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"authority", card.Authority},
		{"stateStore", card.StateStore},
		{"lifecycle", card.Lifecycle},
		{"consumers", card.Consumers},
		{"transactionBoundary", card.TransactionBoundary},
		{"failureRecovery", card.FailureRecovery},
	} {
		if strings.TrimSpace(field.value) == "" {
			return card.ServiceID + ": missing " + field.name
		}
	}
	return ""
}

func packagePaths(rows []PackageRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.PackagePath
	}
	return out
}

func validateRow(row PackageRow) string {
	if row.PackagePath == "" {
		return "package row missing packagePath"
	}
	switch row.Disposition {
	case DispositionRetain, DispositionMove, DispositionDelete:
	default:
		return fmt.Sprintf("%s: unknown disposition %q", row.PackagePath, row.Disposition)
	}
	if row.Destination == "" {
		return fmt.Sprintf("%s: missing destination", row.PackagePath)
	}
	kind, ok := closedDestinationSet()[row.Destination]
	if !ok {
		return fmt.Sprintf("%s: destination %q outside closed vocabulary", row.PackagePath, row.Destination)
	}
	if row.DestinationKind != "" && row.DestinationKind != kind {
		return fmt.Sprintf("%s: destinationKind %q does not match destination %q", row.PackagePath, row.DestinationKind, row.Destination)
	}
	switch row.Disposition {
	case DispositionDelete:
		if row.Destination != DestinationDeletionQueue {
			return fmt.Sprintf("%s: delete disposition requires destination %q", row.PackagePath, DestinationDeletionQueue)
		}
		if strings.TrimSpace(row.Successor) == "" || strings.TrimSpace(row.DeletionCondition) == "" {
			return fmt.Sprintf("%s: deletion-queue mapping requires successor and deletionCondition", row.PackagePath)
		}
	case DispositionMove:
		if row.Destination == DestinationDeletionQueue {
			return fmt.Sprintf("%s: move disposition requires an owner/family/exception destination", row.PackagePath)
		}
		if strings.TrimSpace(row.Successor) == "" || strings.TrimSpace(row.DeletionCondition) == "" {
			return fmt.Sprintf("%s: move mapping requires successor and deletionCondition", row.PackagePath)
		}
	case DispositionRetain:
		if row.Destination == DestinationDeletionQueue {
			return fmt.Sprintf("%s: retain disposition cannot target deletion_queue", row.PackagePath)
		}
	}
	return ""
}

func hasProcessEdgesException(inventory Inventory) bool {
	exception := inventory.ProcessEdgesException
	if exception.PackagePath != ProcessEdgesPackagePath ||
		exception.Destination != DestinationEdges ||
		exception.Kind != DestinationKindArchitectureException ||
		strings.TrimSpace(exception.Note) == "" {
		return false
	}
	for _, row := range inventory.Packages {
		if row.PackagePath != ProcessEdgesPackagePath {
			continue
		}
		return row.Destination == DestinationEdges &&
			row.DestinationKind == DestinationKindArchitectureException &&
			row.Disposition == DispositionRetain
	}
	return false
}
