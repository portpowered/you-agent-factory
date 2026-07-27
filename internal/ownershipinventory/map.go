package ownershipinventory

import (
	"fmt"
	"strings"
)

// MapPackage returns the committed destination row for one production package.
// Explicit move/delete rules from the Packaged Service Structure plan override
// path-prefix retain defaults; the 13-owner tree is not reopened here.
func MapPackage(packagePath string) (PackageRow, error) {
	if packagePath == "" {
		return PackageRow{}, fmt.Errorf("package path is empty")
	}
	if row, ok := explicitPackageMapping(packagePath); ok {
		return row, nil
	}
	switch {
	case packagePath == ProcessEdgesPackagePath || strings.HasPrefix(packagePath, ProcessEdgesPackagePath+"/"):
		return retainRow(packagePath, DestinationEdges, DestinationKindArchitectureException), nil
	case strings.HasPrefix(packagePath, "pkg/services/factory_definitions"):
		return retainRow(packagePath, "factory_definitions", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/factory_sessions"):
		return retainRow(packagePath, "factory_sessions", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/factory_runtime"):
		return retainRow(packagePath, "factory_runtime", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/work/") || packagePath == "pkg/services/work":
		return retainRow(packagePath, "work", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/workers"):
		return retainRow(packagePath, "workers", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/providers"):
		return retainRow(packagePath, "providers", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/provider_sessions"):
		return retainRow(packagePath, "provider_sessions", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/models"):
		return retainRow(packagePath, "models", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/automations"):
		return retainRow(packagePath, "automations", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/recordings"):
		return retainRow(packagePath, "recordings", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/factory_visualization"):
		return retainRow(packagePath, "factory_visualization", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/operator_settings"):
		return retainRow(packagePath, "operator_settings", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/services/system_initialization"):
		return retainRow(packagePath, "system_initialization", DestinationKindOwner), nil
	case strings.HasPrefix(packagePath, "pkg/initializer"):
		return retainRow(packagePath, "initializer", DestinationKindFamily), nil
	case packagePath == "pkg/root" || strings.HasPrefix(packagePath, "pkg/root/"):
		return retainRow(packagePath, "root", DestinationKindFamily), nil
	case strings.HasPrefix(packagePath, "pkg/wire"):
		return retainRow(packagePath, "wire", DestinationKindFamily), nil
	case strings.HasPrefix(packagePath, "pkg/platform"):
		return retainRow(packagePath, "platform", DestinationKindFamily), nil
	case strings.HasPrefix(packagePath, "pkg/transports"):
		return retainRow(packagePath, "transports", DestinationKindFamily), nil
	default:
		return PackageRow{}, fmt.Errorf("no committed destination for %s", packagePath)
	}
}

func explicitPackageMapping(packagePath string) (PackageRow, bool) {
	switch {
	case packagePath == "pkg/services/workers/cliprovider" ||
		strings.HasPrefix(packagePath, "pkg/services/workers/cliprovider/") ||
		packagePath == "pkg/services/workers/provider" ||
		strings.HasPrefix(packagePath, "pkg/services/workers/provider/") ||
		packagePath == "pkg/services/workers/provider_test" ||
		strings.HasPrefix(packagePath, "pkg/services/workers/provider_test/") ||
		packagePath == "pkg/services/workers/agypty" ||
		strings.HasPrefix(packagePath, "pkg/services/workers/agypty/"):
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
		return PackageRow{}, false
	}
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
func BuildInventory(root string, packages []string) (Inventory, error) {
	rows := make([]PackageRow, 0, len(packages))
	for _, packagePath := range packages {
		row, err := MapPackage(packagePath)
		if err != nil {
			return Inventory{}, err
		}
		rows = append(rows, row)
	}
	edges, err := DiscoverCrossServiceEdges(root, rows)
	if err != nil {
		return Inventory{}, err
	}
	return Inventory{
		Version:                1,
		Stage:                  "pss-f01-ownership-inventory",
		SortKey:                SortKeyDescription,
		FND01SeedPath:          FND01SeedRelativePath,
		Destinations:           defaultDestinationVocabulary(),
		ProcessEdgesException:  defaultProcessEdgesException(),
		SeedServices:           append([]SeedService(nil), StructuresSeedServices...),
		AdditionalCurrentRoots: append([]string(nil), AdditionalCurrentRoots...),
		OwnerRationales:         BuildOwnerRationales(),
		ResponsibilityClusters:  BuildResponsibilityClusters(),
		CrossServiceEdges:       edges,
		NamedOwnerConfirmations: BuildNamedOwnerConfirmations(),
		MisplacedGuards:         BuildMisplacedGuards(),
		PublicSurfaces:          BuildPublicSurfaces(),
		OwnedRoles:              BuildOwnedRoles(),
		Packages:                rows,
	}, nil
}
