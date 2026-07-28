package http

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestLoadDocumentRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := LoadDocumentRequestFromHTTP(LoadDocumentInput{
		Path:            "/home/operator/.you-agent-factory/config.json",
		RequireExisting: true,
	})
	if err != nil {
		t.Fatalf("LoadDocumentRequestFromHTTP: %v", err)
	}
	if request.Path != "/home/operator/.you-agent-factory/config.json" || !request.RequireExisting {
		t.Fatalf("request = %#v, want load-document root request", request)
	}
}

func TestLoadDocumentRequestFromHTTP_RejectsInvalidPathBeforeRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input LoadDocumentInput
	}{
		{name: "missing path", input: LoadDocumentInput{RequireExisting: true}},
		{name: "blank path", input: LoadDocumentInput{Path: "   "}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadDocumentRequestFromHTTP(test.input)
			if err == nil || !IsLoadDocumentBadRequest(err) {
				t.Fatalf("LoadDocumentRequestFromHTTP = %v, want typed bad request", err)
			}
		})
	}
}

func TestLoadDocumentResponseToHTTP_MatchesGlobalConfigRepresentation(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join(
		"..",
		"..",
		"testdata",
		"fixtures",
		"valid",
		"worker-presets-canonicalized.json",
	)
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", fixturePath, err)
	}
	config, err := globalconfig.Decode(fixture)
	if err != nil {
		t.Fatalf("globalconfig.Decode() = %v", err)
	}

	document := documentFromConfigForTest(config)
	result := operatorsettings.LoadDocumentResult{
		Found:    true,
		Path:     "/tmp/config.json",
		Document: document,
	}
	response := LoadDocumentResponseToHTTP(result)

	encoded, err := globalconfig.Encode(config)
	if err != nil {
		t.Fatalf("globalconfig.Encode() = %v", err)
	}
	wantConfig, err := globalconfig.Decode(encoded)
	if err != nil {
		t.Fatalf("globalconfig.Decode(encoded) = %v", err)
	}
	want := documentToGlobalConfig(documentFromConfigForTest(wantConfig))

	if !response.Found || response.Path != "/tmp/config.json" {
		t.Fatalf("response metadata = found=%v path=%q, want found true and path", response.Found, response.Path)
	}
	if !reflect.DeepEqual(response.Document, want) {
		t.Fatalf("response.Document = %#v, want %#v", response.Document, want)
	}
}

func documentFromConfigForTest(config operatorsettings.Config) operatorsettings.Document {
	document := operatorsettings.Document{
		BackendScopeID: config.BackendScopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		Runtime: operatorsettings.DocumentRuntimeSettings{
			Logging: operatorsettings.DocumentRuntimeArtifactSettings(config.Runtime.Logging),
			Metrics: operatorsettings.DocumentRuntimeArtifactSettings(config.Runtime.Metrics),
		},
	}
	if config.WorkerPresets != nil {
		document.WorkerPresets = make([]operatorsettings.DocumentWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			document.WorkerPresets[i] = operatorsettings.DocumentWorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return document
}
