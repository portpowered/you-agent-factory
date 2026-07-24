package ownershipinventory

import "slices"

// ProductOwners is the closed 13-owner destination vocabulary.
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
	"pkg/services/operator_settings",
	"pkg/services/system_initialization",
	"pkg/services/factory_visualization",
	ProcessEdgesPackagePath,
}

// StructuresSeedServices are the structures.md TARGET seed services mapped onto
// the committed 13-owner tree without reopening discovery.
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
