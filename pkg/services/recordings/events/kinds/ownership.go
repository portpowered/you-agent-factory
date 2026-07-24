package factoryeventkinds

import (
	"fmt"
	"sort"
	"strings"
)

// Stable owner and disposition constants for the event-contract ownership ledger.
// Paths are sorted lexicographically by Path (Go string <) so identical trees
// produce identical inventories for deletion-only debt tracking.
const (
	OwnerRecordings      = "recordings"
	OwnerFactorySessions = "factory_sessions"

	DispositionRetain = "retain"
	DispositionDelete = "delete"

	// PublicFactoryEventKindInventoryPath is the Recordings-owned home of the
	// public Factory Event kind inventory API.
	PublicFactoryEventKindInventoryPath = "pkg/services/recordings/events/kinds/"

	// PublicFactoryEventKindInventoryImportPath is the Go import path producers
	// and consumers must resolve for the public Factory Event kind inventory.
	PublicFactoryEventKindInventoryImportPath = "github.com/portpowered/infinite-you/pkg/services/recordings/events/kinds"

	// RetiredPublicFactoryEventKindInventoryPath is the retired non-Recordings
	// public kind inventory home tracked as deletion-only debt.
	RetiredPublicFactoryEventKindInventoryPath = "pkg/factory/events/kinds/"
)

// PublicFactoryEventKindInventoryConsumerAPI names the sole import/API surface
// producers and consumers compile against for the public Factory Event kind
// inventory. PackagePath is the repo-relative ownership ledger identity;
// ImportPath is the Go module import producers resolve.
type PublicFactoryEventKindInventoryConsumerAPI struct {
	PackagePath string
	ImportPath  string
	Symbols     []string
}

// EventContractOwnershipRow names one in-scope Factory Event schema or
// vocabulary-inventory path together with its owner and retain/delete disposition.
type EventContractOwnershipRow struct {
	Path        string
	Owner       string
	Disposition string
	Successor   string
	Condition   string
}

// EventContractOwnershipInventory returns the checked-in, stable-sorted
// event-contract ownership ledger for the FND-08 exclusive lease. Rows cover
// authored OpenAPI FactoryEvent type/payload surfaces used for kind parity,
// the Recordings-owned Go vocabulary inventory package, intentionally separate
// Factory Session response-stream contracts, and deletion-only debt for the
// retired non-Recordings inventory home.
func EventContractOwnershipInventory() []EventContractOwnershipRow {
	rows := []EventContractOwnershipRow{
		{
			Path:        "api/components/schemas/events/FactoryEvent.yaml",
			Owner:       OwnerRecordings,
			Disposition: DispositionRetain,
		},
		{
			Path:        "api/components/schemas/events/FactoryEventType.yaml",
			Owner:       OwnerRecordings,
			Disposition: DispositionRetain,
		},
		{
			Path:        "api/components/schemas/events/payloads/",
			Owner:       OwnerRecordings,
			Disposition: DispositionRetain,
		},
		{
			Path:        "api/components/schemas/response-events/",
			Owner:       OwnerFactorySessions,
			Disposition: DispositionRetain,
		},
		{
			Path:        RetiredPublicFactoryEventKindInventoryPath,
			Owner:       OwnerRecordings,
			Disposition: DispositionDelete,
			Successor:   PublicFactoryEventKindInventoryPath,
			Condition:   "retired public Factory Event kind inventory home; delete once no maintainer docs or imports treat it as durable; successor is the Recordings-owned inventory",
		},
		{
			Path:        "pkg/services/factory_sessions/internal/responseevents/",
			Owner:       OwnerFactorySessions,
			Disposition: DispositionRetain,
		},
		{
			Path:        PublicFactoryEventKindInventoryPath,
			Owner:       OwnerRecordings,
			Disposition: DispositionRetain,
		},
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Path < rows[j].Path
	})
	return rows
}

// ValidateEventContractOwnershipInventory fails closed when a required
// event-contract path is missing, duplicated, unsorted, ownerless, maps the
// public Factory Event kind inventory to an owner other than Recordings, or
// records an incomplete delete disposition.
func ValidateEventContractOwnershipInventory(rows []EventContractOwnershipRow) error {
	if len(rows) == 0 {
		return fmt.Errorf("event-contract ownership inventory is empty")
	}

	requiredPaths := []string{
		PublicFactoryEventKindInventoryPath,
		"api/components/schemas/events/FactoryEvent.yaml",
		"api/components/schemas/events/FactoryEventType.yaml",
		"api/components/schemas/events/payloads/",
		"api/components/schemas/response-events/",
		"pkg/services/factory_sessions/internal/responseevents/",
		RetiredPublicFactoryEventKindInventoryPath,
	}

	seen := make(map[string]struct{}, len(rows))
	for i, row := range rows {
		if strings.TrimSpace(row.Path) == "" {
			return fmt.Errorf("event-contract ownership row %d has an empty path", i)
		}
		if strings.TrimSpace(row.Owner) == "" {
			return fmt.Errorf("event-contract ownership path %q is ownerless", row.Path)
		}
		if row.Disposition != DispositionRetain && row.Disposition != DispositionDelete {
			return fmt.Errorf(
				"event-contract ownership path %q has unknown disposition %q",
				row.Path,
				row.Disposition,
			)
		}
		if row.Disposition == DispositionDelete {
			if strings.TrimSpace(row.Successor) == "" {
				return fmt.Errorf("event-contract ownership path %q delete disposition requires successor", row.Path)
			}
			if strings.TrimSpace(row.Condition) == "" {
				return fmt.Errorf("event-contract ownership path %q delete disposition requires condition", row.Path)
			}
		}
		if _, ok := seen[row.Path]; ok {
			return fmt.Errorf("event-contract ownership inventory contains duplicate path %q", row.Path)
		}
		seen[row.Path] = struct{}{}
		if i > 0 && rows[i-1].Path >= row.Path {
			return fmt.Errorf(
				"event-contract ownership inventory is not strictly sorted by path: %q then %q",
				rows[i-1].Path,
				row.Path,
			)
		}
	}

	for _, required := range requiredPaths {
		if _, ok := seen[required]; !ok {
			return fmt.Errorf("event-contract ownership inventory missing required path %q", required)
		}
	}

	for _, row := range rows {
		if row.Path != PublicFactoryEventKindInventoryPath {
			continue
		}
		if row.Owner != OwnerRecordings {
			return fmt.Errorf(
				"public Factory Event kind inventory path %q must be owned by %q, got %q",
				row.Path,
				OwnerRecordings,
				row.Owner,
			)
		}
		if row.Disposition != DispositionRetain {
			return fmt.Errorf(
				"public Factory Event kind inventory path %q must retain disposition, got %q",
				row.Path,
				row.Disposition,
			)
		}
	}

	return nil
}

// PublicFactoryEventKindInventoryConsumerAPISurface returns the Recordings-owned
// import/API identity that producers and consumers must compile against for the
// public Factory Event kind inventory. Symbols are stable-sorted.
func PublicFactoryEventKindInventoryConsumerAPISurface() PublicFactoryEventKindInventoryConsumerAPI {
	symbols := []string{
		"ContractOnlyFactoryEventKinds",
		"CurrentFactoryEventKindInventory",
		"ExcludedNonPublicFactoryEventKinds",
		"PublicEmittableFactoryEventKinds",
		"ValidateFactoryEventKindInventory",
	}
	sort.Strings(symbols)
	return PublicFactoryEventKindInventoryConsumerAPI{
		PackagePath: PublicFactoryEventKindInventoryPath,
		ImportPath:  PublicFactoryEventKindInventoryImportPath,
		Symbols:     symbols,
	}
}

// ClaimedPublicFactoryEventKindInventoryPaths returns every path identity that
// is or was a public Factory Event kind inventory package home. Paths are
// stable-sorted. Competing homes outside Recordings must appear in
// EventContractOwnershipInventory as delete debt with the Recordings successor;
// adjacent response-stream vocabulary is intentionally omitted here.
func ClaimedPublicFactoryEventKindInventoryPaths() []string {
	paths := []string{
		PublicFactoryEventKindInventoryPath,
		RetiredPublicFactoryEventKindInventoryPath,
	}
	sort.Strings(paths)
	return paths
}

// ValidateSolePublicFactoryEventKindInventoryOwnership proves producers and
// consumers resolve to exactly one Recordings-owned public Factory Event kind
// inventory. Competing claimed inventory paths either fail closed or must be
// listed as deletion-only debt with the Recordings successor.
func ValidateSolePublicFactoryEventKindInventoryOwnership(
	rows []EventContractOwnershipRow,
	claimedPaths []string,
	consumerAPI PublicFactoryEventKindInventoryConsumerAPI,
) error {
	if err := ValidateEventContractOwnershipInventory(rows); err != nil {
		return fmt.Errorf("event-contract ownership inventory: %w", err)
	}

	if strings.TrimSpace(consumerAPI.PackagePath) == "" {
		return fmt.Errorf("public Factory Event kind inventory consumer API is missing package path")
	}
	if strings.TrimSpace(consumerAPI.ImportPath) == "" {
		return fmt.Errorf("public Factory Event kind inventory consumer API is missing import path")
	}
	if consumerAPI.PackagePath != PublicFactoryEventKindInventoryPath {
		return fmt.Errorf(
			"public Factory Event kind inventory consumer API package path %q must resolve to %q",
			consumerAPI.PackagePath,
			PublicFactoryEventKindInventoryPath,
		)
	}
	if consumerAPI.ImportPath != PublicFactoryEventKindInventoryImportPath {
		return fmt.Errorf(
			"public Factory Event kind inventory consumer API import path %q must resolve to %q",
			consumerAPI.ImportPath,
			PublicFactoryEventKindInventoryImportPath,
		)
	}
	if len(consumerAPI.Symbols) == 0 {
		return fmt.Errorf("public Factory Event kind inventory consumer API has empty symbols")
	}
	for i, symbol := range consumerAPI.Symbols {
		if strings.TrimSpace(symbol) == "" {
			return fmt.Errorf("public Factory Event kind inventory consumer API symbol %d is empty", i)
		}
		if i > 0 && consumerAPI.Symbols[i-1] >= symbol {
			return fmt.Errorf(
				"public Factory Event kind inventory consumer API symbols are not strictly sorted: %q then %q",
				consumerAPI.Symbols[i-1],
				symbol,
			)
		}
	}

	if len(claimedPaths) == 0 {
		return fmt.Errorf("claimed public Factory Event kind inventory paths are empty")
	}
	claimedSeen := make(map[string]struct{}, len(claimedPaths))
	for i, path := range claimedPaths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("claimed public Factory Event kind inventory path %d is empty", i)
		}
		if _, ok := claimedSeen[path]; ok {
			return fmt.Errorf("claimed public Factory Event kind inventory paths contain duplicate %q", path)
		}
		claimedSeen[path] = struct{}{}
		if i > 0 && claimedPaths[i-1] >= path {
			return fmt.Errorf(
				"claimed public Factory Event kind inventory paths are not strictly sorted: %q then %q",
				claimedPaths[i-1],
				path,
			)
		}
	}
	if _, ok := claimedSeen[PublicFactoryEventKindInventoryPath]; !ok {
		return fmt.Errorf(
			"claimed public Factory Event kind inventory paths missing canonical Recordings path %q",
			PublicFactoryEventKindInventoryPath,
		)
	}

	byPath := make(map[string]EventContractOwnershipRow, len(rows))
	for _, row := range rows {
		byPath[row.Path] = row
	}

	retainCount := 0
	for _, path := range claimedPaths {
		row, ok := byPath[path]
		if !ok {
			return fmt.Errorf(
				"claimed public Factory Event kind inventory path %q is missing from ownership inventory; list it as delete debt with Recordings successor or remove the competing inventory",
				path,
			)
		}
		if path == PublicFactoryEventKindInventoryPath {
			if row.Owner != OwnerRecordings || row.Disposition != DispositionRetain {
				return fmt.Errorf(
					"canonical public Factory Event kind inventory path %q must be owned by %q with disposition %q",
					path,
					OwnerRecordings,
					DispositionRetain,
				)
			}
			retainCount++
			continue
		}
		if row.Disposition == DispositionRetain {
			return fmt.Errorf(
				"competing public Factory Event kind inventory path %q retains ownership; only %q may retain, or list %q as delete debt with Recordings successor",
				path,
				PublicFactoryEventKindInventoryPath,
				path,
			)
		}
		if row.Disposition != DispositionDelete {
			return fmt.Errorf(
				"competing public Factory Event kind inventory path %q must use delete disposition, got %q",
				path,
				row.Disposition,
			)
		}
		if row.Successor != PublicFactoryEventKindInventoryPath {
			return fmt.Errorf(
				"competing public Factory Event kind inventory path %q delete successor %q must be %q",
				path,
				row.Successor,
				PublicFactoryEventKindInventoryPath,
			)
		}
		if strings.TrimSpace(row.Condition) == "" {
			return fmt.Errorf(
				"competing public Factory Event kind inventory path %q delete disposition requires condition",
				path,
			)
		}
	}

	if retainCount != 1 {
		return fmt.Errorf(
			"expected exactly one retained public Factory Event kind inventory path (%q), found %d",
			PublicFactoryEventKindInventoryPath,
			retainCount,
		)
	}

	return nil
}

// ValidateCurrentSolePublicFactoryEventKindInventoryOwnership validates the
// checked-in ownership ledger, claimed inventory paths, and consumer API surface
// against the sole Recordings-owned public Factory Event kind inventory rule.
func ValidateCurrentSolePublicFactoryEventKindInventoryOwnership() error {
	return ValidateSolePublicFactoryEventKindInventoryOwnership(
		EventContractOwnershipInventory(),
		ClaimedPublicFactoryEventKindInventoryPaths(),
		PublicFactoryEventKindInventoryConsumerAPISurface(),
	)
}
