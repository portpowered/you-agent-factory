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
	report := ValidateInventory(inventory, packages)
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

	return report
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
