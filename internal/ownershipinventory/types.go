// Package ownershipinventory freezes Packaged Service Structure package
// destinations for PSS-F01 and validates that freeze against production pkg.
package ownershipinventory

const (
	InventoryRelativePath = "docs/internal/baselines/ownership-inventory.json"

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

	CrossServiceEdgeSortKeyDescription = "fromOwner then toOwner ascending byte order"
	NamedOwnerSortKeyDescription       = "owner ascending byte order"
	MisplacedGuardSortKeyDescription   = "id ascending byte order"

	NamedOwnerStatusConfirmed = "confirmed"

	EdgeClassCommand             = "command"
	EdgeClassQuery               = "query"
	EdgeClassEvent               = "event"
	EdgeClassProtocolComposition = "protocol_composition"
	EdgeClassConstruction        = "construction"
	EdgeClassLifecycle           = "lifecycle"
	EdgeClassExternalEffect      = "external_effect"
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
//
// UnfinishedMoves and Packages are not part of the artifact on disk. Open moves
// live in one consolidated ledger (UnfinishedMovesRelativePath) shared with the
// packaged-service-structure checker; Load attaches them here so inventory
// consumers keep a single row view.
//
// Design intent about the owner tree — per-service rationale cards,
// responsibility clusters, public-surface ownership, and owned roles — is not
// part of this artifact either. It is prose that no gate counts, so it lives in
// docs/architecture/service-ownership-rationale.md instead of here.
type Inventory struct {
	Version                 int                      `json:"version"`
	Stage                   string                   `json:"stage"`
	SortKey                 string                   `json:"sortKey"`
	Destinations            DestinationVocabulary    `json:"destinations"`
	ProcessEdgesException   ProcessEdgesException    `json:"processEdgesException"`
	SeedServices            []SeedService            `json:"seedServices"`
	AdditionalCurrentRoots  []string                 `json:"additionalCurrentRoots"`
	CrossServiceEdges       []CrossServiceEdge       `json:"crossServiceEdges"`
	NamedOwnerConfirmations []NamedOwnerConfirmation `json:"namedOwnerConfirmations"`
	MisplacedGuards         []MisplacedGuardEntry    `json:"misplacedGuards"`
	UnfinishedMoves         UnfinishedMoves          `json:"-"`
	Packages                []PackageRow             `json:"-"`
}

// MisplacedGuardEntry records one normative standard, allowlist, package guard,
// baseline, or diagnostic that incorrectly assigns provider inference or
// hosted polling to Workers. The committed inventory is empty once corrected.
type MisplacedGuardEntry struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	SurfacePath       string `json:"surfacePath"`
	CurrentOwnerClaim string `json:"currentOwnerClaim"`
	MisplacedConcern  string `json:"misplacedConcern"`
	ReplacementOwner  string `json:"replacementOwner"`
	Note              string `json:"note"`
}

// NamedOwnerConfirmation freezes one PRD-named owner onto the committed tree
// with its reviewed nested-subservice map so PSS-F02 does not reopen discovery.
type NamedOwnerConfirmation struct {
	Owner                string                `json:"owner"`
	DisplayName          string                `json:"displayName"`
	TargetPath           string                `json:"targetPath"`
	Status               string                `json:"status"`
	NestedSubservices    []string              `json:"nestedSubservices"`
	ResidualPackageRules []ResidualPackageRule `json:"residualPackageRules,omitempty"`
	Note                 string                `json:"note"`
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
	FromOwner string `json:"fromOwner"`
	ToOwner   string `json:"toOwner"`
	Class     string `json:"class"`
	// Bidirectional and Unresolved keep a reciprocal import pair visible as
	// convergence debt even when each direction has a valid interaction class.
	Bidirectional         bool   `json:"bidirectional,omitempty"`
	Unresolved            bool   `json:"unresolved,omitempty"`
	ArchitectureException bool   `json:"architectureException,omitempty"`
	Evidence              string `json:"evidence"`
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
// destination in the product-owner tree.
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
	// UnexpectedPackages lists package rows whose packagePath is absent from the
	// live tree. There is deliberately no "missing package" counterpart: package
	// ownership is derived by OwnerForPackage, so a live package without a row is
	// valid.
	UnexpectedPackages           []string
	DuplicatePackages            []string
	InvalidMappings              []string
	MissingSeedServices          []string
	MissingAdditionalRoots       []string
	MissingCrossServiceEdges     []string
	UnexpectedCrossServiceEdges  []string
	InvalidEdgeClassifications   []string
	InvalidBidirectionalEdges    []string
	MissingNamedOwners           []string
	UnconfirmedNamedOwners       []string
	InvalidNamedOwnerMaps        []string
	MissingMisplacedGuards       []string
	InvalidMisplacedGuards       []string
	MissingCrossServiceEdgeTable bool
	MissingProcessEdgesException bool
	UnstableSort                 bool
	UnstableEdgeSort             bool
	UnstableNamedOwnerSort       bool
	UnstableMisplacedGuardSort   bool
}

// ViolationCount returns the number of independently reported inventory
// findings. Structured list entries count individually; boolean gate failures
// count once each so a consolidated diagnostic cannot hide added debt.
func (r Report) ViolationCount() int {
	return countStrings(
		r.UnexpectedPackages,
		r.DuplicatePackages,
		r.InvalidMappings,
		r.MissingSeedServices,
		r.MissingAdditionalRoots,
		r.MissingCrossServiceEdges,
		r.UnexpectedCrossServiceEdges,
		r.InvalidEdgeClassifications,
		r.InvalidBidirectionalEdges,
		r.MissingNamedOwners,
		r.UnconfirmedNamedOwners,
		r.InvalidNamedOwnerMaps,
		r.MissingMisplacedGuards,
		r.InvalidMisplacedGuards,
	) + countFlags(
		r.MissingCrossServiceEdgeTable,
		r.MissingProcessEdgesException,
		r.UnstableSort,
		r.UnstableEdgeSort,
		r.UnstableNamedOwnerSort,
		r.UnstableMisplacedGuardSort,
	)
}

func countStrings(groups ...[]string) int {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	return count
}

func countFlags(flags ...bool) int {
	count := 0
	for _, flag := range flags {
		if flag {
			count++
		}
	}
	return count
}

// OK reports whether validation found no defects.
func (r Report) OK() bool {
	return len(r.UnexpectedPackages) == 0 &&
		len(r.DuplicatePackages) == 0 &&
		len(r.InvalidMappings) == 0 &&
		len(r.MissingSeedServices) == 0 &&
		len(r.MissingAdditionalRoots) == 0 &&
		len(r.MissingCrossServiceEdges) == 0 &&
		len(r.UnexpectedCrossServiceEdges) == 0 &&
		len(r.InvalidEdgeClassifications) == 0 &&
		len(r.InvalidBidirectionalEdges) == 0 &&
		len(r.MissingNamedOwners) == 0 &&
		len(r.UnconfirmedNamedOwners) == 0 &&
		len(r.InvalidNamedOwnerMaps) == 0 &&
		len(r.MissingMisplacedGuards) == 0 &&
		len(r.InvalidMisplacedGuards) == 0 &&
		!r.MissingCrossServiceEdgeTable &&
		!r.MissingProcessEdgesException &&
		!r.UnstableSort &&
		!r.UnstableEdgeSort &&
		!r.UnstableNamedOwnerSort &&
		!r.UnstableMisplacedGuardSort
}
