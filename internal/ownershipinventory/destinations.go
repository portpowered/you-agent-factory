package ownershipinventory

import (
	"slices"
	"strings"
)

// ProductOwners is the closed destination vocabulary. Chat Sessions, Events,
// and Worker Sessions are confirmed as distinct L1 ACP Core owners in
// docs/internal/projects/acp-program/README.md (D2, cross-lane contracts); this
// reconciles the committed tree with that already-settled decision rather than
// reopening owner discovery.
var ProductOwners = []string{
	"factory_definitions",
	"factory_sessions",
	"factory_runtime",
	"work",
	"workers",
	"providers",
	"provider_sessions",
	"models",
	"automations",
	"recordings",
	"factory_visualization",
	"operator_settings",
	"system_initialization",
	"chat_sessions",
	"events",
	"worker_sessions",
	"webhooks",
}

// ApprovedFamilies are retain destinations that are not product services.
var ApprovedFamilies = []string{
	"initializer",
	"root",
	"wire",
	"platform",
	"transports",
}

// ArchitectureExceptions is the closed exception vocabulary.
var ArchitectureExceptions = []string{
	DestinationEdges,
}

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

func closedDestinationSet() map[string]string {
	out := make(map[string]string, len(ProductOwners)+len(ApprovedFamilies)+len(ArchitectureExceptions)+1)
	for _, owner := range ProductOwners {
		out[owner] = DestinationKindOwner
	}
	for _, family := range ApprovedFamilies {
		out[family] = DestinationKindFamily
	}
	for _, exception := range ArchitectureExceptions {
		out[exception] = DestinationKindArchitectureException
	}
	out[DestinationDeletionQueue] = DestinationKindDeletionQueue
	return out
}

func isKnownDestination(destination string) bool {
	_, ok := closedDestinationSet()[destination]
	return ok
}

// OwnerForPackage derives the destination a production package belongs to from
// the repository tree layout alone: pkg/services/<owner>/... belongs to <owner>
// (or to the Process Edges architecture exception), and pkg/<family>/... belongs
// to that approved non-service family. Reporting false means the path sits
// outside the closed destination vocabulary and has no derivable owner.
//
// This is why the inventory no longer carries a "retain" row per package: a row
// that only restates the directory the package already lives in adds nothing
// this function cannot compute.
func OwnerForPackage(packagePath string) (string, bool) {
	const pkgPrefix = "pkg/"
	const servicesPrefix = "services/"

	remainder, ok := strings.CutPrefix(strings.TrimSpace(packagePath), pkgPrefix)
	if !ok || remainder == "" {
		return "", false
	}
	closed := closedDestinationSet()
	if serviceRemainder, isService := strings.CutPrefix(remainder, servicesPrefix); isService {
		owner := firstPathSegment(serviceRemainder)
		switch closed[owner] {
		case DestinationKindOwner, DestinationKindArchitectureException:
			return owner, true
		default:
			return "", false
		}
	}
	family := firstPathSegment(remainder)
	if closed[family] == DestinationKindFamily {
		return family, true
	}
	return "", false
}

func firstPathSegment(path string) string {
	if index := strings.Index(path, "/"); index >= 0 {
		return path[:index]
	}
	return path
}

func defaultDestinationVocabulary() DestinationVocabulary {
	return DestinationVocabulary{
		Owners:    slices.Clone(ProductOwners),
		Families:  slices.Clone(ApprovedFamilies),
		Exception: slices.Clone(ArchitectureExceptions),
	}
}

func defaultProcessEdgesException() ProcessEdgesException {
	return ProcessEdgesException{
		PackagePath: ProcessEdgesPackagePath,
		Destination: DestinationEdges,
		Kind:        DestinationKindArchitectureException,
		Note:        "Process Edges is the sole broad external-effect architecture exception for Packaged Service Structure; it is construction input for root.BuildProcess and functional-test overrides, not a product service.",
	}
}
