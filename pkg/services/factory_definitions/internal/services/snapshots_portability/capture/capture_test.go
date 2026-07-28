package capture_test

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
)

type source struct {
	dir     string
	factory *factorydefinitions.FactoryConfig
}

func (s source) FactoryDir() string {
	return s.dir
}

func (s source) FactoryConfig() *factorydefinitions.FactoryConfig {
	return s.factory
}

func (source) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (source) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func TestNewLoadedCapturesThroughInjectedRepresentationBoundary(t *testing.T) {
	t.Parallel()

	capture := snapshotsportabilitycapture.NewLoaded(
		func(factory *factorydefinitions.FactoryConfig) (map[string]any, error) {
			return map[string]any{"name": factory.Name}, nil
		},
	)

	snapshot, err := capture(
		source{
			dir:     "factory-dir",
			factory: &factorydefinitions.FactoryConfig{Name: "example"},
		},
		"",
		map[string]string{"workflow": "test"},
	)
	if err != nil {
		t.Fatalf("capture loaded snapshot: %v", err)
	}

	var object map[string]any
	if err := snapshot.Decode(&object); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if got := object["name"]; got != "example" {
		t.Fatalf("snapshot name = %#v, want example", got)
	}
	if got := object["factoryDirectory"]; got != "factory-dir" {
		t.Fatalf("factoryDirectory = %#v, want factory-dir", got)
	}
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata type = %T, want map[string]any", object["metadata"])
	}
	if got := metadata["workflow"]; got != "test" {
		t.Fatalf("metadata workflow = %#v, want test", got)
	}
	if got := metadata["source_format"]; got != factorydefinitions.ReplayV1SourceFormat {
		t.Fatalf("metadata source_format = %#v, want %q", got, factorydefinitions.ReplayV1SourceFormat)
	}
}

func TestCaptureLoadedRequiresRepresentationMapper(t *testing.T) {
	t.Parallel()

	factorySource := source{factory: &factorydefinitions.FactoryConfig{Name: "example"}}

	_, err := snapshotsportabilitycapture.CaptureLoaded(factorySource, "", nil, nil)
	if err == nil {
		t.Fatal("CaptureLoaded mapper error = nil, want error")
	}
	_, err = snapshotsportabilitycapture.CaptureLoaded(
		factorySource,
		"",
		nil,
		func(*factorydefinitions.FactoryConfig) (map[string]any, error) {
			return nil, errors.New("map failed")
		},
	)
	if err == nil || err.Error() != "encode factory snapshot: map failed" {
		t.Fatalf("CaptureLoaded mapper error = %v", err)
	}
}
