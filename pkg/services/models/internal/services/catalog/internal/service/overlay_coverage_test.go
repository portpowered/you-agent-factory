package service_test

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
)

func TestCatalogProjectsFileAndLocalOperatorSources(t *testing.T) {
	t.Parallel()

	fileSource := " file:///models/embed.gguf "
	localSource := " ./models/tts.gguf "
	fileScopes := newRuntimeScopes(t, "catalog-file-source")
	fileScope := openCatalogScopeWithOverlays(t, fileScopes, map[string]models.ModelOverlay{
		"file-embed": {
			Source:     &fileSource,
			Backend:    stringPointer("localai-llamacpp"),
			LoadPolicy: loadPolicyPointer(models.LoadPolicyOnDemand),
			Operations: []string{models.OperationEMBED},
		},
	})
	localScopes := newRuntimeScopes(t, "catalog-local-source")
	localScope := openCatalogScopeWithOverlays(t, localScopes, map[string]models.ModelOverlay{
		"local-tts": {
			Source:     &localSource,
			Backend:    stringPointer("localai-vibevoice"),
			LoadPolicy: loadPolicyPointer(models.LoadPolicyKeepWarm),
			Operations: []string{models.OperationTTS},
		},
	})

	for _, test := range []struct {
		name      string
		service   catalog.Service
		scope     models.RuntimeScopeRef
		model     string
		provider  string
		reference string
		operation string
	}{
		{
			name: "file URI", service: newCatalogService(t, fileScopes), scope: fileScope, model: "file-embed",
			provider: string(models.ModelReferenceSourceFileURI), reference: "file:///models/embed.gguf", operation: models.OperationEMBED,
		},
		{
			name: "local path", service: newCatalogService(t, localScopes), scope: localScope, model: "local-tts",
			provider: string(models.ModelReferenceSourceLocalPath), reference: "./models/tts.gguf", operation: models.OperationTTS,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result, err := test.service.GetCatalogModel(context.Background(), models.GetModelRequest{
				Scope: test.scope, Name: test.model, Operation: test.operation,
			})
			if err != nil {
				t.Fatalf("GetCatalogModel: %v", err)
			}
			if len(result.Model.Sources) != 1 ||
				result.Model.Sources[0].Provider != test.provider ||
				result.Model.Sources[0].Reference != test.reference {
				t.Fatalf("sources = %#v, want provider=%q reference=%q", result.Model.Sources, test.provider, test.reference)
			}
		})
	}
}

func TestCatalogSanitizesListAndDetailReadinessFailures(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-readiness-errors")
	scope := publicScope(t, openCatalogScope(t, scopes, "readiness-error-model", models.OperationOMNI))
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{}, errors.New("backend inspection failed")
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}

	if _, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope}); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog error = %v, want ErrUnavailable", err)
	}
	if _, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{Scope: scope, Name: "readiness-error-model"}); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("GetCatalogModel error = %v, want ErrUnavailable", err)
	}
}

func TestCatalogRejectsInvalidOperatorOverlayDefinitions(t *testing.T) {
	t.Parallel()

	valid := func() models.ModelOverlay {
		return models.ModelOverlay{
			Source:     stringPointer("file:///models/overlay.gguf"),
			Backend:    stringPointer("localai-test"),
			LoadPolicy: loadPolicyPointer(models.LoadPolicyOnDemand),
			Operations: []string{models.OperationEMBED},
		}
	}
	blank := "  "
	invalidLoadPolicy := models.LoadPolicy("not-a-load-policy")
	tests := []struct {
		name    string
		overlay map[string]models.ModelOverlay
		field   string
	}{
		{name: "unsafe name", overlay: map[string]models.ModelOverlay{"bad/name": {}}, field: "name"},
		{name: "normalized duplicate", overlay: map[string]models.ModelOverlay{
			"duplicate":   valid(),
			" DUPLICATE ": valid(),
		}, field: "name"},
		{name: "blank source", overlay: map[string]models.ModelOverlay{
			models.BuiltInModelNameLLM: {Source: &blank},
		}, field: "source"},
		{name: "blank backend", overlay: map[string]models.ModelOverlay{
			models.BuiltInModelNameLLM: {Backend: &blank},
		}, field: "backend"},
		{name: "invalid load policy", overlay: map[string]models.ModelOverlay{
			models.BuiltInModelNameLLM: {LoadPolicy: &invalidLoadPolicy},
		}, field: "loadPolicy"},
		{name: "unsupported operation", overlay: map[string]models.ModelOverlay{
			models.BuiltInModelNameLLM: {Operations: []string{"not-an-operation"}},
		}, field: "operations"},
		{name: "incomplete new model", overlay: map[string]models.ModelOverlay{
			"new-model": {Backend: stringPointer("localai-test")},
		}, field: "source"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scopes := newRuntimeScopes(t, "catalog-invalid-overlay-"+test.name)
			scope := openCatalogScopeWithOverlays(t, scopes, test.overlay)
			service := newCatalogService(t, scopes)
			if _, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope}); err == nil {
				t.Fatal("ListCatalog error = nil, want ModelConfigurationFailure")
			} else {
				var failure models.ModelConfigurationFailure
				if !errors.As(err, &failure) {
					t.Fatalf("ListCatalog error = %v, want ModelConfigurationFailure", err)
				}
				if failure.Field != test.field {
					t.Fatalf("configuration failure field = %q, want %q", failure.Field, test.field)
				}
			}
		})
	}
}

func TestCatalogRejectsInvalidOperatorOverlaysWithFieldDiagnostics(t *testing.T) {
	t.Parallel()

	valid := func() models.ModelOverlay {
		source := "file:///models/custom.gguf"
		backend := "localai-test"
		loadPolicy := models.LoadPolicyOnDemand
		return models.ModelOverlay{
			Source:     &source,
			Backend:    &backend,
			LoadPolicy: &loadPolicy,
			Operations: []string{models.OperationOMNI},
		}
	}

	blankSource := "   "
	blankBackend := "\t"
	invalidLoadPolicy := models.LoadPolicy("ALWAYS")
	completeCustom := valid()
	completeCustom.Operations = nil

	tests := []invalidCatalogOverlayCase{
		{
			name:      "invalid model name",
			overlays:  map[string]models.ModelOverlay{"bad/name": valid()},
			modelName: "bad/name",
			field:     "name",
		},
		{
			name: "duplicate normalized names",
			overlays: map[string]models.ModelOverlay{
				"Alias":   valid(),
				" alias ": valid(),
			},
			modelName: "Alias",
			field:     "name",
		},
		{
			name:      "built-in blank source",
			overlays:  map[string]models.ModelOverlay{"llm": {Source: &blankSource}},
			modelName: "llm",
			field:     "source",
		},
		{
			name:      "built-in blank backend",
			overlays:  map[string]models.ModelOverlay{"llm": {Backend: &blankBackend}},
			modelName: "llm",
			field:     "backend",
		},
		{
			name:      "invalid load policy",
			overlays:  map[string]models.ModelOverlay{"llm": {LoadPolicy: &invalidLoadPolicy}},
			modelName: "llm",
			field:     "loadPolicy",
		},
		{
			name:      "unsupported operation",
			overlays:  map[string]models.ModelOverlay{"llm": {Operations: []string{"classify"}}},
			modelName: "llm",
			field:     "operations",
		},
		{
			name:      "custom operation is required",
			overlays:  map[string]models.ModelOverlay{"custom": completeCustom},
			modelName: "custom",
			field:     "operations",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			assertInvalidCatalogOverlay(t, test)
		})
	}
}

type invalidCatalogOverlayCase struct {
	name      string
	overlays  map[string]models.ModelOverlay
	modelName string
	field     string
}

func assertInvalidCatalogOverlay(t *testing.T, test invalidCatalogOverlayCase) {
	t.Helper()
	scopes := newRuntimeScopes(t, "catalog-overlay-"+test.name)
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig:  func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		OperatorModels: test.overlays,
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	service := newCatalogService(t, scopes)
	_, err = service.ListCatalog(context.Background(), models.ListModelsRequest{
		Scope: publicScope(t, privateRef),
	})

	var failure models.ModelConfigurationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("ListCatalog error = %v, want ModelConfigurationFailure", err)
	}
	if !errors.Is(err, models.ErrModelConfigurationInvalid) {
		t.Fatalf("ListCatalog error = %v, want ErrModelConfigurationInvalid", err)
	}
	if failure.ModelName != test.modelName || failure.Field != test.field {
		t.Fatalf("configuration failure = %#v, want model %q field %q", failure, test.modelName, test.field)
	}
}
