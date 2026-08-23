package capture_test

import (
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

type runtimeDefinitions struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func (d runtimeDefinitions) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := d.workers[name]
	return worker, ok
}

func (d runtimeDefinitions) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := d.workstations[name]
	return workstation, ok
}

func TestCaptureLoadedUsesOneCanonicalEffectiveRuntimeDefinition(t *testing.T) {
	t.Parallel()

	authored, runtime := canonicalRuntimeFixtures()

	loaded, err := loadedsource.New("factory", authored, runtime, nil)
	if err != nil {
		t.Fatalf("construct loaded source: %v", err)
	}
	capture := snapshotsportabilitycapture.NewLoaded(factorysnapshot.ObjectFromFactoryConfig)
	loadedSnapshot, err := capture(loaded, "", nil)
	if err != nil {
		t.Fatalf("capture loaded source: %v", err)
	}
	explicitSnapshot, err := capture(
		snapshotsportabilitycapture.NewExplicitSource("factory", authored, runtime),
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("capture explicit source: %v", err)
	}

	loadedMetadata := snapshotMetadata(t, loadedSnapshot)
	explicitMetadata := snapshotMetadata(t, explicitSnapshot)
	for _, key := range []string{
		"factory_hash", "workers_hash", "workstations_hash", "runtime_config_hash",
	} {
		if loadedMetadata[key] != explicitMetadata[key] {
			t.Fatalf(
				"%s differs for equivalent effective sources: loaded=%q explicit=%q",
				key, loadedMetadata[key], explicitMetadata[key],
			)
		}
	}
	if warnings := recordings.FactoryMetadataWarnings(loadedSnapshot, explicitSnapshot); len(warnings) != 0 {
		t.Fatalf("equivalent effective sources produced metadata warnings: %#v", warnings)
	}
}

func TestCaptureLoadedHashDomainsRemainSensitiveToEffectiveChanges(t *testing.T) {
	t.Parallel()

	baselineFactory, baselineRuntime := canonicalRuntimeFixtures()
	baseline := captureExplicitMetadata(t, baselineFactory, baselineRuntime)

	tests := []struct {
		name   string
		keys   []string
		change func(*factorydefinitions.FactoryConfig, runtimeDefinitions)
	}{
		{
			name: "factory definition",
			keys: []string{"factory_hash", "runtime_config_hash"},
			change: func(factory *factorydefinitions.FactoryConfig, _ runtimeDefinitions) {
				factory.Project = "changed-project"
			},
		},
		{
			name: "worker definition",
			keys: []string{"factory_hash", "workers_hash", "runtime_config_hash"},
			change: func(_ *factorydefinitions.FactoryConfig, runtime runtimeDefinitions) {
				runtime.workers["worker"].Body = "changed worker body"
			},
		},
		{
			name: "workstation definition",
			keys: []string{"factory_hash", "workstations_hash", "runtime_config_hash"},
			change: func(_ *factorydefinitions.FactoryConfig, runtime runtimeDefinitions) {
				runtime.workstations["station"].PromptTemplate = "changed station prompt"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, runtime := canonicalRuntimeFixtures()
			test.change(factory, runtime)
			got := captureExplicitMetadata(t, factory, runtime)
			for _, key := range test.keys {
				if got[key] == baseline[key] {
					t.Fatalf(
						"%s = %q after effective change, want a value different from %q",
						key, got[key], baseline[key],
					)
				}
			}
		})
	}
}

func canonicalRuntimeFixtures() (*factorydefinitions.FactoryConfig, runtimeDefinitions) {
	return &factorydefinitions.FactoryConfig{
			Name: "factory",
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				Name: "worker",
				Type: "AGENT_WORKER",
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				ID:             "station-id",
				Name:           "station",
				Type:           "AGENT_RUN",
				WorkerTypeName: "worker",
				Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				StopWords:      []string{"authored"},
				Env:            map[string]string{"AUTHORED": "true"},
			}},
		}, runtimeDefinitions{
			workers: map[string]*factorydefinitions.FactoryWorkerConfig{
				"worker": {
					Name:             "worker",
					Body:             "worker body",
					PromptSourcePath: "workers/worker/AGENTS.md",
				},
			},
			workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
				"station": {
					Name:             "station",
					Body:             "station body",
					PromptTemplate:   "station prompt",
					PromptSourcePath: "workstations/station/AGENTS.md",
					StopWords:        []string{"runtime"},
					Env:              map[string]string{"RUNTIME": "true"},
				},
			},
		}
}

func captureExplicitMetadata(
	t *testing.T,
	factory *factorydefinitions.FactoryConfig,
	runtime runtimeDefinitions,
) map[string]string {
	t.Helper()
	capture := snapshotsportabilitycapture.NewLoaded(factorysnapshot.ObjectFromFactoryConfig)
	snapshot, err := capture(
		snapshotsportabilitycapture.NewExplicitSource("factory", factory, runtime),
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("capture explicit source: %v", err)
	}
	return snapshotMetadata(t, snapshot)
}

func snapshotMetadata(t *testing.T, snapshot *factorydefinitions.FactorySnapshot) map[string]string {
	t.Helper()
	var object struct {
		Metadata map[string]string `json:"metadata"`
	}
	if snapshot == nil {
		t.Fatal("snapshot = nil")
	}
	if err := json.Unmarshal([]byte(*snapshot), &object); err != nil {
		t.Fatalf("decode snapshot metadata: %v", err)
	}
	return object.Metadata
}
