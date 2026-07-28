package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

type stubLoadedSource struct {
	dir string
	cfg *factorydefinitions.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return s.dir }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	var cfg factorydefinitions.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factorydefinitions.ErrInvalidNamedFactory
	}
	return stubLoadedSource{dir: "/factories/example", cfg: &cfg}, nil
}

func stubPreparePortable(
	_ string,
	factoryConfig *factorydefinitions.FactoryConfig,
	_ bool,
) (*factorydefinitions.FactoryConfig, error) {
	return factoryConfig, nil
}

func newCaptureService(t *testing.T) snapshotsportability.Service {
	t.Helper()
	return newSnapshotService(t, stubDecodeSnapshot)
}

func stubDecodeSnapshot(payload []byte) (*factorydefinitions.FactorySnapshot, error) {
	return factorydefinitions.NewFactorySnapshot(json.RawMessage(payload))
}

func newSnapshotService(
	t *testing.T,
	decode factorydefinitions.FactorySnapshotJSONDecoder,
) snapshotsportability.Service {
	t.Helper()
	fileSystem := platformfilesystem.Local{}
	svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             stubLoadCanonical,
		CaptureLoaded:             snapshotsportabilitycapture.NewLoaded(snapshotObjectMapper),
		PreparePortable:           stubPreparePortable,
		DecodeSnapshot:            decode,
		MaterializePortableFiles:  snapshotsportabilitymaterialize.NewMaterializer(fileSystem),
		ValidateMaterializeWrites: snapshotsportabilitymaterialize.NewWritesValidator(fileSystem),
	})
	if err != nil {
		t.Fatalf("snapshotsportabilitywire.NewService: %v", err)
	}
	return svc
}

func testSnapshotPayload() []byte {
	return []byte(`{
		"name": "alpha",
		"factoryDirectory": "/factories/alpha",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
}

func snapshotObjectMapper(factory *factorydefinitions.FactoryConfig) (map[string]any, error) {
	return map[string]any{"name": factory.Name}, nil
}

func fullSnapshotObjectMapper(factory *factorydefinitions.FactoryConfig) (map[string]any, error) {
	encoded, err := json.Marshal(factory)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	return object, nil
}

func newRoundTripService(t *testing.T) snapshotsportability.Service {
	t.Helper()
	fileSystem := platformfilesystem.Local{}
	svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             stubLoadCanonical,
		CaptureLoaded:             snapshotsportabilitycapture.NewLoaded(fullSnapshotObjectMapper),
		PreparePortable:           stubPreparePortable,
		DecodeSnapshot:            stubDecodeSnapshot,
		MaterializePortableFiles:  snapshotsportabilitymaterialize.NewMaterializer(fileSystem),
		ValidateMaterializeWrites: snapshotsportabilitymaterialize.NewWritesValidator(fileSystem),
	})
	if err != nil {
		t.Fatalf("snapshotsportabilitywire.NewService: %v", err)
	}
	return svc
}

func roundTripCanonicalPayload() []byte {
	return []byte(`{
		"name": "alpha",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
}

func TestCaptureFactorySnapshot_SuccessFromCanonicalPayload(t *testing.T) {
	t.Parallel()

	svc := newCaptureService(t)
	payload := []byte(`{"name":"alpha"}`)

	captured, err := svc.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  payload,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}

	var object map[string]any
	if decodeErr := captured.Snapshot.Decode(&object); decodeErr != nil {
		t.Fatalf("CaptureFactorySnapshot decode: %v", decodeErr)
	}
	if object["name"] != "alpha" {
		t.Fatalf("snapshot name = %#v, want alpha", object["name"])
	}
	if object["factoryDirectory"] != "/factories/alpha" {
		t.Fatalf("factoryDirectory = %#v, want /factories/alpha", object["factoryDirectory"])
	}
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata type = %T, want map[string]any", object["metadata"])
	}
	if metadata["source_format"] != factorydefinitions.ReplayV1SourceFormat {
		t.Fatalf("metadata source_format = %#v, want %q", metadata["source_format"], factorydefinitions.ReplayV1SourceFormat)
	}
}

func TestCaptureFactorySnapshot_InvalidPayloadReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newCaptureService(t)

	_, err := svc.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"string"`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"CaptureFactorySnapshot invalid-payload error = %v, want %v",
			err,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}
}

func TestPrepareFactorySnapshotImport_SuccessReturnsPortableFacts(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)
	payload := testSnapshotPayload()

	imported, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" ||
		len(imported.Portable.Assets) == 0 ||
		imported.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("PrepareFactorySnapshotImport portable = %#v, want portable success facts", imported.Portable)
	}
}

func TestPrepareFactorySnapshotImport_InvalidPayloadReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)

	_, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport invalid-payload error = %v, want %v",
			err,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}
}

func TestMaterializeFactorySnapshot_SuccessRestoresBundledAssets(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)
	payload := testSnapshotPayload()

	imported, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}

	targetDir := t.TempDir()
	materialized, err := svc.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: targetDir,
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}
	if materialized.TargetDir != targetDir ||
		materialized.Portable.FactoryDir != targetDir ||
		len(materialized.Portable.Assets) == 0 {
		t.Fatalf("MaterializeFactorySnapshot result = %#v, want portable success facts", materialized)
	}

	docPath := filepath.Join(targetDir, "docs", "README.md")
	content, readErr := os.ReadFile(docPath)
	if readErr != nil {
		t.Fatalf("read materialized doc: %v", readErr)
	}
	if string(content) != "hello" {
		t.Fatalf("materialized doc content = %q, want hello", content)
	}
}

func TestDetachedSnapshot_CapturePrepareImportMaterializeRoundTrip(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	ctx := context.Background()
	canonical := roundTripCanonicalPayload()
	factoryDir := "/factories/alpha"

	captured, err := svc.CaptureFactorySnapshot(
		ctx,
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: factoryDir,
			Canonical:  canonical,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}

	detachedPayload, marshalErr := json.Marshal(captured.Snapshot)
	if marshalErr != nil {
		t.Fatalf("marshal detached snapshot payload: %v", marshalErr)
	}
	if !json.Valid(detachedPayload) || detachedPayload[0] != '{' {
		t.Fatalf("detached snapshot payload = %s, want JSON object", detachedPayload)
	}

	var capturedObject map[string]any
	if decodeErr := captured.Snapshot.Decode(&capturedObject); decodeErr != nil {
		t.Fatalf("decode captured snapshot: %v", decodeErr)
	}
	if capturedObject["name"] != "alpha" {
		t.Fatalf("captured name = %#v, want alpha", capturedObject["name"])
	}
	if capturedObject["factoryDirectory"] != factoryDir {
		t.Fatalf("captured factoryDirectory = %#v, want %q", capturedObject["factoryDirectory"], factoryDir)
	}
	metadata, ok := capturedObject["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("captured metadata type = %T, want map[string]any", capturedObject["metadata"])
	}
	if metadata["source_format"] != factorydefinitions.ReplayV1SourceFormat {
		t.Fatalf("captured source_format = %#v, want %q", metadata["source_format"], factorydefinitions.ReplayV1SourceFormat)
	}
	if metadata["factory_hash"] == "" || metadata["runtime_config_hash"] == "" {
		t.Fatalf("captured replay metadata = %#v, want non-empty portable hashes", metadata)
	}

	imported, err := svc.PrepareFactorySnapshotImport(
		ctx,
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: detachedPayload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != factoryDir ||
		len(imported.Portable.Assets) == 0 ||
		imported.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("PrepareFactorySnapshotImport portable = %#v, want replay-compatible asset facts", imported.Portable)
	}

	targetDir := t.TempDir()
	materialized, err := svc.MaterializeFactorySnapshot(
		ctx,
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: targetDir,
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}
	if materialized.TargetDir != targetDir ||
		materialized.Portable.FactoryDir != targetDir ||
		len(materialized.Portable.Assets) == 0 ||
		materialized.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("MaterializeFactorySnapshot result = %#v, want portable success facts", materialized)
	}

	docPath := filepath.Join(targetDir, "docs", "README.md")
	content, readErr := os.ReadFile(docPath)
	if readErr != nil {
		t.Fatalf("read materialized doc: %v", readErr)
	}
	if string(content) != "hello" {
		t.Fatalf("materialized doc content = %q, want hello", content)
	}
}

func TestMaterializeFactorySnapshot_UnsafeTargetReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "alpha"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	_, unsafeErr := svc.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  snapshot,
		},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf(
			"MaterializeFactorySnapshot unsafe error = %v, want %v",
			unsafeErr,
			factorydefinitions.ErrUnsafeFactorySnapshotMaterialize,
		)
	}
	if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatal("unsafe materialize must not also match ErrInvalidFactorySnapshotPayload")
	}
}
