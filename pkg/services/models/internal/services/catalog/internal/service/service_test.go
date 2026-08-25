package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
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

func TestCatalogDiscoversBuiltInsWithoutFactoryDeclarations(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-built-in-discovery")
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{}
		},
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	scope := publicScope(t, privateRef)
	service := newCatalogService(t, scopes)

	listed, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	counts := make(map[string]int, len(listed.Models))
	for _, model := range listed.Models {
		counts[model.Name]++
	}
	for _, name := range []string{
		models.BuiltInModelNameASR,
		models.BuiltInModelNameEmbed,
		models.BuiltInModelNameLLM,
		models.BuiltInModelNameTTS,
	} {
		if counts[name] != 1 {
			t.Fatalf("listed %q count = %d, want one; models = %#v", name, counts[name], listed.Models)
		}
	}

	inspected, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: " ASR ", Operation: models.OperationASR,
	})
	if err != nil {
		t.Fatalf("GetCatalogModel(asr): %v", err)
	}
	if inspected.Model.Name != models.BuiltInModelNameASR ||
		len(inspected.Model.Operations) != 1 ||
		inspected.Model.Operations[0].Name != models.OperationASR {
		t.Fatalf("asr detail = %#v, want effective ASR definition", inspected.Model)
	}
	if inspected.Model.ManagedRuntime.ReadinessState != models.ReadinessStateMissing ||
		inspected.Model.ManagedRuntime.LifecycleState != models.LifecycleStateNotInstalled {
		t.Fatalf("asr runtime = %#v, want missing/not-installed baseline", inspected.Model.ManagedRuntime)
	}
	if len(inspected.Model.Sources) != 1 || inspected.Model.Sources[0].Provider != string(models.ModelReferenceSourceHuggingFace) {
		t.Fatalf("asr sources = %#v, want built-in source metadata", inspected.Model.Sources)
	}

	if _, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: "unregistered-model",
	}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("unknown model error = %v, want ErrNotFound", err)
	}
}

func TestCatalogPreservesFactoryAndOperatorPrecedenceOverBuiltIns(t *testing.T) {
	t.Parallel()

	fixture := newCatalogPrecedenceFixture(t)
	assertCatalogPrecedenceList(t, fixture)
	assertFactoryCatalogPrecedence(t, fixture)
	assertOperatorCatalogPrecedence(t, fixture)
	assertCustomCatalogPrecedence(t, fixture)
}

type catalogPrecedenceFixture struct {
	service        catalog.Service
	scope          models.RuntimeScopeRef
	operatorSource string
}

func newCatalogPrecedenceFixture(t *testing.T) catalogPrecedenceFixture {
	t.Helper()
	operatorSource := "hf://operator/tts/model@0123456789012345678901234567890123456789"
	operatorBackend := "localai-test"
	operatorLoadPolicy := models.LoadPolicyKeepWarm
	customSource := "hf://operator/custom/model@0123456789012345678901234567890123456789"
	customBackend := "localai-custom"
	customLoadPolicy := models.LoadPolicyOnDemand
	scopes := newRuntimeScopes(t, "catalog-precedence")
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{
					catalogWorker("factory-asr", models.BuiltInModelNameASR, "factory-operation"),
				},
			}
		},
		OperatorModels: map[string]models.ModelOverlay{
			models.BuiltInModelNameASR: {
				Operations: []string{models.OperationTTS},
			},
			" TTS ": {
				Source: &operatorSource, Backend: &operatorBackend,
				LoadPolicy: &operatorLoadPolicy, Operations: []string{models.OperationOMNI},
			},
			"custom": {
				Source: &customSource, Backend: &customBackend,
				LoadPolicy: &customLoadPolicy, Operations: []string{models.OperationASR},
			},
		},
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	scope := publicScope(t, privateRef)
	return catalogPrecedenceFixture{
		service:        newCatalogService(t, scopes),
		scope:          scope,
		operatorSource: operatorSource,
	}
}

func assertCatalogPrecedenceList(t *testing.T, fixture catalogPrecedenceFixture) {
	t.Helper()
	listed, err := fixture.service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: fixture.scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	countByName := make(map[string]int, len(listed.Models))
	for _, model := range listed.Models {
		countByName[model.Name]++
	}
	if countByName[models.BuiltInModelNameASR] != 1 {
		t.Fatalf("asr count = %d, want one effective Factory entry", countByName[models.BuiltInModelNameASR])
	}
}

func assertFactoryCatalogPrecedence(t *testing.T, fixture catalogPrecedenceFixture) {
	t.Helper()
	factory := getCatalogPrecedenceModel(t, fixture, models.GetModelRequest{
		Scope: fixture.scope, Name: models.BuiltInModelNameASR, Operation: "factory-operation",
	})
	if len(factory.Model.Operations) != 1 || factory.Model.Operations[0].Name != "factory-operation" {
		t.Fatalf("factory asr operations = %#v, want Factory operation", factory.Model.Operations)
	}
	if _, err := fixture.service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: fixture.scope, Name: models.BuiltInModelNameASR, Operation: models.OperationTTS,
	}); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("Factory precedence operation error = %v, want ErrUnsupportedOperation", err)
	}
}

func assertOperatorCatalogPrecedence(t *testing.T, fixture catalogPrecedenceFixture) {
	t.Helper()
	operator := getCatalogPrecedenceModel(t, fixture, models.GetModelRequest{
		Scope: fixture.scope, Name: models.BuiltInModelNameTTS, Operation: models.OperationOMNI,
	})
	if len(operator.Model.Operations) != 1 || operator.Model.Operations[0].Name != models.OperationOMNI {
		t.Fatalf("operator tts operations = %#v, want overlaid OMNI", operator.Model.Operations)
	}
	if len(operator.Model.Sources) != 1 || operator.Model.Sources[0].Reference != fixture.operatorSource {
		t.Fatalf("operator tts sources = %#v, want overlay source", operator.Model.Sources)
	}
}

func assertCustomCatalogPrecedence(t *testing.T, fixture catalogPrecedenceFixture) {
	t.Helper()
	custom := getCatalogPrecedenceModel(t, fixture, models.GetModelRequest{
		Scope: fixture.scope, Name: "CUSTOM", Operation: models.OperationASR,
	})
	if custom.Model.Name != "custom" || len(custom.Model.Operations) != 1 || custom.Model.Operations[0].Name != models.OperationASR {
		t.Fatalf("custom detail = %#v, want operator definition", custom.Model)
	}
}

func getCatalogPrecedenceModel(
	t *testing.T,
	fixture catalogPrecedenceFixture,
	request models.GetModelRequest,
) models.GetModelResult {
	t.Helper()
	result, err := fixture.service.GetCatalogModel(context.Background(), request)
	if err != nil {
		t.Fatalf("GetCatalogModel(%s): %v", request.Name, err)
	}
	return result
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

func TestCatalogReadsOverlayResolvedRuntimeStateAndCacheFacts(t *testing.T) {
	t.Parallel()

	revision := "rev-installed"
	cachePath := "/tmp/models/cache-model/rev-installed"
	cacheBytes := int64(1234)
	scopes := newRuntimeScopes(t, "catalog-cache-facts")
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{
				ReadinessState: models.ReadinessStateFailed,
				LifecycleState: models.LifecycleStateInstalling,
				Revision:       &revision,
				CachePath:      &cachePath,
				CacheBytes:     &cacheBytes,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, openCatalogScope(t, scopes, "cache-model", "generate"))
	listCatalogCacheFacts(t, service, scope, revision, cachePath, cacheBytes)
	detail := inspectCatalogCacheFacts(t, service, scope, revision, cachePath, cacheBytes)
	if detail.Model.ManagedRuntime.ReadinessState != models.ReadinessStateFailed ||
		detail.Model.ManagedRuntime.LifecycleState != models.LifecycleStateInstalling {
		t.Fatalf("inspect runtime states = (%s, %s), want FAILED/INSTALLING", detail.Model.ManagedRuntime.ReadinessState, detail.Model.ManagedRuntime.LifecycleState)
	}
	if detail.Model.Diagnostics["readinessState"] != string(detail.Model.ManagedRuntime.ReadinessState) ||
		detail.Model.Diagnostics["lifecycleState"] != string(detail.Model.ManagedRuntime.LifecycleState) {
		t.Fatalf("detail diagnostics state = %#v, want the resolved managed runtime state", detail.Model.Diagnostics)
	}
}

func listCatalogCacheFacts(
	t *testing.T,
	service catalog.Service,
	scope models.RuntimeScopeRef,
	revision string,
	cachePath string,
	cacheBytes int64,
) {
	result, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	var model *models.Summary
	for index := range result.Models {
		if result.Models[index].Name == "cache-model" {
			model = &result.Models[index]
			break
		}
	}
	if model == nil {
		t.Fatalf("models = %#v, want cache-model", result.Models)
	}
	runtime := model.ManagedRuntime
	if runtime.ReadinessState != models.ReadinessStateFailed ||
		runtime.LifecycleState != models.LifecycleStateInstalling {
		t.Fatalf("catalog states = (%s, %s), want FAILED/INSTALLING", runtime.ReadinessState, runtime.LifecycleState)
	}
	if runtime.Revision == nil || *runtime.Revision != revision ||
		runtime.CachePath == nil || *runtime.CachePath != cachePath ||
		runtime.CacheBytes == nil || *runtime.CacheBytes != cacheBytes {
		t.Fatalf("catalog cache facts = revision=%v path=%v bytes=%v, want rev-installed/path/1234", runtime.Revision, runtime.CachePath, runtime.CacheBytes)
	}
	if runtime.Diagnostics["readinessState"] != string(models.ReadinessStateFailed) ||
		runtime.Diagnostics["lifecycleState"] != string(models.LifecycleStateInstalling) {
		t.Fatalf("catalog diagnostics state = %#v, want FAILED/INSTALLING", runtime.Diagnostics)
	}
}

func inspectCatalogCacheFacts(
	t *testing.T,
	service catalog.Service,
	scope models.RuntimeScopeRef,
	revision string,
	cachePath string,
	cacheBytes int64,
) models.GetModelResult {
	detail, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: "cache-model",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	inspectRuntime := detail.Model.ManagedRuntime
	if inspectRuntime.ReadinessState != models.ReadinessStateFailed ||
		inspectRuntime.LifecycleState != models.LifecycleStateInstalling ||
		inspectRuntime.CachePath == nil || *inspectRuntime.CachePath != cachePath ||
		inspectRuntime.Revision == nil || *inspectRuntime.Revision != revision ||
		inspectRuntime.CacheBytes == nil || *inspectRuntime.CacheBytes != cacheBytes {
		t.Fatalf("inspect runtime = %#v, want resolved states and cache facts", inspectRuntime)
	}
	return detail
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

func TestCatalogOperationsPreserveEveryScopeClassification(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-scope-classifications")
	foreignScopes := newRuntimeScopes(t, "catalog-scope-foreign")
	service := newCatalogService(t, scopes)
	closedRef := openCatalogScope(t, scopes, "closed-model", "generate")
	if err := scopes.Close(closedRef); err != nil {
		t.Fatalf("close scope: %v", err)
	}
	foreignRef := openCatalogScope(t, foreignScopes, "foreign-model", "generate")
	stale, err := (models.RuntimeScopeRef{}).Parse("stale-scope")
	if err != nil {
		t.Fatalf("parse stale scope: %v", err)
	}

	tests := []struct {
		name  string
		scope models.RuntimeScopeRef
		want  error
	}{
		{name: "missing", want: models.ErrRuntimeScopeInvalid},
		{name: "closed", scope: publicScope(t, closedRef), want: models.ErrRuntimeScopeClosed},
		{name: "stale", scope: stale, want: models.ErrRuntimeScopeStale},
		{name: "foreign", scope: publicScope(t, foreignRef), want: models.ErrRuntimeScopeForeign},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := service.ListCatalog(
				context.Background(),
				models.ListModelsRequest{Scope: test.scope},
			); !errors.Is(err, test.want) {
				t.Errorf("ListCatalog error = %v, want %v", err, test.want)
			}
			if _, err := service.GetCatalogModel(
				context.Background(),
				models.GetModelRequest{Scope: test.scope},
			); !errors.Is(err, test.want) {
				t.Errorf("GetCatalogModel error = %v, want %v", err, test.want)
			}
			if _, err := service.GetModelReadiness(
				context.Background(),
				models.GetModelReadinessRequest{Scope: test.scope},
			); !errors.Is(err, test.want) {
				t.Errorf("GetModelReadiness error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCatalogOperationsKeepOpenScopesIsolatedAfterOneCloses(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-scope-isolation")
	service := newCatalogService(t, scopes)
	firstRef := openCatalogScope(t, scopes, "shared-model", "generate")
	secondRef := openCatalogScope(t, scopes, "shared-model", "embed")
	firstScope := publicScope(t, firstRef)
	secondScope := publicScope(t, secondRef)

	first, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: firstScope, Name: "shared-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("first readiness: %v", err)
	}
	second, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: secondScope, Name: "shared-model", Operation: "embed",
	})
	if err != nil {
		t.Fatalf("second readiness: %v", err)
	}
	if first.Readiness.SupportedOperations[0].Name != "generate" ||
		second.Readiness.SupportedOperations[0].Name != "embed" {
		t.Fatalf("readiness crossed scopes: first=%#v second=%#v", first, second)
	}

	if err := scopes.Close(firstRef); err != nil {
		t.Fatalf("close first scope: %v", err)
	}
	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: firstScope},
	); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("closed first scope error = %v, want ErrRuntimeScopeClosed", err)
	}
	remaining, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: secondScope, Name: "shared-model", Operation: "embed",
	})
	if err != nil {
		t.Fatalf("remaining scope get: %v", err)
	}
	if remaining.Model.Operations[0].Name != "embed" {
		t.Fatalf("remaining scope detail = %#v, want isolated embed operation", remaining)
	}
}

func TestCatalogOperationsReturnUnavailableForLiveScopeWithoutCatalog(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-unavailable-all-operations")
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return nil },
	})
	if err != nil {
		t.Fatalf("open unavailable scope: %v", err)
	}
	scope := publicScope(t, privateRef)
	service := newCatalogService(t, scopes)

	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: scope},
	); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Errorf("ListCatalog error = %v, want sanitized ErrUnavailable", err)
	}
	if _, err := service.GetCatalogModel(
		context.Background(),
		models.GetModelRequest{Scope: scope},
	); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Errorf("GetCatalogModel error = %v, want sanitized ErrUnavailable", err)
	}
	if _, err := service.GetModelReadiness(
		context.Background(),
		models.GetModelReadinessRequest{Scope: scope},
	); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Errorf("GetModelReadiness error = %v, want sanitized ErrUnavailable", err)
	}
}

func TestCatalogOperationsHonorCancellationBeforeAndDuringReadinessQuery(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-cancellation")
	queryStarted := make(chan struct{})
	service, err := catalogwire.NewService(
		scopes,
		func(ctx context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, _ models.Detail) (models.Runtime, error) {
			close(queryStarted)
			<-ctx.Done()
			return models.Runtime{}, ctx.Err()
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	privateRef := openCatalogScope(t, scopes, "cancel-model", "generate")
	scope := publicScope(t, privateRef)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ListCatalog(
		cancelled,
		models.ListModelsRequest{Scope: scope},
	); !errors.Is(err, context.Canceled) {
		t.Errorf("ListCatalog pre-canceled error = %v, want context.Canceled", err)
	}
	if _, err := service.GetCatalogModel(
		cancelled,
		models.GetModelRequest{Scope: scope},
	); !errors.Is(err, context.Canceled) {
		t.Errorf("GetCatalogModel pre-canceled error = %v, want context.Canceled", err)
	}
	if _, err := service.GetModelReadiness(
		cancelled,
		models.GetModelReadinessRequest{Scope: scope},
	); !errors.Is(err, context.Canceled) {
		t.Errorf("GetModelReadiness pre-canceled error = %v, want context.Canceled", err)
	}

	ctx, cancelDuring := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, queryErr := service.GetModelReadiness(ctx, models.GetModelReadinessRequest{
			Scope: scope, Name: "cancel-model", Operation: "generate",
		})
		result <- queryErr
	}()
	select {
	case <-queryStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("readiness query did not start")
	}
	cancelDuring()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled readiness error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled readiness query did not stop")
	}
	if _, err := scopes.Resolve(privateRef); err != nil {
		t.Fatalf("canceled query changed scope state: %v", err)
	}
}

func TestReadinessDependencyFailuresAreSanitizedAsUnavailable(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-readiness-failure")
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{}, errors.New(`inspect C:\private\model-cache: access denied`)
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, openCatalogScope(t, scopes, "failed-model", "generate"))
	if _, err := service.GetModelReadiness(
		context.Background(),
		models.GetModelReadinessRequest{Scope: scope, Name: "failed-model"},
	); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Fatalf("GetModelReadiness error = %v, want sanitized ErrUnavailable", err)
	}
}

func TestCatalogReadinessFailuresKeepOperationTaxonomy(t *testing.T) {
	t.Parallel()

	// Exercise the list and detail adapters with a dependency failure so both
	// projections retain the public unavailable taxonomy.
	scopes := newRuntimeScopes(t, "catalog-readiness-failure-projections")
	privateRef := openCatalogScope(t, scopes, "readiness-model", "generate")
	scope := publicScope(t, privateRef)
	readinessFailure := func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
		return models.Runtime{}, errors.New("cache probe failed")
	}
	service, err := catalogwire.NewService(scopes, readinessFailure)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	if _, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope}); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog failure = %v, want ErrUnavailable", err)
	}
	if _, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{Scope: scope, Name: "readiness-model"}); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("GetCatalogModel failure = %v, want ErrUnavailable", err)
	}

	// A dependency that reports cancellation/deadline is allowed to preserve
	// that cause instead of being sanitized as a generic unavailable error.
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		cause := cause
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			localScopes := newRuntimeScopes(t, "catalog-readiness-context")
			localScope := publicScope(t, openCatalogScope(t, localScopes, "readiness-model", "generate"))
			localService, err := catalogwire.NewService(localScopes, func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
				return models.Runtime{}, cause
			})
			if err != nil {
				t.Fatalf("construct Catalog: %v", err)
			}
			if _, err := localService.ListCatalog(context.Background(), models.ListModelsRequest{Scope: localScope}); !errors.Is(err, cause) {
				t.Fatalf("ListCatalog failure = %v, want %v", err, cause)
			}
			if _, err := localService.GetCatalogModel(context.Background(), models.GetModelRequest{Scope: localScope, Name: "readiness-model"}); !errors.Is(err, cause) {
				t.Fatalf("GetCatalogModel failure = %v, want %v", err, cause)
			}
		})
	}

	// Keep a valid scope available to document the stable effective-definition
	// baseline when no readiness dependency is installed.
	nilReadinessScopes := newRuntimeScopes(t, "catalog-no-readiness")
	nilReadinessScope := publicScope(t, openCatalogScope(t, nilReadinessScopes, "readiness-model", "generate"))
	if _, err := newCatalogService(t, nilReadinessScopes).GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: nilReadinessScope, Name: "readiness-model",
	}); err != nil {
		t.Fatalf("GetModelReadiness without dependency = %v, want stable baseline", err)
	}
}

func TestCatalogRejectsInvalidOperatorOverlays(t *testing.T) {
	t.Parallel()

	blank := ""
	validSource := "hf://operator/model@0123456789012345678901234567890123456789"
	validBackend := "localai"
	validLoadPolicy := models.LoadPolicyOnDemand
	validOverlay := models.ModelOverlay{
		Source: &validSource, Backend: &validBackend, LoadPolicy: &validLoadPolicy,
		Operations: []string{models.OperationTTS},
	}
	tests := []struct {
		name     string
		operator map[string]models.ModelOverlay
		field    string
	}{
		{name: "invalid name", operator: map[string]models.ModelOverlay{"bad/name": validOverlay}, field: "name"},
		{name: "duplicate normalized name", operator: map[string]models.ModelOverlay{"Model": validOverlay, " model ": validOverlay}, field: "name"},
		{name: "empty source", operator: map[string]models.ModelOverlay{"model": {Source: &blank, Backend: &validBackend, LoadPolicy: &validLoadPolicy, Operations: []string{models.OperationTTS}}}, field: "source"},
		{name: "empty backend", operator: map[string]models.ModelOverlay{"model": {Source: &validSource, Backend: &blank, LoadPolicy: &validLoadPolicy, Operations: []string{models.OperationTTS}}}, field: "backend"},
		{name: "invalid load policy", operator: map[string]models.ModelOverlay{"model": {Source: &validSource, Backend: &validBackend, LoadPolicy: func() *models.LoadPolicy { value := models.LoadPolicy("NEVER"); return &value }(), Operations: []string{models.OperationTTS}}}, field: "loadPolicy"},
		{name: "empty operations", operator: map[string]models.ModelOverlay{"model": {Source: &validSource, Backend: &validBackend, LoadPolicy: &validLoadPolicy, Operations: []string{}}}, field: "operations"},
		{name: "unsupported operation", operator: map[string]models.ModelOverlay{"model": {Source: &validSource, Backend: &validBackend, LoadPolicy: &validLoadPolicy, Operations: []string{"UNKNOWN"}}}, field: "operations"},
		{name: "custom missing source", operator: map[string]models.ModelOverlay{"custom": {Backend: &validBackend, LoadPolicy: &validLoadPolicy, Operations: []string{models.OperationTTS}}}, field: "source"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scopes := newRuntimeScopes(t, "catalog-invalid-overlay")
			privateRef, err := scopes.Open(models.RuntimeBinding{
				RuntimeConfig:  func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
				OperatorModels: test.operator,
			})
			if err != nil {
				t.Fatalf("open scope: %v", err)
			}
			service := newCatalogService(t, scopes)
			_, err = service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: publicScope(t, privateRef)})
			if err == nil {
				t.Fatal("ListCatalog() = nil, want model configuration failure")
			}
			var failure models.ModelConfigurationFailure
			if !errors.As(err, &failure) || failure.Field != test.field {
				t.Fatalf("ListCatalog() error = %v (%T), want configuration field %q", err, err, test.field)
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

func TestCatalogReadinessFailuresAreSanitizedAcrossListAndDetail(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-readiness-failure-projections")
	dependencyFailure := errors.New(`inspect C:\private\model-cache: access denied`)
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{}, dependencyFailure
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, openCatalogScope(t, scopes, "failed-model", "generate"))

	if _, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope}); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Fatalf("ListCatalog error = %v, want sanitized ErrUnavailable", err)
	}
	if _, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: "failed-model",
	}); !errors.Is(err, models.ErrUnavailable) || err.Error() != models.ErrUnavailable.Error() {
		t.Fatalf("GetCatalogModel error = %v, want sanitized ErrUnavailable", err)
	}
}

func TestBuiltInReadinessUsesStableDiscoveryBaseline(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-built-in-readiness-baseline")
	queryCalls := 0
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			queryCalls++
			return models.Runtime{ReadinessState: models.ReadinessStateReady}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, openCatalogScope(t, scopes, "configured-model", "generate"))
	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: scope, Name: " ASR ", Operation: models.OperationASR,
	})
	if err != nil {
		t.Fatalf("GetModelReadiness built-in: %v", err)
	}
	if readiness.ModelName != models.BuiltInModelNameASR ||
		readiness.Readiness.Identity != models.BuiltInModelNameASR ||
		readiness.Readiness.ReadinessState != models.ReadinessStateMissing ||
		readiness.Readiness.LifecycleState != models.LifecycleStateNotInstalled {
		t.Fatalf("built-in readiness = %#v, want stable missing/not-installed baseline", readiness)
	}
	if queryCalls != 0 {
		t.Fatalf("built-in readiness queried current state %d times, want zero", queryCalls)
	}
}

func newRuntimeScopes(t *testing.T, issuer string) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return issuer })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	return scopes
}

func newCatalogService(t *testing.T, scopes runtimescopes.Service) catalog.Service {
	t.Helper()
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	return service
}

func openCatalogScope(
	t *testing.T,
	scopes runtimescopes.Service,
	model string,
	operation string,
) runtimescopes.Reference {
	t.Helper()
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{catalogWorker("worker", model, operation)},
			}
		},
	})
	if err != nil {
		t.Fatalf("open catalog scope: %v", err)
	}
	return privateRef
}

func openCatalogScopeWithOverlays(
	t *testing.T,
	scopes runtimescopes.Service,
	overlays map[string]models.ModelOverlay,
) models.RuntimeScopeRef {
	t.Helper()
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{}
		},
		OperatorModels: overlays,
	})
	if err != nil {
		t.Fatalf("open catalog overlay scope: %v", err)
	}
	return publicScope(t, privateRef)
}

func stringPointer(value string) *string {
	return &value
}

func loadPolicyPointer(value models.LoadPolicy) *models.LoadPolicy {
	return &value
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
