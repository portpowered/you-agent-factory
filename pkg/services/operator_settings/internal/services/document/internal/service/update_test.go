package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/internal/service"
)

func TestMergeDocumentProviderModelPreservesDetachedMessaging(t *testing.T) {
	t.Parallel()
	enabled, days, hop := false, 2, 0
	keys := []string{"tag:project"}
	document := operatorsettings.Document{
		Messaging: &operatorsettings.MessagingSettings{
			Enabled: &enabled, RetentionDays: &days,
			Policy: &operatorsettings.MessagingPolicy{ScopeLabelKeys: &keys},
			Limits: &operatorsettings.MessagingLimits{MaxHop: &hop},
		},
	}
	// Model-only merge needs no effects. Document persistence, decoding and
	// provider catalogs remain controlled ports outside this component witness.
	service := internalservice.NewWithPreserver(nil, nil, nil, nil, nil, nil, nil)
	model := "new-model"
	updated, err := service.MergeDocumentProviderModel(document, operatorsettings.DocumentProviderModelUpdate{Model: &model})
	if err != nil || !reflect.DeepEqual(document.Messaging, updated.Messaging) || updated.Defaults.WorkerModel != model {
		t.Fatalf("merge lost Messaging or model update: %v", err)
	}
	*updated.Messaging.Enabled = true
	(*updated.Messaging.Policy.ScopeLabelKeys)[0] = "foreign"
	*updated.Messaging.Limits.MaxHop = 1
	if *document.Messaging.Enabled || keys[0] != "tag:project" || hop != 0 {
		t.Fatal("returned document aliases authored Messaging")
	}
	invalid := operatorsettings.Document{Messaging: &operatorsettings.MessagingSettings{InvalidJSON: []byte(`"bad"`)}}
	updated, err = service.MergeDocumentProviderModel(invalid, operatorsettings.DocumentProviderModelUpdate{Model: &model})
	if err != nil || string(updated.Messaging.InvalidJSON) != `"bad"` {
		t.Fatalf("unrelated model update discarded invalid Messaging: %v", err)
	}
	cloned := updated.Clone()
	cloned.Messaging.InvalidJSON[1] = 'x'
	if string(updated.Messaging.InvalidJSON) != `"bad"` {
		t.Fatal("document clone aliases preserved invalid section")
	}
}

func TestApplyDocumentUpdate_ModelOnlyUpdatePreservesProviderAndReturnsValidatedDocument(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	service := newDocumentUpdateService(t)

	nextModel := "claude-opus"
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: path,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &nextModel,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if !updated.Persisted {
		t.Fatal("Persisted = false, want true after atomic persist")
	}
	if updated.Path != path {
		t.Fatalf("Path = %q, want %q", updated.Path, path)
	}
	if updated.Document.Defaults.WorkerModel != nextModel {
		t.Fatalf("WorkerModel = %q, want %q", updated.Document.Defaults.WorkerModel, nextModel)
	}
	if updated.Document.Defaults.WorkerModelProvider != "claude" {
		t.Fatalf("WorkerModelProvider = %q, want unchanged claude", updated.Document.Defaults.WorkerModelProvider)
	}
	if updated.Document.Runtime != operatorsettings.EmptyDocument.Runtime {
		t.Fatalf("Runtime = %#v, want production defaults", updated.Document.Runtime)
	}

	reloaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() after persist = %v", err)
	}
	if reloaded.Document.Defaults.WorkerModel != nextModel {
		t.Fatalf("reloaded WorkerModel = %q, want %q", reloaded.Document.Defaults.WorkerModel, nextModel)
	}
}

func TestApplyDocumentUpdate_PreservesModelsOverlayAndUnrelatedSettings(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"defaults":{"workerModelProvider":"claude","workerModel":"claude-sonnet"},
		"models":{"llm":{"source":"hf://custom/gemma"}}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	service := newDocumentUpdateService(t)

	model := "claude-opus"
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:          path,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{Model: &model},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if updated.Document.Defaults.WorkerModel != model || updated.Document.Defaults.WorkerModelProvider != "claude" {
		t.Fatalf("updated defaults = %#v, want model update and preserved provider", updated.Document.Defaults)
	}
	entry := updated.Document.Models["llm"]
	if entry.Source == nil || *entry.Source != "hf://custom/gemma" {
		t.Fatalf("updated models = %#v, want preserved overlay", updated.Document.Models)
	}

	reloaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() after update = %v", err)
	}
	if reloaded.Document.Defaults.WorkerModel != model || reloaded.Document.Models["llm"].Source == nil || *reloaded.Document.Models["llm"].Source != "hf://custom/gemma" {
		t.Fatalf("reloaded document = %#v, want model overlay and defaults preserved", reloaded.Document)
	}
}

func TestApplyDocumentUpdate_UnsupportedProviderPreservesDocumentAndDestinationBytes(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	initialBytes := readFixture(t, "valid/load-defaults.json")
	service := newDocumentUpdateService(t)

	unsupported := "other"
	_, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: path,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &unsupported,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("ApplyDocumentUpdate() = %v, want ErrDocumentUnsupported", err)
	}
	if failure, ok := err.(operatorsettings.DocumentFailure); !ok ||
		failure.Kind != operatorsettings.DocumentFailureKindUnsupported {
		t.Fatalf("ApplyDocumentUpdate() = %#v, want unsupported DocumentFailure", err)
	}
	if !reflect.DeepEqual(readFixtureFromPath(t, path), initialBytes) {
		t.Fatal("destination bytes changed after unsupported provider update")
	}

	reloaded, loadErr := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if loadErr != nil {
		t.Fatalf("LoadDocument() after failed update = %v", loadErr)
	}
	if reloaded.Document.Defaults != (operatorsettings.DocumentDefaults{
		WorkerModelProvider: "claude",
		WorkerModel:         "claude-sonnet",
	}) {
		t.Fatalf("reloaded defaults = %#v, want unchanged fixture defaults", reloaded.Document.Defaults)
	}
}

func TestApplyDocumentUpdate_BackendScopeConflictPreservesDocumentAndDestinationBytes(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/backend-scope-sibling.json")
	initialBytes := readFixture(t, "valid/backend-scope-sibling.json")
	service := newDocumentUpdateService(t)

	model := "gpt-5"
	_, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 path,
		ExpectedBackendScope: "local-stale-scope",
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &model,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentConflict) {
		t.Fatalf("ApplyDocumentUpdate() = %v, want ErrDocumentConflict", err)
	}
	if failure, ok := err.(operatorsettings.DocumentFailure); !ok ||
		failure.Kind != operatorsettings.DocumentFailureKindConflict ||
		!strings.Contains(failure.Message, "backend scope mismatch") {
		t.Fatalf("ApplyDocumentUpdate() = %#v, want backend scope conflict", err)
	}
	if !reflect.DeepEqual(readFixtureFromPath(t, path), initialBytes) {
		t.Fatal("destination bytes changed after backend scope conflict")
	}

	reloaded, loadErr := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if loadErr != nil {
		t.Fatalf("LoadDocument() after conflict = %v", loadErr)
	}
	if reloaded.Document.BackendScopeID != "local-11111111-1111-4111-8111-111111111111" {
		t.Fatalf("BackendScopeID = %q, want unchanged persisted scope", reloaded.Document.BackendScopeID)
	}
}

func TestApplyDocumentUpdate_ProviderUpdateCanonicalizesThroughCatalog(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	service := newDocumentUpdateService(t)

	alias := " openai "
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: path,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &alias,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if updated.Document.Defaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("WorkerModelProvider = %q, want catalog canonical CODEX", updated.Document.Defaults.WorkerModelProvider)
	}
	if updated.Document.Defaults.WorkerModel != "claude-sonnet" {
		t.Fatalf("WorkerModel = %q, want unchanged claude-sonnet", updated.Document.Defaults.WorkerModel)
	}
}

func newDocumentUpdateService(t *testing.T) *internalservice.Service {
	t.Helper()

	return newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)
}

func controlledProviderCatalog(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		return "", false
	}
}
