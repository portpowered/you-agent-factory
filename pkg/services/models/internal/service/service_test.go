package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestRootListCatalogReturnsStableDetachedScopedProjection(t *testing.T) {
	t.Parallel()

	root := newScopedCatalogRoot(t)
	request := models.OpenRuntimeScopeRequest{Config: models.RuntimeScopeConfig{
		CacheDirectory: "original-cache",
		Runtime: models.RuntimeConfig{
			FactoryDirectory: "selected-factory",
			Workers: []models.RuntimeWorker{
				scopedCatalogWorker("zeta", "zeta-model", "summarize"),
				scopedCatalogWorker("alpha-generate", " alpha-model ", "generate"),
				scopedCatalogWorker("alpha-embed", "ALPHA-MODEL", "embed"),
			},
			Resources: []models.RuntimeResource{
				scopedCatalogResource("zeta-cache", "zeta-model", ""),
				scopedCatalogResource("alpha-cache", "ALPHA-MODEL", "MODELSCOPE"),
			},
		},
	}}
	opened, err := root.OpenRuntimeScope(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	// The accepted scope owns the effective snapshot taken at open time.
	request.Config.Runtime.Workers[0].Model = "mutated-model"
	request.Config.Runtime.Resources[1].Provider = "mutated-provider"

	first, err := root.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	assertScopedCatalog(t, first)

	second, err := root.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil {
		t.Fatalf("ListCatalog repeated: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated ListCatalog differs:\nfirst:  %#v\nsecond: %#v", first, second)
	}

	mutateScopedCatalogResult(first)
	afterMutation, err := root.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil {
		t.Fatalf("ListCatalog after caller mutation: %v", err)
	}
	assertScopedCatalog(t, afterMutation)
}

func TestRootGetCatalogModelDelegatesScopedLookup(t *testing.T) {
	t.Parallel()

	root := newScopedCatalogRoot(t)
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{
				scopedCatalogWorker("summarizer", "scoped-model", "summarize"),
				scopedCatalogWorker("generator", "SCOPED-MODEL", "generate"),
			},
			Resources: []models.RuntimeResource{
				scopedCatalogResource("zeta-cache", "scoped-model", "MODELSCOPE"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	got, err := root.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: " scoped-model ", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	if localmodels.CanonicalModelName(got.Model.Name) != "SCOPED-MODEL" ||
		len(got.Model.Operations) != 2 ||
		got.Model.Operations[0].Name != "generate" ||
		len(got.Model.Sources) != 1 ||
		got.Model.Sources[0].Provider != localmodels.ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("GetCatalogModel = %#v, want delegated stable scoped detail", got)
	}

	_, err = root.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "scoped-model", Operation: "embed",
	})
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("unsupported GetCatalogModel error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestRootCatalogMatchesDirectPrivateCatalogBehavior(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "parent-parity-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	currentReadiness := models.Runtime{
		ReadinessState: models.ReadinessStateMissing,
		LifecycleState: models.LifecycleStateNotInstalled,
		Diagnostics:    map[string]string{"observation": "initial"},
	}
	privateCatalog, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return currentReadiness.Clone(), nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	root := &Root{runtimeScopes: scopes, catalog: privateCatalog, resources: mustResourceLimiter(t), pullModel: inertScopedLocalExecution{}.PullModelForScope, closeScopedExecution: inertScopedLocalExecution{}.CloseScope, closeExecution: inertScopedLocalExecution{}.Close}
	opened := openScopedCatalogModel(t, root, "parity-model", "generate")

	directList, err := privateCatalog.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: opened.Scope},
	)
	if err != nil {
		t.Fatalf("direct ListCatalog: %v", err)
	}
	rootList, err := root.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil || !reflect.DeepEqual(rootList, directList) {
		t.Fatalf("root ListCatalog = (%#v, %v), want direct result %#v", rootList, err, directList)
	}

	getRequest := models.GetModelRequest{
		Scope: opened.Scope, Name: " parity-model ", Operation: "generate",
	}
	directModel, err := privateCatalog.GetCatalogModel(context.Background(), getRequest)
	if err != nil {
		t.Fatalf("direct GetCatalogModel: %v", err)
	}
	rootModel, err := root.GetCatalogModel(context.Background(), getRequest)
	if err != nil || !reflect.DeepEqual(rootModel, directModel) {
		t.Fatalf("root GetCatalogModel = (%#v, %v), want direct result %#v", rootModel, err, directModel)
	}

	assertRootReadinessMatchesPrivate(t, root, privateCatalog, opened.Scope, currentReadiness)
	currentReadiness.ReadinessState = models.ReadinessStateReady
	currentReadiness.LifecycleState = models.LifecycleStateInstalled
	currentReadiness.Diagnostics["observation"] = "transitioned"
	assertRootReadinessMatchesPrivate(t, root, privateCatalog, opened.Scope, currentReadiness)
}

func TestRootDelegatesAssetPreparation(t *testing.T) {
	t.Parallel()

	want := models.PrepareModelAssetsResult{
		Outcome: models.AssetPreparationAlreadyAvailable,
		Asset: models.AssetSnapshot{
			ModelName: "scoped-model", Readiness: models.AssetReadinessAvailable,
		},
	}
	privateAssets := &preparationAssetService{result: want}
	root := &Root{assets: privateAssets}
	request := models.PrepareModelAssetsRequest{Name: "scoped-model"}
	got, err := root.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(privateAssets.request, request) {
		t.Fatalf("PrepareModelAssets = %#v request %#v, want %#v / %#v", got, privateAssets.request, want, request)
	}
}

type preparationAssetService struct {
	request models.PrepareModelAssetsRequest
	result  models.PrepareModelAssetsResult
}

func (*preparationAssetService) PreflightModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, nil
}

func (service *preparationAssetService) PrepareModelAssets(
	_ context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	service.request = request
	return service.result, nil
}

func (*preparationAssetService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, nil
}

func (*preparationAssetService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (*preparationAssetService) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, nil
}

func (*preparationAssetService) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{}, nil
}

func TestRootCatalogMatchesDirectPrivateCatalogFailures(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "parent-error-parity-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	readinessErr := error(nil)
	privateCatalog, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{}, readinessErr
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	root := &Root{runtimeScopes: scopes, catalog: privateCatalog, resources: mustResourceLimiter(t), pullModel: inertScopedLocalExecution{}.PullModelForScope, closeScopedExecution: inertScopedLocalExecution{}.CloseScope, closeExecution: inertScopedLocalExecution{}.Close}
	opened := openScopedCatalogModel(t, root, "parity-model", "generate")

	assertCatalogGetFailureParity(t, root, privateCatalog, models.GetModelRequest{
		Scope: opened.Scope, Name: "missing-model",
	}, models.ErrNotFound)
	assertCatalogGetFailureParity(t, root, privateCatalog, models.GetModelRequest{
		Scope: opened.Scope, Name: "parity-model", Operation: "embed",
	}, models.ErrUnsupportedOperation)
	assertCatalogListFailureParity(
		t, root, privateCatalog, context.Background(), models.RuntimeScopeRef{}, models.ErrRuntimeScopeInvalid,
	)

	unavailableRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return nil },
	})
	if err != nil {
		t.Fatalf("open unavailable scope: %v", err)
	}
	unavailableScope, err := (models.RuntimeScopeRef{}).Parse(string(unavailableRef))
	if err != nil {
		t.Fatalf("parse unavailable scope: %v", err)
	}
	assertCatalogListFailureParity(
		t, root, privateCatalog, context.Background(), unavailableScope, models.ErrUnavailable,
	)

	if _, err := root.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: opened.Scope},
	); err != nil {
		t.Fatalf("close parity scope: %v", err)
	}
	assertCatalogListFailureParity(
		t, root, privateCatalog, context.Background(), opened.Scope, models.ErrRuntimeScopeClosed,
	)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	assertCatalogListFailureParity(t, root, privateCatalog, canceled, unavailableScope, context.Canceled)
}

func TestRootGetModelReadinessPreservesRequestResultAndFailure(t *testing.T) {
	t.Parallel()
	scope, err := (models.RuntimeScopeRef{}).Parse("root-readiness-boundary")
	if err != nil {
		t.Fatal(err)
	}
	request := models.GetModelReadinessRequest{Scope: scope, Name: "selected-model", Operation: "generate"}
	want := models.GetModelReadinessResult{Readiness: models.Runtime{
		Identity: "selected-model", ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateInstalled,
	}}
	failure := errors.New("readiness query failed")
	for _, queryErr := range []error{nil, failure} {
		ctx, cancel := context.WithCancel(context.Background())
		boundary := rootReadinessBoundary{query: func(gotCtx context.Context, got models.GetModelReadinessRequest) (models.GetModelReadinessResult, error) {
			if gotCtx != ctx || got != request {
				t.Fatalf("readiness request/context = (%#v, %v), want selected request/context", got, gotCtx)
			}
			return want, queryErr
		}}
		root := &Root{catalog: boundary}
		got, gotErr := root.GetModelReadiness(ctx, request)
		cancel()
		if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, queryErr) {
			t.Fatalf("GetModelReadiness = (%#v, %v), want (%#v, %v)", got, gotErr, want, queryErr)
		}
	}
}

type rootReadinessBoundary struct {
	modelcatalog.Service
	query func(context.Context, models.GetModelReadinessRequest) (models.GetModelReadinessResult, error)
}

func (b rootReadinessBoundary) GetModelReadiness(ctx context.Context, request models.GetModelReadinessRequest) (models.GetModelReadinessResult, error) {
	return b.query(ctx, request)
}

func TestRootClosesScopeAndPreservesClosedClassification(t *testing.T) {
	t.Parallel()

	root := newScopedCatalogRoot(t)
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{
				scopedCatalogWorker("worker", "closed-model", "generate"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	closed, err := root.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: opened.Scope},
	)
	if err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if !closed.Closed || closed.Scope != opened.Scope {
		t.Fatalf("CloseRuntimeScope = %#v, want closed issued scope", closed)
	}
	if _, err := root.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: opened.Scope},
	); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("repeated CloseRuntimeScope error = %v, want ErrRuntimeScopeClosed", err)
	}
	if _, err := root.GetModelReadiness(
		context.Background(),
		models.GetModelReadinessRequest{Scope: opened.Scope, Name: "closed-model"},
	); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("GetModelReadiness closed scope error = %v, want ErrRuntimeScopeClosed", err)
	}
}

func openScopedCatalogModel(t *testing.T, root *Root, name, operation string) models.OpenRuntimeScopeResult {
	t.Helper()
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{scopedCatalogWorker("worker", name, operation)},
		}},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	return opened
}

func assertRootReadinessMatchesPrivate(
	t *testing.T,
	root *Root,
	privateCatalog interface {
		GetModelReadiness(context.Context, models.GetModelReadinessRequest) (models.GetModelReadinessResult, error)
	},
	scope models.RuntimeScopeRef,
	want models.Runtime,
) {
	t.Helper()
	request := models.GetModelReadinessRequest{Scope: scope, Name: "parity-model", Operation: "generate"}
	direct, err := privateCatalog.GetModelReadiness(context.Background(), request)
	if err != nil {
		t.Fatalf("direct GetModelReadiness: %v", err)
	}
	got, err := root.GetModelReadiness(context.Background(), request)
	if err != nil || !reflect.DeepEqual(got, direct) {
		t.Fatalf("root GetModelReadiness = (%#v, %v), want direct result %#v", got, err, direct)
	}
	if got.Readiness.ReadinessState != want.ReadinessState ||
		got.Readiness.LifecycleState != want.LifecycleState ||
		got.Readiness.Diagnostics["observation"] != want.Diagnostics["observation"] {
		t.Fatalf("root readiness = %#v, want current facts %#v", got.Readiness, want)
	}
}

func assertCatalogGetFailureParity(
	t *testing.T,
	root *Root,
	privateCatalog interface {
		GetCatalogModel(context.Context, models.GetModelRequest) (models.GetModelResult, error)
	},
	request models.GetModelRequest,
	want error,
) {
	t.Helper()
	_, directErr := privateCatalog.GetCatalogModel(context.Background(), request)
	_, rootErr := root.GetCatalogModel(context.Background(), request)
	if !errors.Is(directErr, want) || !errors.Is(rootErr, want) {
		t.Fatalf("GetCatalogModel errors = (direct %v, root %v), want %v", directErr, rootErr, want)
	}
}

func assertCatalogListFailureParity(
	t *testing.T,
	root *Root,
	privateCatalog interface {
		ListCatalog(context.Context, models.ListModelsRequest) (models.ListModelsResult, error)
	},
	ctx context.Context,
	scope models.RuntimeScopeRef,
	want error,
) {
	t.Helper()
	request := models.ListModelsRequest{Scope: scope}
	_, directErr := privateCatalog.ListCatalog(ctx, request)
	_, rootErr := root.ListCatalog(ctx, request)
	if !errors.Is(directErr, want) || !errors.Is(rootErr, want) {
		t.Fatalf("ListCatalog errors = (direct %v, root %v), want %v", directErr, rootErr, want)
	}
}

func newScopedCatalogRoot(t *testing.T) *Root {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-root-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes, func(ctx context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
		if err := ctx.Err(); err != nil {
			return models.Runtime{}, err
		}
		return detail.ManagedRuntime.Clone(), nil
	})
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	return &Root{runtimeScopes: scopes, catalog: catalog, resources: mustResourceLimiter(t), pullModel: inertScopedLocalExecution{}.PullModelForScope, closeScopedExecution: inertScopedLocalExecution{}.CloseScope, closeExecution: inertScopedLocalExecution{}.Close}
}

func scopedCatalogWorker(name, model, operation string) models.RuntimeWorker {
	return models.RuntimeWorker{
		Name:          name,
		Type:          models.RuntimeWorkerTypeInference,
		Model:         model,
		ModelLocality: models.RuntimeModelLocalityLocal,
		Operations: []models.RuntimeOperation{{
			Name: operation,
			Inputs: []models.RuntimeOperationSlot{{
				Name: "input", ContentTypes: []string{
					models.RuntimeContentTypeText,
					models.RuntimeContentTypeAudio,
				},
			}},
		}},
		Resources: []models.RuntimeResource{{Name: modelResourceName(model)}},
	}
}

func modelResourceName(model string) string {
	if localmodels.CanonicalModelName(model) == "ALPHA-MODEL" {
		return "alpha-cache"
	}
	return "zeta-cache"
}

func scopedCatalogResource(name, model, provider string) models.RuntimeResource {
	return models.RuntimeResource{
		Name: name, Type: models.RuntimeResourceTypeModel, Capacity: 1,
		Model: model, Backend: "GGUF", LoadPolicy: "ON_DEMAND", Provider: provider,
	}
}

func assertScopedCatalog(t *testing.T, result models.ListModelsResult) {
	t.Helper()
	alpha, zeta := findScopedCatalogModels(result)
	if alpha == nil || zeta == nil {
		t.Fatalf("ListCatalog models = %#v, want Factory alpha/zeta identities", result.Models)
	}
	assertScopedCatalogIdentity(t, alpha, zeta)
	assertScopedCatalogOperations(t, alpha)
	assertScopedCatalogResource(t, alpha)
	assertScopedCatalogRuntime(t, alpha)
}

func findScopedCatalogModels(result models.ListModelsResult) (alpha, zeta *models.Summary) {
	for index := range result.Models {
		switch localmodels.CanonicalModelName(result.Models[index].Name) {
		case "ALPHA-MODEL":
			alpha = &result.Models[index]
		case "ZETA-MODEL":
			zeta = &result.Models[index]
		}
	}
	return alpha, zeta
}

func assertScopedCatalogIdentity(t *testing.T, alpha, zeta *models.Summary) {
	t.Helper()
	if localmodels.CanonicalModelName(alpha.Name) != "ALPHA-MODEL" {
		t.Fatalf("alpha model = %q, want canonical ALPHA-MODEL identity", alpha.Name)
	}
	if localmodels.CanonicalModelName(zeta.Name) != "ZETA-MODEL" {
		t.Fatalf("zeta model = %q, want ZETA-MODEL", zeta.Name)
	}
}

func assertScopedCatalogOperations(t *testing.T, alpha *models.Summary) {
	t.Helper()
	if len(alpha.Operations) != 2 ||
		alpha.Operations[0].Name != "embed" ||
		alpha.Operations[1].Name != "generate" {
		t.Fatalf("alpha operations = %#v, want embed then generate", alpha.Operations)
	}
	if got := alpha.Operations[0].Inputs[0].ContentTypes; !reflect.DeepEqual(
		got,
		[]string{models.RuntimeContentTypeAudio, models.RuntimeContentTypeText},
	) {
		t.Fatalf("alpha content types = %#v, want deterministic AUDIO/TEXT order", got)
	}
}

func assertScopedCatalogResource(t *testing.T, alpha *models.Summary) {
	t.Helper()
	if len(alpha.Resources) != 1 || alpha.Resources[0].Model == nil ||
		*alpha.Resources[0].Model != "ALPHA-MODEL" {
		t.Fatalf("alpha resources = %#v, want detached model resource", alpha.Resources)
	}
}

func assertScopedCatalogRuntime(t *testing.T, alpha *models.Summary) {
	t.Helper()
	if alpha.Status != models.StatusUnavailable ||
		alpha.ManagedRuntime.Diagnostics["sourceKind"] != localmodels.ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("alpha status/runtime = %#v, want unavailable managed-mirror projection", alpha)
	}
}

func mutateScopedCatalogResult(result models.ListModelsResult) {
	for index := range result.Models {
		if localmodels.CanonicalModelName(result.Models[index].Name) != "ALPHA-MODEL" {
			continue
		}
		result.Models[index].Name = "mutated"
		result.Models[index].Operations[0].Name = "mutated"
		result.Models[index].Operations[0].Inputs[0].ContentTypes[0] = "mutated"
		*result.Models[index].Resources[0].Model = "mutated"
		result.Models[index].ManagedRuntime.SupportedOperations[0].Name = "mutated"
		result.Models[index].ManagedRuntime.Diagnostics["sourceKind"] = "mutated"
		return
	}
}

func assertContractOnlyUnsupported(t *testing.T, operation string, err error) {
	t.Helper()
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("%s error = %v, want ErrUnsupportedOperation", operation, err)
	}
}

func TestRootContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := &Root{}
	_, err := root.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = root.ListCatalog(ctx, models.ListModelsRequest{})
	assertContractOnlyUnsupported(t, "ListCatalog", err)
	_, err = root.GetCatalogModel(ctx, models.GetModelRequest{})
	assertContractOnlyUnsupported(t, "GetCatalogModel", err)
	_, err = root.GetModelReadiness(ctx, models.GetModelReadinessRequest{})
	assertContractOnlyUnsupported(t, "GetModelReadiness", err)
	_, err = root.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{})
	assertContractOnlyUnsupported(t, "ResolveModelReference", err)
	_, err = root.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = root.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = root.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = root.EnsureModelHost(ctx, models.EnsureModelHostRequest{})
	assertContractOnlyUnsupported(t, "EnsureModelHost", err)
	_, err = root.InspectModelHost(ctx, models.InspectModelHostRequest{})
	assertContractOnlyUnsupported(t, "InspectModelHost", err)
	_, err = root.StopModelHost(ctx, models.StopModelHostRequest{})
	assertContractOnlyUnsupported(t, "StopModelHost", err)
	_, err = root.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "AcquireModelLease", err)
	_, err = root.GetModelLease(ctx, models.GetModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "GetModelLease", err)
	_, err = root.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "ReleaseModelLease", err)
	_, err = root.InvokeModelWithLease(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModelWithLease", err)
	_, err = root.CancelInvocation(ctx, models.CancelInvocationRequest{})
	assertContractOnlyUnsupported(t, "CancelInvocation", err)
}
