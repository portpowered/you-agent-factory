package operatorsettings

import (
	"errors"
	"reflect"
	"testing"
)

func TestACPIntegrationSemanticUpdatesPreserveUnrelatedSettings(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	document := ConfigDocument{config: Config{
		BackendScopeID: "local-scope",
		Defaults:       Defaults{WorkerModelProvider: "CODEX", WorkerModel: "model"},
		WorkerPresets:  []WorkerPreset{{ID: "build", ModelProvider: "CODEX"}},
	}}
	added, err := service.AddACPIntegration(document, ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "STDIO", Command: " cursor-agent acp ",
	})
	if err != nil {
		t.Fatalf("AddACPIntegration() error = %v", err)
	}
	got := added.FileConfig()
	if got.BackendScopeID != "local-scope" || got.Defaults != document.config.Defaults || !reflect.DeepEqual(got.WorkerPresets, document.config.WorkerPresets) {
		t.Fatalf("unrelated settings changed: %#v", got)
	}
	wantIntegration := ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	if !reflect.DeepEqual(got.Workers.ACP.Integrations, []ACPIntegration{wantIntegration}) {
		t.Fatalf("integrations = %#v, want %#v", got.Workers.ACP.Integrations, wantIntegration)
	}

	deleted, err := service.DeleteACPIntegration(added, " cursor-acp ")
	if err != nil {
		t.Fatalf("DeleteACPIntegration() error = %v", err)
	}
	if len(deleted.FileConfig().Workers.ACP.Integrations) != 0 {
		t.Fatalf("integrations after delete = %#v, want empty", deleted.FileConfig().Workers.ACP.Integrations)
	}
	if _, err := service.DeleteACPIntegration(deleted, "cursor-acp"); !errors.Is(err, ErrACPIntegrationNotFound) {
		t.Fatalf("DeleteACPIntegration(missing) error = %v, want ErrACPIntegrationNotFound", err)
	}
}

func TestACPIntegrationRejectsDuplicateAndMalformedProviderIdentities(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	base := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}}}}}
	if _, err := service.AddACPIntegration(base, ACPIntegration{
		ID: "entry-2", Name: "cursor-acp", Transport: "stdio", Command: "replacement",
	}); err == nil {
		t.Fatal("AddACPIntegration(duplicate name) error = nil")
	}
	if _, err := service.AddACPIntegration(ConfigDocument{}, ACPIntegration{
		ID: "entry-1", Name: "Cursor ACP", Transport: "stdio", Command: "cursor-agent acp",
	}); err == nil {
		t.Fatal("AddACPIntegration(malformed name) error = nil")
	}
}
