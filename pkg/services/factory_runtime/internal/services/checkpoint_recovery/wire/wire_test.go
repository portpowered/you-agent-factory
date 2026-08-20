package wire_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestNewProcessLocalCheckpointStoreConstructsWorkingAdapter(t *testing.T) {
	t.Parallel()

	var store checkpointrecovery.CheckpointStore = checkpointrecoverywire.NewProcessLocalCheckpointStore()
	if store == nil {
		t.Fatal("NewProcessLocalCheckpointStore() = nil, want process-local adapter")
	}
}

func TestNewConstructsCheckpointRecoveryCapability(t *testing.T) {
	t.Parallel()

	var recovery checkpointrecovery.Service = checkpointrecoverywire.New()
	if recovery == nil {
		t.Fatal("New() = nil, want checkpoint recovery capability")
	}
}

func TestNewJavaScriptCheckpointStoreConstructsWorkingAdapter(t *testing.T) {
	t.Parallel()

	var store factoryruntime.JavaScriptCheckpointStore = checkpointrecoverywire.NewJavaScriptCheckpointStore()
	if store == nil {
		t.Fatal("NewJavaScriptCheckpointStore() = nil, want JavaScript checkpoint store")
	}
}

func TestDurableCheckpointStoresRoundTripAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), ".you-agent-factory", "durable-sessions")
	opaqueConstructor := checkpointrecoverywire.NewDurableCheckpointStore(platformfilesystem.Local{})
	opaqueWriter, err := opaqueConstructor(root)
	if err != nil {
		t.Fatalf("NewDurableCheckpointStore(writer): %v", err)
	}
	javascriptConstructor := checkpointrecoverywire.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	javascriptWriter, err := javascriptConstructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(writer): %v", err)
	}
	opaque := checkpointrecovery.Envelope{
		CheckpointID:  "checkpoint-1",
		SchemaVersion: 1,
		StrategyKind:  "runtime",
		Payload:       []byte{0x00, 0x01, 0x7f, 0xff},
	}
	if err := opaqueWriter.Put(opaque); err != nil {
		t.Fatalf("opaque Put(): %v", err)
	}
	javascript := factorydefinitions.JavaScriptCheckpointRecord{
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
	javascriptWriter.Put(javascript)

	opaqueReader, err := opaqueConstructor(root)
	if err != nil {
		t.Fatalf("NewDurableCheckpointStore(reader): %v", err)
	}
	javascriptReader, err := javascriptConstructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	gotOpaque, err := opaqueReader.Get(opaque.CheckpointID)
	if err != nil {
		t.Fatalf("opaque Get(): %v", err)
	}
	if !reflect.DeepEqual(gotOpaque, opaque) {
		t.Fatalf("opaque Get() = %#v, want %#v", gotOpaque, opaque)
	}
	gotJavaScript, ok := javascriptReader.Get(javascript.ID)
	if !ok {
		t.Fatal("JavaScript Get() returned false, want stored record")
	}
	if !reflect.DeepEqual(gotJavaScript, javascript) {
		t.Fatalf("JavaScript Get() = %#v, want %#v", gotJavaScript, javascript)
	}
}

func TestDurableCheckpointStoresRequireExplicitRoot(t *testing.T) {
	t.Parallel()

	if _, err := checkpointrecoverywire.NewDurableCheckpointStore(platformfilesystem.Local{})(" "); err == nil {
		t.Fatal("NewDurableCheckpointStore(blank) error = nil")
	}
	if _, err := checkpointrecoverywire.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})(" "); err == nil {
		t.Fatal("NewDurableJavaScriptCheckpointStore(blank) error = nil")
	}
}

func TestNewJavaScriptCheckpointSummariesConstructsWorkingProjector(t *testing.T) {
	t.Parallel()

	summaries := checkpointrecoverywire.NewJavaScriptCheckpointSummaries()
	if summaries == nil {
		t.Fatal("NewJavaScriptCheckpointSummaries() = nil, want JavaScript checkpoint summary projector")
	}
}
