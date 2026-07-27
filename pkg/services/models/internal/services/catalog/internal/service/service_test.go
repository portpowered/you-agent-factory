package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestListCatalogClassifiesScopeBeforeProjection(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-service-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}

	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{},
	); !errors.Is(err, models.ErrRuntimeScopeInvalid) {
		t.Fatalf("ListCatalog empty scope error = %v, want ErrRuntimeScopeInvalid", err)
	}

	stale, err := (models.RuntimeScopeRef{}).Parse("stale")
	if err != nil {
		t.Fatalf("parse stale scope: %v", err)
	}
	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: stale},
	); !errors.Is(err, models.ErrRuntimeScopeStale) {
		t.Fatalf("ListCatalog stale scope error = %v, want ErrRuntimeScopeStale", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ListCatalog(
		ctx,
		models.ListModelsRequest{Scope: stale},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListCatalog canceled error = %v, want context.Canceled", err)
	}
}

func TestListCatalogRejectsUnavailableScopedConfiguration(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-unavailable-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return nil },
	})
	if err != nil {
		t.Fatalf("open unavailable scope: %v", err)
	}
	scope := publicScope(t, privateRef)
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}

	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: scope},
	); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog unavailable error = %v, want ErrUnavailable", err)
	}
}

func TestGetCatalogModelReturnsStableDetachedDetail(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-get-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{
					catalogWorker("zeta-worker", " scoped-model ", "summarize"),
					catalogWorker("alpha-worker", "SCOPED-MODEL", "generate"),
				},
				Resources: []models.RuntimeResource{{
					Name: "model-cache", Type: models.RuntimeResourceTypeModel,
					Model: "SCOPED-MODEL", Backend: "GGUF", Provider: "MODELSCOPE",
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	request := models.GetModelRequest{
		Scope: publicScope(t, privateRef), Name: "  scoped-model  ", Operation: "generate",
	}

	first, err := service.GetCatalogModel(context.Background(), request)
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	assertCatalogDetail(t, first.Model)
	second, err := service.GetCatalogModel(context.Background(), request)
	if err != nil {
		t.Fatalf("GetCatalogModel repeated: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated result differs:\nfirst:  %#v\nsecond: %#v", first, second)
	}

	first.Model.Name = "mutated"
	first.Model.Operations[0].Name = "mutated"
	first.Model.Operations[0].Inputs[0].ContentTypes[0] = "mutated"
	first.Model.Capabilities[0].Worker = "mutated"
	*first.Model.Capabilities[0].ModelProvider = "mutated"
	first.Model.Capabilities[0].ResourceNames[0] = "mutated"
	first.Model.Sources[0].Reference = "mutated"
	first.Model.Diagnostics["workers"] = "mutated"
	first.Model.ManagedRuntime.Diagnostics["sourceId"] = "mutated"

	afterMutation, err := service.GetCatalogModel(context.Background(), request)
	if err != nil {
		t.Fatalf("GetCatalogModel after mutation: %v", err)
	}
	assertCatalogDetail(t, afterMutation.Model)
}

func TestGetCatalogModelClassifiesLookupAndOperationFailures(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-get-errors-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{catalogWorker("worker", "known-model", "generate")},
			}
		},
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, privateRef)

	for _, test := range []struct {
		name    string
		request models.GetModelRequest
		want    error
	}{
		{name: "empty identity", request: models.GetModelRequest{Scope: scope}, want: models.ErrNotFound},
		{name: "unknown identity", request: models.GetModelRequest{Scope: scope, Name: "missing"}, want: models.ErrNotFound},
		{
			name: "unsupported operation",
			request: models.GetModelRequest{
				Scope: scope, Name: "known-model", Operation: "embed",
			},
			want: models.ErrUnsupportedOperation,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := service.GetCatalogModel(context.Background(), test.request); !errors.Is(err, test.want) {
				t.Fatalf("GetCatalogModel error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestConstructionRejectsMissingRuntimeScopes(t *testing.T) {
	t.Parallel()

	service, err := catalogwire.NewService(nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil) = (%#v, %v), want nil service and error", service, err)
	}
}

func catalogWorker(name, model, operation string) models.RuntimeWorker {
	return models.RuntimeWorker{
		Name: name, Type: models.RuntimeWorkerTypeInference,
		Model: model, ModelLocality: models.RuntimeModelLocalityLocal,
		ModelProvider: "configured-provider",
		Operations: []models.RuntimeOperation{{
			Name: operation,
			Inputs: []models.RuntimeOperationSlot{{
				Name: "input", ContentTypes: []string{
					models.RuntimeContentTypeText,
					models.RuntimeContentTypeAudio,
				},
			}},
		}},
		Resources: []models.RuntimeResource{{Name: "model-cache"}},
	}
}

func assertCatalogDetail(t *testing.T, detail models.Detail) {
	t.Helper()
	if detail.Name != "scoped-model" {
		t.Fatalf("model name = %q, want scoped-model", detail.Name)
	}
	if got := []string{detail.Operations[0].Name, detail.Operations[1].Name}; !reflect.DeepEqual(
		got,
		[]string{"generate", "summarize"},
	) {
		t.Fatalf("operations = %#v, want stable generate/summarize order", detail.Operations)
	}
	if got := detail.Operations[0].Inputs[0].ContentTypes; !reflect.DeepEqual(
		got,
		[]string{models.RuntimeContentTypeAudio, models.RuntimeContentTypeText},
	) {
		t.Fatalf("content types = %#v, want stable AUDIO/TEXT order", got)
	}
	if len(detail.Capabilities) != 2 ||
		detail.Capabilities[0].Worker != "alpha-worker" ||
		detail.Capabilities[0].ModelProvider == nil ||
		*detail.Capabilities[0].ModelProvider != "configured-provider" {
		t.Fatalf("capabilities = %#v, want stable configured bindings", detail.Capabilities)
	}
	if len(detail.Sources) != 1 ||
		detail.Sources[0].Provider != "MANAGED_MIRROR" ||
		detail.Sources[0].Reference != "managed-mirror:SCOPED-MODEL" {
		t.Fatalf("sources = %#v, want configured managed source", detail.Sources)
	}
	if detail.Diagnostics["workers"] != "alpha-worker,zeta-worker" ||
		detail.ManagedRuntime.Diagnostics["sourceId"] != "managed-mirror:SCOPED-MODEL" {
		t.Fatalf("diagnostics = %#v / %#v, want detached discovery facts", detail.Diagnostics, detail.ManagedRuntime.Diagnostics)
	}
}

func publicScope(t *testing.T, ref runtimescopes.Reference) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse public scope: %v", err)
	}
	return scope
}
