package service_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/internal/service"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestApplyDocumentUpdate_ModelOnlyUpdatePreservesProviderAndReturnsValidatedDocument(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	initialBytes := readFixture(t, "valid/load-defaults.json")
	files := map[string][]byte{path: initialBytes}
	service := newDocumentUpdateService(t, files)

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
	if updated.Persisted {
		t.Fatal("Persisted = true, want false before atomic persist story")
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
	if updated.Document.Runtime != operatorsettings.EmptyDocument().Runtime {
		t.Fatalf("Runtime = %#v, want production defaults", updated.Document.Runtime)
	}
	if !reflect.DeepEqual(files[path], initialBytes) {
		t.Fatal("destination bytes changed after semantic-only update")
	}
}

func TestApplyDocumentUpdate_UnsupportedProviderPreservesDocumentAndDestinationBytes(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	initialBytes := readFixture(t, "valid/load-defaults.json")
	files := map[string][]byte{path: append([]byte(nil), initialBytes...)}
	service := newDocumentUpdateService(t, files)

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
	if !reflect.DeepEqual(files[path], initialBytes) {
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
	files := map[string][]byte{path: append([]byte(nil), initialBytes...)}
	service := newDocumentUpdateService(t, files)

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
	if !reflect.DeepEqual(files[path], initialBytes) {
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
	files := map[string][]byte{path: readFixture(t, "valid/load-defaults.json")}
	service := newDocumentUpdateService(t, files)

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

func newDocumentUpdateService(t *testing.T, files map[string][]byte) *internalservice.Service {
	t.Helper()

	return internalservice.New(
		&mapFileSystem{files: files},
		func(string, string) (operatorsettings.TemporaryFile, error) {
			t.Fatal("temp-file creation is unexpected during semantic update")
			return nil, errors.New("unexpected temp-file creation")
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		controlledProviderCatalog,
	)
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
