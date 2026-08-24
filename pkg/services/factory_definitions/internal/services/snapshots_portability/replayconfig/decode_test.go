package replayconfig_test

import (
	"encoding/json"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilityreplayconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig"
)

func TestDecodeBuildsReplayLookupsFromSnapshot(t *testing.T) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name":             "recorded",
		"factoryDirectory": "/recorded/factory",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		Name: "recorded",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name:             "writer",
			ExecutorProvider: "  codex  ",
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			ID:   "write-id",
			Name: "write",
			Type: "LOGICAL_MOVE",
		}},
	}

	got, err := snapshotsportabilityreplayconfig.Decode(snapshot, func([]byte) (*factorydefinitions.FactoryConfig, error) {
		return factoryConfig, nil
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.FactoryConfig() != factoryConfig {
		t.Fatal("FactoryConfig did not return the decoded Factory config")
	}
	if got.FactoryDir() != "/recorded/factory" || got.RuntimeBaseDir() != "/recorded/factory" {
		t.Fatalf("replay paths = (%q, %q)", got.FactoryDir(), got.RuntimeBaseDir())
	}
	worker, ok := got.Worker("writer")
	if !ok || worker.ExecutorProvider != "codex" {
		t.Fatalf("Worker(writer) = (%#v, %v)", worker, ok)
	}
	workstation, ok := got.Workstation("write")
	if !ok || workstation.ID != "write-id" {
		t.Fatalf("Workstation(write) = (%#v, %v)", workstation, ok)
	}
	byID, ok := got.WorkstationByID("write-id")
	if !ok || byID.Name != "write" {
		t.Fatalf("WorkstationByID(write-id) = (%#v, %v)", byID, ok)
	}
}

func TestDecodeRejectsMissingInputs(t *testing.T) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "recorded"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	tests := []struct {
		name     string
		snapshot *factorydefinitions.FactorySnapshot
		decoder  factorydefinitions.FactoryConfigJSONDecoder
		want     string
	}{
		{name: "snapshot", want: "factory is required"},
		{name: "decoder", snapshot: snapshot, want: "decoder is required"},
		{
			name:     "decoded config",
			snapshot: snapshot,
			decoder: func([]byte) (*factorydefinitions.FactoryConfig, error) {
				return nil, nil
			},
			want: "config is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := snapshotsportabilityreplayconfig.Decode(test.snapshot, test.decoder)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("Decode error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDecodeMaterializesTypedRedactionMarkersWithoutRestoringValues(t *testing.T) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name": "recorded",
		"workstations": []any{map[string]any{
			"name": "write",
			"body": map[string]any{
				"redacted":   true,
				"provenance": "DECLARED_SECRET",
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	var decodedPayload []byte
	_, err = snapshotsportabilityreplayconfig.Decode(snapshot, func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
		decodedPayload = append([]byte(nil), payload...)
		return &factorydefinitions.FactoryConfig{Name: "recorded"}, nil
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var document struct {
		Workstations []struct {
			Body any `json:"body"`
		} `json:"workstations"`
	}
	if err := json.Unmarshal(decodedPayload, &document); err != nil {
		t.Fatalf("decode materialized payload: %v", err)
	}
	if len(document.Workstations) != 1 || document.Workstations[0].Body != "" {
		t.Fatalf("materialized redacted body = %#v, want empty string", document.Workstations)
	}
}
