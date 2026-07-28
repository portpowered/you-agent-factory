package javascriptstore_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore"
)

func TestStoreKeepsRecordsIsolatedAndSorted(t *testing.T) {
	t.Parallel()

	store := javascriptstore.New()
	store.Put(factorydefinitions.JavaScriptCheckpointRecord{ID: "checkpoint-b"})
	store.Put(factorydefinitions.JavaScriptCheckpointRecord{ID: "checkpoint-a"})
	store.Put(factorydefinitions.JavaScriptCheckpointRecord{})

	records := store.List()
	if len(records) != 2 {
		t.Fatalf("List length = %d, want 2", len(records))
	}
	if records[0].ID != "checkpoint-a" || records[1].ID != "checkpoint-b" {
		t.Fatalf("List IDs = [%q %q], want stable ID order", records[0].ID, records[1].ID)
	}
	record, ok := store.Get("checkpoint-b")
	if !ok || record.ID != "checkpoint-b" {
		t.Fatalf("Get(checkpoint-b) = (%#v, %t), want stored record", record, ok)
	}
}
