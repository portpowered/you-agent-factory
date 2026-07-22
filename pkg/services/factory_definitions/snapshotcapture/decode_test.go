package snapshotcapture_test

import (
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture"
)

type mutableSource struct {
	source
	runtimeBaseDir string
}

func (s *mutableSource) RuntimeBaseDir() string {
	return s.runtimeBaseDir
}

func (s *mutableSource) SetRuntimeBaseDir(runtimeBaseDir string) {
	s.runtimeBaseDir = runtimeBaseDir
}

func (*mutableSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}

func (*mutableSource) MutateWorkers(func(*factorydefinitions.FactoryWorkerConfig) error) error {
	return nil
}

func TestNewJSONDecoderCapturesDetachedSnapshot(t *testing.T) {
	decoder := snapshotcapture.NewJSONDecoder(func(data []byte) (map[string]any, error) {
		return map[string]any{"name": string(data)}, nil
	})

	snapshot, err := decoder([]byte("example"))
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	var got map[string]any
	if err := snapshot.Decode(&got); err != nil {
		t.Fatalf("decode captured snapshot: %v", err)
	}
	if got["name"] != "example" {
		t.Fatalf("snapshot name = %#v, want example", got["name"])
	}
}

func TestDecodeJSONRequiresBoundaryDecoder(t *testing.T) {
	_, err := snapshotcapture.DecodeJSON[map[string]any]([]byte(`{}`), nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("DecodeJSON error = %v, want decoder requirement", err)
	}
}

func TestNewDirectoryLoaderUsesInjectedOwnerCapabilities(t *testing.T) {
	loaded := &mutableSource{
		source: source{
			dir:     "/factory",
			factory: &factorydefinitions.FactoryConfig{Name: "example"},
		},
		runtimeBaseDir: "/factory",
	}
	var captured factorydefinitions.FactorySnapshotSource
	load := snapshotcapture.NewDirectoryLoader(
		func(
			factoryDir string,
			workstationLoader factorydefinitions.WorkstationLoader,
		) (factorydefinitions.MutableLoadedFactorySource, error) {
			if factoryDir != "/factory" {
				t.Fatalf("factoryDir = %q, want /factory", factoryDir)
			}
			if workstationLoader != nil {
				t.Fatal("directory loader supplied unexpected workstation loader")
			}
			return loaded, nil
		},
		func(
			source factorydefinitions.FactorySnapshotSource,
			sourceDirectory string,
			metadata map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			captured = source
			if sourceDirectory != "/factory" {
				t.Fatalf("sourceDirectory = %q, want /factory", sourceDirectory)
			}
			if metadata != nil {
				t.Fatalf("metadata = %#v, want nil", metadata)
			}
			return factorydefinitions.NewFactorySnapshot(map[string]any{"name": "example"})
		},
	)

	if _, err := load("/factory"); err != nil {
		t.Fatalf("load directory snapshot: %v", err)
	}
	if captured != loaded {
		t.Fatal("capturer did not receive the loaded Factory source")
	}
}

func TestLoadDirectoryRequiresInjectedCapabilities(t *testing.T) {
	loaded := &mutableSource{source: source{dir: "/factory"}}
	tests := []struct {
		name    string
		load    factorydefinitions.LoadedFactoryLoader
		capture factorydefinitions.LoadedFactorySnapshotCapturer
		want    string
	}{
		{name: "loader", want: "loader is required"},
		{
			name: "capturer",
			load: func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				return loaded, nil
			},
			want: "capturer is required",
		},
		{
			name: "loader error",
			load: func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				return nil, errors.New("load failed")
			},
			capture: func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
				t.Fatal("capturer called after loader failure")
				return nil, nil
			},
			want: "load failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := snapshotcapture.LoadDirectory("/factory", test.load, test.capture)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadDirectory error = %v, want containing %q", err, test.want)
			}
		})
	}
}
