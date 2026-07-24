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
)

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
			Path:        "pkg/factory/events/kinds/",
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
		"pkg/factory/events/kinds/",
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
