package wire_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
)

func TestNewJavaScriptCheckpointStoreConstructsWorkingAdapter(t *testing.T) {
	t.Parallel()

	var store factoryruntime.JavaScriptCheckpointStore = factoryruntimewire.NewJavaScriptCheckpointStore()
	if store == nil {
		t.Fatal("NewJavaScriptCheckpointStore() = nil, want JavaScript checkpoint store")
	}
}

func TestNewDurableJavaScriptCheckpointStoreRoundTripsAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), ".you-agent-factory", "durable-sessions")
	writer, err := factoryruntimewire.NewDurableJavaScriptCheckpointStore(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(writer): %v", err)
	}
	want := factorydefinitions.JavaScriptCheckpointRecord{
		ID:          "checkpoint-1",
		Label:       "after-plan",
		Summary:     "checkpoint summary",
		Timestamp:   time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		ArtifactID:  "artifact-1",
		ContentHash: "sha256:checkpoint",
		SizeBytes:   4,
		RawBody:     []byte{0x00, 0x01, 0x7f, 0xff},
		StoragePath: "checkpoint-1.json",
	}
	writer.Put(want)

	reader, err := factoryruntimewire.NewDurableJavaScriptCheckpointStore(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	got, ok := reader.Get(want.ID)
	if !ok {
		t.Fatal("Get() returned false, want stored record")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}

func TestNewJavaScriptCheckpointSummariesConstructsWorkingProjector(t *testing.T) {
	t.Parallel()

	summaries := factoryruntimewire.NewJavaScriptCheckpointSummaries()
	if summaries == nil {
		t.Fatal("NewJavaScriptCheckpointSummaries() = nil, want JavaScript checkpoint summary projector")
	}
}
