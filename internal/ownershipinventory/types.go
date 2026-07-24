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
)

// Inventory is the frozen PSS-F01 ownership inventory artifact.
type Inventory struct {
	Version                int                   `json:"version"`
	Stage                  string                `json:"stage"`
	SortKey                string                `json:"sortKey"`
	FND01SeedPath          string                `json:"fnd01SeedPath"`
	Destinations           DestinationVocabulary `json:"destinations"`
	ProcessEdgesException  ProcessEdgesException `json:"processEdgesException"`
	SeedServices           []SeedService         `json:"seedServices"`
	AdditionalCurrentRoots []string              `json:"additionalCurrentRoots"`
	Packages               []PackageRow          `json:"packages"`
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
	MissingPackages              []string
	UnexpectedPackages           []string
	DuplicatePackages            []string
	InvalidMappings              []string
	MissingSeedServices          []string
	MissingAdditionalRoots       []string
	MissingProcessEdgesException bool
	UnstableSort                 bool
	ReusedFND01Seed              bool
}

// OK reports whether validation found no defects.
func (r Report) OK() bool {
	return len(r.MissingPackages) == 0 &&
		len(r.UnexpectedPackages) == 0 &&
		len(r.DuplicatePackages) == 0 &&
		len(r.InvalidMappings) == 0 &&
		len(r.MissingSeedServices) == 0 &&
		len(r.MissingAdditionalRoots) == 0 &&
		!r.MissingProcessEdgesException &&
		!r.UnstableSort
}
