package ownershipinventory

import (
	"slices"
	"strings"
)

// AdditionalCurrentRoots appear in the committed ownership table outside the
// structures.md seed list.
var AdditionalCurrentRoots = []string{
	"pkg/initializer",
	"pkg/root",
	"pkg/wire",
	"pkg/platform",
	"pkg/transports",
	"pkg/services/providers",
	"pkg/services/worker_sessions",
	"pkg/services/operator_settings",
	"pkg/services/system_initialization",
	"pkg/services/factory_visualization",
	"pkg/services/costs",
	"pkg/services/webhooks",
	ProcessEdgesPackagePath,
}

// StructuresSeedServices are the structures.md TARGET seed services mapped onto
// the committed owner tree without reopening discovery. ACP Core owners that
// are not structures.md seeds are listed in AdditionalCurrentRoots.
var StructuresSeedServices = []SeedService{
	{Name: "Factory Definition", Source: "docs/architecture/structures.md", Destination: "factory_definitions"},
	{Name: "Factory Session", Source: "docs/architecture/structures.md", Destination: "factory_sessions"},
	{Name: "Factory Runtime", Source: "docs/architecture/structures.md", Destination: "factory_runtime"},
	{Name: "Work", Source: "docs/architecture/structures.md", Destination: "work"},
	{Name: "Worker Execution", Source: "docs/architecture/structures.md", Destination: "workers"},
	{Name: "Provider Session", Source: "docs/architecture/structures.md", Destination: "provider_sessions"},
	{Name: "Model Runtime", Source: "docs/architecture/structures.md", Destination: "models"},
	{Name: "Automation", Source: "docs/architecture/structures.md", Destination: "automations"},
	{Name: "Session Ledger and Projection", Source: "docs/architecture/structures.md", Destination: "recordings"},
}

// OwnerForPackage derives the destination a production package belongs to from
// the repository tree layout alone: pkg/services/<owner>/... belongs to <owner>
// (or to the Process Edges architecture exception), and pkg/<family>/... belongs
// to that approved non-service family. Reporting false means the path sits
// outside the derivable owner tree.
//
// For a path under pkg/services/ the directory segment IS the owner. No service
// roster is consulted, because for any path taken from the live tree the
// directory pkg/services/<owner> necessarily exists — checking the name against
// a list would only restate what the path already proves, and would reject a
// newly added service until a checker literal was edited to match.
//
// This is also why the inventory no longer carries a "retain" row per package: a
// row that only restates the directory the package already lives in adds nothing
// this function cannot compute.
func OwnerForPackage(packagePath string) (string, bool) {
	const pkgPrefix = "pkg/"
	const servicesPrefix = "services/"

	remainder, ok := strings.CutPrefix(strings.TrimSpace(packagePath), pkgPrefix)
	if !ok || remainder == "" {
		return "", false
	}
	if serviceRemainder, isService := strings.CutPrefix(remainder, servicesPrefix); isService {
		owner := firstPathSegment(serviceRemainder)
		if owner == "" {
			return "", false
		}
		if _, ignored := ignoredServiceDirectoryNames[owner]; ignored {
			return "", false
		}
		return owner, true
	}
	family := firstPathSegment(remainder)
	if slices.Contains(NonServiceFamilies, family) {
		return family, true
	}
	return "", false
}

// KindForDerivedOwner reports the destination kind for an owner derived by
// OwnerForPackage.
func KindForDerivedOwner(owner string) string {
	switch {
	case slices.Contains(architectureExceptionServices, owner):
		return DestinationKindArchitectureException
	case slices.Contains(NonServiceFamilies, owner):
		return DestinationKindFamily
	default:
		return DestinationKindOwner
	}
}

func firstPathSegment(path string) string {
	if index := strings.Index(path, "/"); index >= 0 {
		return path[:index]
	}
	return path
}

func defaultProcessEdgesException() ProcessEdgesException {
	return ProcessEdgesException{
		PackagePath: ProcessEdgesPackagePath,
		Destination: DestinationEdges,
		Kind:        DestinationKindArchitectureException,
		Note:        "Process Edges is the sole broad external-effect architecture exception for Packaged Service Structure; it is construction input for root.BuildProcess and functional-test overrides, not a product service.",
	}
}
