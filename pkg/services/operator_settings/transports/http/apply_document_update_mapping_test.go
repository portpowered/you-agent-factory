package http

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestApplyDocumentUpdateRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	provider := "codex"
	model := "gpt-5"
	request, err := ApplyDocumentUpdateRequestFromHTTP(ApplyDocumentUpdateInput{
		Path:                 "/home/operator/.you-agent-factory/config.json",
		ExpectedBackendScope: "local-00000000-0000-4000-8000-000000000010",
		Provider:             &provider,
		Model:                &model,
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdateRequestFromHTTP: %v", err)
	}
	if request.Path != "/home/operator/.you-agent-factory/config.json" {
		t.Fatalf("request.Path = %q, want config path", request.Path)
	}
	if request.ExpectedBackendScope != "local-00000000-0000-4000-8000-000000000010" {
		t.Fatalf("request.ExpectedBackendScope = %q, want backend scope", request.ExpectedBackendScope)
	}
	if request.ProviderModel.Provider == nil || *request.ProviderModel.Provider != "codex" {
		t.Fatalf("request.ProviderModel.Provider = %#v, want codex", request.ProviderModel.Provider)
	}
	if request.ProviderModel.Model == nil || *request.ProviderModel.Model != "gpt-5" {
		t.Fatalf("request.ProviderModel.Model = %#v, want gpt-5", request.ProviderModel.Model)
	}
}

func TestApplyDocumentUpdateRequestFromHTTP_RejectsInvalidPathBeforeRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input ApplyDocumentUpdateInput
	}{
		{name: "missing path", input: ApplyDocumentUpdateInput{Model: stringPointer("gpt-5")}},
		{name: "blank path", input: ApplyDocumentUpdateInput{Path: "   ", Model: stringPointer("gpt-5")}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ApplyDocumentUpdateRequestFromHTTP(test.input)
			if err == nil || !IsApplyDocumentUpdateBadRequest(err) {
				t.Fatalf("ApplyDocumentUpdateRequestFromHTTP = %v, want typed bad request", err)
			}
		})
	}
}

func TestApplyDocumentUpdateRequestFromHTTP_RejectsMissingProviderModelBeforeRoot(t *testing.T) {
	t.Parallel()

	_, err := ApplyDocumentUpdateRequestFromHTTP(ApplyDocumentUpdateInput{
		Path: "/tmp/config.json",
	})
	if err == nil {
		t.Fatal("ApplyDocumentUpdateRequestFromHTTP = nil, want malformed validation failure")
	}
}

func TestApplyDocumentUpdateResponseToHTTP_MatchesGlobalConfigRepresentation(t *testing.T) {
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
	result := operatorsettings.ApplyDocumentUpdateResult{
		Path:      "/tmp/config.json",
		Persisted: true,
		Document:  document,
	}
	response := ApplyDocumentUpdateResponseToHTTP(result)

	encoded, err := globalconfig.Encode(config)
	if err != nil {
		t.Fatalf("globalconfig.Encode() = %v", err)
	}
	wantConfig, err := globalconfig.Decode(encoded)
	if err != nil {
		t.Fatalf("globalconfig.Decode(encoded) = %v", err)
	}
	want := documentToGlobalConfig(documentFromConfigForTest(wantConfig))

	if response.Path != "/tmp/config.json" || !response.Persisted {
		t.Fatalf("response metadata = path=%q persisted=%v, want persisted update", response.Path, response.Persisted)
	}
	if !reflect.DeepEqual(response.Document, want) {
		t.Fatalf("response.Document = %#v, want %#v", response.Document, want)
	}
}

func stringPointer(value string) *string {
	return &value
}
