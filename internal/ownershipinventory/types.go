// Package ownershipinventory freezes Packaged Service Structure package
// destinations for PSS-F01 and validates that freeze against production pkg.
package ownershipinventory

const (
	InventoryRelativePath = "docs/internal/baselines/ownership-inventory.json"
	FND01SeedRelativePath = "docs/internal/baselines/package-target-manifest.json"

	SortKeyDescription = "packagePath ascending byte order (path slash-separated)"

	DispositionRetain = "retain"
	DispositionMove   = "move"
	DispositionDelete = "delete"

	DestinationKindOwner                 = "owner"
	DestinationKindFamily                = "family"
	DestinationKindArchitectureException = "architecture_exception"
	DestinationKindDeletionQueue         = "deletion_queue"

	DestinationDeletionQueue = "deletion_queue"
	DestinationEdges         = "edges"

	ProcessEdgesPackagePath = "pkg/services/edges"

	RationaleKindTopLevel = "top_level"
	RationaleKindNested   = "nested"

	OwnerRationaleSortKeyDescription         = "serviceID ascending byte order"
	ResponsibilityClusterSortKeyDescription  = "owner then clusterID ascending byte order"
	CrossServiceEdgeSortKeyDescription       = "fromOwner then toOwner ascending byte order"
	NamedOwnerSortKeyDescription             = "owner ascending byte order"

	NamedOwnerStatusConfirmed = "confirmed"

	EdgeClassCommand              = "command"
	EdgeClassQuery                = "query"
	EdgeClassEvent                = "event"
	EdgeClassProtocolComposition  = "protocol_composition"
	EdgeClassConstruction         = "construction"
	EdgeClassLifecycle            = "lifecycle"
	EdgeClassExternalEffect       = "external_effect"
)

// AllowedEdgeClasses is the closed cross-service edge classification set.
var AllowedEdgeClasses = []string{
	EdgeClassCommand,
	EdgeClassQuery,
	EdgeClassEvent,
	EdgeClassProtocolComposition,
	EdgeClassConstruction,
	EdgeClassLifecycle,
	EdgeClassExternalEffect,
}

// Inventory is the frozen PSS-F01 ownership inventory artifact.
type Inventory struct {
	Version                  int                     `json:"version"`
	Stage                    string                  `json:"stage"`
	SortKey                  string                  `json:"sortKey"`
	FND01SeedPath            string                  `json:"fnd01SeedPath"`
	Destinations             DestinationVocabulary   `json:"destinations"`
	ProcessEdgesException    ProcessEdgesException   `json:"processEdgesException"`
	SeedServices             []SeedService           `json:"seedServices"`
	AdditionalCurrentRoots   []string                `json:"additionalCurrentRoots"`
	OwnerRationales          []OwnerRationaleCard    `json:"ownerRationales"`
	ResponsibilityClusters   []ResponsibilityCluster `json:"responsibilityClusters"`
	CrossServiceEdges        []CrossServiceEdge      `json:"crossServiceEdges"`
	NamedOwnerConfirmations  []NamedOwnerConfirmation `json:"namedOwnerConfirmations"`
	Packages                 []PackageRow            `json:"packages"`
}

// NamedOwnerConfirmation freezes one PRD-named owner onto the committed tree
// with its reviewed nested-subservice map so PSS-F02 does not reopen discovery.
type NamedOwnerConfirmation struct {
	Owner                 string                `json:"owner"`
	DisplayName           string                `json:"displayName"`
	TargetPath            string                `json:"targetPath"`
	Status                string                `json:"status"`
	NestedSubservices     []string              `json:"nestedSubservices"`
	ResidualPackageRules  []ResidualPackageRule `json:"residualPackageRules,omitempty"`
	Note                  string                `json:"note"`
}

// ResidualPackageRule records how packages that currently sit near a named
// owner map to the closest committed destination or deletion queue.
type ResidualPackageRule struct {
	PackagePrefix string `json:"packagePrefix"`
	Destination   string `json:"destination"`
	Disposition   string `json:"disposition"`
	Note          string `json:"note"`
}

// CrossServiceEdge records one distinct-owner production dependency edge and
// its Packaged Service Structure interaction class.
type CrossServiceEdge struct {
	FromOwner              string `json:"fromOwner"`
	ToOwner                string `json:"toOwner"`
	Class                  string `json:"class"`
	ArchitectureException  bool   `json:"architectureException,omitempty"`
	Evidence               string `json:"evidence"`
}

// OwnerRationaleCard records authority, state, lifecycle, consumers,
// transaction, and failure rationale for one committed top-level or nested
// service from the Packaged Service Structure plan target tree.
type OwnerRationaleCard struct {
	ServiceID            string `json:"serviceId"`
	Owner                string `json:"owner"`
	Kind                 string `json:"kind"`
	ParentServiceID      string `json:"parentServiceId,omitempty"`
	TargetPath           string `json:"targetPath"`
	Authority            string `json:"authority"`
	StateStore           string `json:"stateStore"`
	Lifecycle            string `json:"lifecycle"`
	Consumers            string `json:"consumers"`
	TransactionBoundary  string `json:"transactionBoundary"`
	FailureRecovery      string `json:"failureRecovery"`
}

// ResponsibilityCluster records a large non-subservice responsibility cluster
// that remains under a committed owner without becoming its own nested service.
type ResponsibilityCluster struct {
	Owner     string `json:"owner"`
	ClusterID string `json:"clusterId"`
	Name      string `json:"name"`
	Note      string `json:"note"`
}

// PackageTargetManifest is the FND-01 package-to-target/deletion seed shape.
// When present on disk, PSS-F01 reuses its package rows instead of inventing a
// second destination catalog.
type PackageTargetManifest struct {
	Version  int          `json:"version"`
	Stage    string       `json:"stage"`
	SortKey  string       `json:"sortKey"`
	Packages []PackageRow `json:"packages"`
}

// DestinationVocabulary is the closed destination set for inventory rows.
type DestinationVocabulary struct {
	Owners    []string `json:"owners"`
	Families  []string `json:"families"`
	Exception []string `json:"exception"`
}

// ProcessEdgesException records pkg/services/edges as the sole broad
// external-effect architecture exception (not a product service).
type ProcessEdgesException struct {
	PackagePath string `json:"packagePath"`
	Destination string `json:"destination"`
	Kind        string `json:"kind"`
	Note        string `json:"note"`
}

// SeedService records a structures.md seed logical service and its committed
// destination in the 13-owner tree.
type SeedService struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// PackageRow maps one production package path to exactly one destination or
// deletion-queue entry.
type PackageRow struct {
	PackagePath       string `json:"packagePath"`
	Disposition       string `json:"disposition"`
	Destination       string `json:"destination"`
	DestinationKind   string `json:"destinationKind"`
	Successor         string `json:"successor,omitempty"`
	DeletionCondition string `json:"deletionCondition,omitempty"`
}

// Report is the focused ownership-inventory validation result.
type Report struct {
	MissingPackages                []string
	UnexpectedPackages             []string
	DuplicatePackages              []string
	InvalidMappings                []string
	MissingSeedServices            []string
	MissingAdditionalRoots         []string
	MissingOwnerRationales         []string
	MissingNestedRationales        []string
	InvalidRationaleFields         []string
	MissingResponsibilityClusters  []string
	MissingCrossServiceEdges       []string
	UnexpectedCrossServiceEdges    []string
	InvalidEdgeClassifications     []string
	MissingNamedOwners             []string
	UnconfirmedNamedOwners         []string
	InvalidNamedOwnerMaps          []string
	MissingCrossServiceEdgeTable   bool
	MissingProcessEdgesException   bool
	UnstableSort                   bool
	UnstableRationaleSort          bool
	UnstableResponsibilitySort     bool
	UnstableEdgeSort               bool
	UnstableNamedOwnerSort         bool
	ReusedFND01Seed                bool
}

// OK reports whether validation found no defects.
func (r Report) OK() bool {
	return len(r.MissingPackages) == 0 &&
		len(r.UnexpectedPackages) == 0 &&
		len(r.DuplicatePackages) == 0 &&
		len(r.InvalidMappings) == 0 &&
		len(r.MissingSeedServices) == 0 &&
		len(r.MissingAdditionalRoots) == 0 &&
		len(r.MissingOwnerRationales) == 0 &&
		len(r.MissingNestedRationales) == 0 &&
		len(r.InvalidRationaleFields) == 0 &&
		len(r.MissingResponsibilityClusters) == 0 &&
		len(r.MissingCrossServiceEdges) == 0 &&
		len(r.UnexpectedCrossServiceEdges) == 0 &&
		len(r.InvalidEdgeClassifications) == 0 &&
		len(r.MissingNamedOwners) == 0 &&
		len(r.UnconfirmedNamedOwners) == 0 &&
		len(r.InvalidNamedOwnerMaps) == 0 &&
		!r.MissingCrossServiceEdgeTable &&
		!r.MissingProcessEdgesException &&
		!r.UnstableSort &&
		!r.UnstableRationaleSort &&
		!r.UnstableResponsibilitySort &&
		!r.UnstableEdgeSort &&
		!r.UnstableNamedOwnerSort
}
