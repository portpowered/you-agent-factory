package ownershipinventory

import (
	"fmt"
	"strings"
)

// ownerMoveRuleMappings holds the per-owner authored migration rules. Only
// owners with unfinished migration intent appear here; every other owner needs
// no entry because retaining a package where it already sits is the default.
//
// This map is keyed by owner name but it is not a service roster: adding a
// service does not add an entry, so a new service never requires editing it.
// Owners are resolved by OwnerForPackage from the directory tree, and an owner
// absent from this map falls through to the derived retain default below.
var ownerMoveRuleMappings = map[string]func(string) (PackageRow, bool){
	"factory_definitions": factoryDefinitionsMapping,
	"factory_runtime":     factoryRuntimeMapping,
	"operator_settings":   operatorSettingsMapping,
	"recordings":          recordingsMapping,
	"work":                workMapping,
	"workers":             workersMapping,
}

// MapPackage returns the committed destination row for one production package.
// Explicit move/delete rules from the Packaged Service Structure plan override
// the retain default; the committed owner tree is not reopened here.
//
// The owner comes from OwnerForPackage, which reads it off the directory tree.
// There is deliberately no per-service switch: a package inside a service with
// no authored migration rules retains where it sits, so adding a service (or a
// package inside one) needs no edit here.
func MapPackage(packagePath string) (PackageRow, error) {
	if packagePath == "" {
		return PackageRow{}, fmt.Errorf("package path is empty")
	}
	if row, ok := explicitPackageMapping(packagePath); ok {
		return row, nil
	}
	owner, ok := OwnerForPackage(packagePath)
	if !ok {
		return PackageRow{}, fmt.Errorf("no committed destination for %s", packagePath)
	}
	if mapping, hasRules := ownerMoveRuleMappings[owner]; hasRules {
		row, mapped := mapping(packagePath)
		if !mapped {
			return PackageRow{}, fmt.Errorf("no committed destination for %s", packagePath)
		}
		return row, nil
	}
	return retainRow(packagePath, owner, KindForDerivedOwner(owner)), nil
}

func explicitPackageMapping(packagePath string) (PackageRow, bool) {
	switch {
	case
		packagePath == "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty" ||
			strings.HasPrefix(packagePath, "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty/"):
		return moveRow(
			packagePath,
			"providers",
			"pkg/services/providers",
			"delete Workers provider packages after Providers root cutover proof (IMP-providers-*)",
		), true
	case packagePath == "pkg/services/workers/services/hosted_logic" ||
		strings.HasPrefix(packagePath, "pkg/services/workers/services/hosted_logic/"):
		return moveRow(
			packagePath,
			"automations",
			"pkg/services/automations/internal/services/hosted_sources",
			"delete Workers hosted_logic after Automation Hosted Sources cutover proof (IMP-automations-hosted-sources)",
		), true
	default:
		if row, ok := legacyServiceImplementationMapping(packagePath); ok {
			return row, true
		}
		return PackageRow{}, false
	}
}

func legacyServiceImplementationMapping(packagePath string) (PackageRow, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	remainder := strings.TrimPrefix(packagePath, prefix)
	parts := strings.SplitN(remainder, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] != "service" {
		return PackageRow{}, false
	}
	owner := parts[0]
	return moveRow(
		packagePath,
		owner,
		"pkg/services/"+owner+"/internal",
		"delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
	), true
}

// DerivedPackageRow returns the implicit retain row for a package that carries
// no inventory row. Retaining a package where it already sits is the default,
// so the registry only records departures from it; callers that need a row for
// every live package reconstruct the default here instead of reading one back
// out of the registry.
func DerivedPackageRow(packagePath string) (PackageRow, bool) {
	owner, ok := OwnerForPackage(packagePath)
	if !ok {
		return PackageRow{}, false
	}
	return retainRow(packagePath, owner, KindForDerivedOwner(owner)), true
}

func retainRow(packagePath, destination, kind string) PackageRow {
	return PackageRow{
		PackagePath:     packagePath,
		Disposition:     DispositionRetain,
		Destination:     destination,
		DestinationKind: kind,
	}
}

func moveRow(packagePath, destination, successor, condition string) PackageRow {
	return PackageRow{
		PackagePath:       packagePath,
		Disposition:       DispositionMove,
		Destination:       destination,
		DestinationKind:   DestinationKindOwner,
		Successor:         successor,
		DeletionCondition: condition,
	}
}

// BuildInventory constructs the frozen ownership inventory for the provided
// production package list using FND-01 destination vocabulary and plan mappings.
//
// Package rows are not part of the artifact this builds. Open moves are
// authored migration intent, so they are read from the consolidated ledger
// rather than re-derived here; every other package is where it already belongs
// and OwnerForPackage computes that from the tree.
//
// Owner rationale, responsibility clusters, public-surface ownership, and owned
// roles are not part of it either: that is design prose nothing counts, and it
// is published at docs/architecture/service-ownership-rationale.md.
func BuildInventory(root string, packages []string) (Inventory, error) {
	// Every live package must still resolve to a committed destination, even
	// though the resolved row is no longer written down for the retain case.
	for _, packagePath := range packages {
		if _, err := MapPackage(packagePath); err != nil {
			return Inventory{}, err
		}
	}
	moves, err := LoadUnfinishedMoves(root)
	if err != nil {
		return Inventory{}, err
	}
	rows := moves.PackageRows()
	edges, err := DiscoverCrossServiceEdges(root, rows)
	if err != nil {
		return Inventory{}, err
	}
	vocabulary, err := DiscoverDestinationVocabulary(root)
	if err != nil {
		return Inventory{}, err
	}
	return Inventory{
		Version:                 1,
		Stage:                   "pss-f01-ownership-inventory",
		SortKey:                 SortKeyDescription,
		Destinations:            vocabulary,
		ProcessEdgesException:   defaultProcessEdgesException(),
		SeedServices:            append([]SeedService(nil), StructuresSeedServices...),
		AdditionalCurrentRoots:  append([]string(nil), AdditionalCurrentRoots...),
		CrossServiceEdges:       edges,
		NamedOwnerConfirmations: BuildNamedOwnerConfirmations(),
		MisplacedGuards:         BuildMisplacedGuards(),
		UnfinishedMoves:         moves,
		Packages:                rows,
	}, nil
}
