package factoryeventkinds

import (
	"slices"
	"strings"
	"testing"
)

func TestEventContractOwnershipInventory_IsStableSortedAndValid(t *testing.T) {
	t.Parallel()

	first := EventContractOwnershipInventory()
	if err := ValidateEventContractOwnershipInventory(first); err != nil {
		t.Fatalf("canonical ownership inventory must validate: %v", err)
	}

	second := EventContractOwnershipInventory()
	if !slices.Equal(first, second) {
		t.Fatal("EventContractOwnershipInventory must return an identical ordered list on repeated calls")
	}

	for i := 1; i < len(first); i++ {
		if first[i-1].Path >= first[i].Path {
			t.Fatalf("ownership inventory must be strictly sorted by path; out of order at %q then %q", first[i-1].Path, first[i].Path)
		}
	}
}

func TestEventContractOwnershipInventory_NamesRequiredLeasePaths(t *testing.T) {
	t.Parallel()

	byPath := map[string]EventContractOwnershipRow{}
	for _, row := range EventContractOwnershipInventory() {
		byPath[row.Path] = row
	}

	required := []struct {
		path        string
		owner       string
		disposition string
	}{
		{PublicFactoryEventKindInventoryPath, OwnerRecordings, DispositionRetain},
		{"api/components/schemas/events/FactoryEvent.yaml", OwnerRecordings, DispositionRetain},
		{"api/components/schemas/events/FactoryEventType.yaml", OwnerRecordings, DispositionRetain},
		{"api/components/schemas/events/payloads/", OwnerRecordings, DispositionRetain},
		{"api/components/schemas/response-events/", OwnerFactorySessions, DispositionRetain},
		{"pkg/services/factory_sessions/internal/responseevents/", OwnerFactorySessions, DispositionRetain},
		{"pkg/factory/events/kinds/", OwnerRecordings, DispositionDelete},
	}

	for _, want := range required {
		got, ok := byPath[want.path]
		if !ok {
			t.Fatalf("required event-contract path %q missing from ownership inventory", want.path)
		}
		if got.Owner != want.owner {
			t.Fatalf("path %q owner = %q, want %q", want.path, got.Owner, want.owner)
		}
		if got.Disposition != want.disposition {
			t.Fatalf("path %q disposition = %q, want %q", want.path, got.Disposition, want.disposition)
		}
	}

	retired := byPath["pkg/factory/events/kinds/"]
	if retired.Successor != PublicFactoryEventKindInventoryPath {
		t.Fatalf("retired inventory path successor = %q, want %q", retired.Successor, PublicFactoryEventKindInventoryPath)
	}
	if strings.TrimSpace(retired.Condition) == "" {
		t.Fatal("retired inventory path must record a non-empty delete condition for deletion-only debt tracking")
	}
}

func TestValidateEventContractOwnershipInventory_FailsClosed(t *testing.T) {
	t.Parallel()

	canonical := EventContractOwnershipInventory()

	t.Run("missing_required_path", func(t *testing.T) {
		t.Parallel()
		rows := filterOutPath(canonical, PublicFactoryEventKindInventoryPath)
		if err := ValidateEventContractOwnershipInventory(rows); err == nil {
			t.Fatal("expected missing required path to fail validation")
		}
	})

	t.Run("duplicate_path", func(t *testing.T) {
		t.Parallel()
		rows := append([]EventContractOwnershipRow{}, canonical...)
		rows = append(rows, canonical[0])
		if err := ValidateEventContractOwnershipInventory(rows); err == nil {
			t.Fatal("expected duplicate path to fail validation")
		}
	})

	t.Run("unsorted", func(t *testing.T) {
		t.Parallel()
		if len(canonical) < 2 {
			t.Fatal("need at least two ownership rows to prove sort validation")
		}
		rows := append([]EventContractOwnershipRow{}, canonical...)
		rows[0], rows[len(rows)-1] = rows[len(rows)-1], rows[0]
		if err := ValidateEventContractOwnershipInventory(rows); err == nil {
			t.Fatal("expected unsorted inventory to fail validation")
		}
	})

	t.Run("ownerless", func(t *testing.T) {
		t.Parallel()
		rows := append([]EventContractOwnershipRow{}, canonical...)
		rows[0].Owner = ""
		if err := ValidateEventContractOwnershipInventory(rows); err == nil {
			t.Fatal("expected ownerless row to fail validation")
		}
	})

	t.Run("kind_inventory_not_owned_by_recordings", func(t *testing.T) {
		t.Parallel()
		rows := append([]EventContractOwnershipRow{}, canonical...)
		for i := range rows {
			if rows[i].Path == PublicFactoryEventKindInventoryPath {
				rows[i].Owner = OwnerFactorySessions
				break
			}
		}
		if err := ValidateEventContractOwnershipInventory(rows); err == nil {
			t.Fatal("expected non-recordings owner for public kind inventory to fail validation")
		}
	})
}

func filterOutPath(rows []EventContractOwnershipRow, path string) []EventContractOwnershipRow {
	filtered := make([]EventContractOwnershipRow, 0, len(rows))
	for _, row := range rows {
		if row.Path == path {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}
