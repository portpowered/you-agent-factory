package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

type stubLoadedSource struct {
	dir string
	cfg *factorycontracts.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorycontracts.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return s.dir }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorycontracts.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factorycontracts.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
	var cfg factorycontracts.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factorydefinitions.ErrInvalidNamedFactory
	}
	return stubLoadedSource{dir: "/factories/example", cfg: &cfg}, nil
}

func stubPreparePortable(
	_ string,
	factoryConfig *factorycontracts.FactoryConfig,
	_ bool,
) (*factorycontracts.FactoryConfig, error) {
	return factoryConfig, nil
}

func newCaptureService(t *testing.T) snapshotsportability.Service {
	t.Helper()
	svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             stubLoadCanonical,
		CaptureLoaded:             snapshotsportabilitycapture.NewLoaded(snapshotObjectMapper),
		PreparePortable:           stubPreparePortable,
		DecodeSnapshot:            func([]byte) (*factorydefinitions.FactorySnapshot, error) { return nil, nil },
		MaterializePortableFiles:  func(string, *factorycontracts.FactoryConfig) ([]factorycontracts.PortableBundledFileReplacement, error) { return nil, nil },
		ValidateMaterializeWrites: func(string, *factorycontracts.FactoryConfig) error { return nil },
	})
	if err != nil {
		t.Fatalf("snapshotsportabilitywire.NewService: %v", err)
	}
	return svc
}

func snapshotObjectMapper(factory *factorydefinitions.FactoryConfig) (map[string]any, error) {
	return map[string]any{"name": factory.Name}, nil
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
