package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

func TestNewRootRejectsMissingHostPlatform(t *testing.T) {
	t.Parallel()

	for _, platform := range []localmodels.HostPlatform{
		{Architecture: "amd64"},
		{OperatingSystem: "linux"},
		{OperatingSystem: " ", Architecture: "amd64"},
		{OperatingSystem: "linux", Architecture: " "},
	} {
		opener, err := NewRoot(
			platform,
			nil, modelassets.Endpoints{},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil,
		)
		if opener != nil || !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), "model asset host platform") {
			t.Fatalf("NewRoot(%#v) = (%#v, %v), want missing host-platform dependency", platform, opener, err)
		}
	}
}

func TestNewServiceRetainsExplicitDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	puller := constructionAssetPuller{}
	logger := zap.NewNop()
	metrics := &constructionPullMetrics{}
	host := constructionModelHost{}

	svc, err := NewService(
		func() *modelRuntimeConfig { return runtimeCfg },
		host,
		puller,
		logger,
		func() time.Time { return time.Unix(123, 0) },
		metrics,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.runtimeConfig() != runtimeCfg || svc.modelHost() != host || svc.modelAssetPuller() != puller {
		t.Fatal("NewService did not retain required dependencies")
	}
	if svc.logger() != logger || svc.pullMetrics != metrics {
		t.Fatal("NewService did not retain optional dependencies")
	}
}

func TestNewServiceRejectsMissingClockAndLogger(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	for _, test := range []struct {
		name   string
		logger *zap.Logger
		clock  func() time.Time
		want   string
	}{
		{name: "logger", clock: time.Now, want: "logger is required"},
		{name: "clock", logger: zap.NewNop(), want: "clock is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, err := NewService(
				func() *modelRuntimeConfig { return runtimeCfg }, constructionModelHost{},
				constructionAssetPuller{}, test.logger, test.clock, nil,
			)
			if svc != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService = (%#v, %v), want %q", svc, err, test.want)
			}
		})
	}
}

func TestServiceNilReceiverPreservesUnavailableRuntimeErrors(t *testing.T) {
	t.Parallel()

	var svc *Service
	assertUnavailable := func(operation string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "runtime is not available") {
			t.Fatalf("%s error = %v, want runtime unavailable", operation, err)
		}
	}

	_, err := svc.ListModels(context.Background())
	assertUnavailable("ListModels", err)
	_, err = svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("GetModel", err)
	_, err = svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("PullModel", err)
	if svc.logger() != nil || svc.modelHost() != nil {
		t.Fatal("nil service accessors returned configured collaborators")
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	runtimeConfig := func() *modelRuntimeConfig { return runtimeCfg }
	host := constructionModelHost{}
	puller := constructionAssetPuller{}
	tests := []struct {
		name       string
		dependency string
		construct  func() (*Service, error)
	}{
		{
			name: "runtime lookup", dependency: "runtime configuration lookup",
			construct: func() (*Service, error) { return NewService(nil, host, puller, zap.NewNop(), time.Now, nil) },
		},
		{
			name: "model host", dependency: "model host",
			construct: func() (*Service, error) { return NewService(runtimeConfig, nil, puller, zap.NewNop(), time.Now, nil) },
		},
		{
			name: "typed nil model host", dependency: "model host",
			construct: func() (*Service, error) {
				var typedNilHost *constructionModelHost
				return NewService(runtimeConfig, typedNilHost, puller, zap.NewNop(), time.Now, nil)
			},
		},
		{
			name: "asset puller", dependency: "model asset puller",
			construct: func() (*Service, error) { return NewService(runtimeConfig, host, nil, zap.NewNop(), time.Now, nil) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, err := test.construct()
			if svc != nil {
				t.Fatalf("NewService service = %#v, want nil", svc)
			}
			if !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), test.dependency) {
				t.Fatalf("NewService error = %v, want invalid-dependencies error naming %q", err, test.dependency)
			}
		})
	}
}

type constructionPullMetrics struct{}

func (*constructionPullMetrics) RecordModelPullMetric(models.PullMetric) {}

type constructionAssetPuller struct{}

func (constructionAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (constructionAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (constructionAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}
func (constructionAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

func mustConstructionRuntimeConfig(t *testing.T) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig(t.TempDir(), &testFactoryConfig{
		Name: "construction-test",
	})
}

type constructionModelHost struct{}

func (constructionModelHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}

func (constructionModelHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}

func (constructionModelHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, nil
}

func (constructionModelHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}

func (constructionModelHost) ReleaseLease(context.Context, string) error { return nil }

func (constructionModelHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
}

func TestService_AcquireLease_ReturnsRuntimeNotReadyWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		clock:               time.Now,
	}

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"})
	if !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireLease nil host = %v, want ErrHostRuntimeNotReady", err)
	}
}

func TestService_ReleaseLease_ReturnsLeaseNotFoundWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		loggerValue:         zap.NewNop(),
		clock:               time.Now,
	}

	err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease nil host = %v, want ErrHostLeaseNotFound", err)
	}
}

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
		func(context.Context, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return currentReadiness.Clone(), nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	root := &Root{runtimeScopes: scopes, catalog: privateCatalog}
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

func TestRootCatalogMatchesDirectPrivateCatalogFailures(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "parent-error-parity-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	readinessErr := error(nil)
	privateCatalog, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return models.Runtime{}, readinessErr
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	root := &Root{runtimeScopes: scopes, catalog: privateCatalog}
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

func TestScopedCatalogPreservesCompatibilityBehavior(t *testing.T) {
	t.Parallel()

	runtimeConfig := models.RuntimeConfig{
		Workers: []models.RuntimeWorker{
			scopedCatalogWorker("compatibility-worker", "compatibility-model", "generate"),
		},
	}
	entry := localmodels.BuildCatalogWithRuntime(
		&runtimeConfig,
		nil,
		localmodels.DefaultManagedRuntimeSourceResolver(),
	)[localmodels.CanonicalModelName("compatibility-model")]
	currentReadiness := entry.Detail.ManagedRuntime.Clone()
	host := &compatibilityParityHost{snapshot: readinessSnapshot(currentReadiness)}
	compatibility, err := NewService(
		func() *models.RuntimeConfig { return &runtimeConfig },
		host,
		constructionAssetPuller{},
		zap.NewNop(),
		time.Now,
		nil,
	)
	if err != nil {
		t.Fatalf("construct compatibility service: %v", err)
	}

	scopes, err := runtimescopeswire.NewService(func() string { return "compatibility-parity-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateCatalog, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			return currentReadiness.Clone(), nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	root := &Root{runtimeScopes: scopes, catalog: privateCatalog}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: runtimeConfig},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	legacyList, err := compatibility.ListModels(context.Background())
	if err != nil {
		t.Fatalf("compatibility ListModels: %v", err)
	}
	scopedList, err := root.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: opened.Scope},
	)
	if err != nil || !reflect.DeepEqual(legacyList.Results, scopedList.Models) {
		t.Fatalf("catalog list parity = (legacy %#v, scoped %#v, %v)", legacyList.Results, scopedList.Models, err)
	}

	legacyDetail, err := compatibility.GetModel(context.Background(), "compatibility-model")
	if err != nil {
		t.Fatalf("compatibility GetModel: %v", err)
	}
	scopedDetail, err := root.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "compatibility-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	scopedCompatibilityFields := scopedDetail.Model.Clone()
	scopedCompatibilityFields.Sources = nil
	if !reflect.DeepEqual(legacyDetail, scopedCompatibilityFields) {
		t.Fatalf("catalog get parity = (legacy %#v, scoped %#v)", legacyDetail, scopedCompatibilityFields)
	}

	currentReadiness.ReadinessState = models.ReadinessStateReady
	currentReadiness.LifecycleState = models.LifecycleStateInstalled
	host.snapshot = readinessSnapshot(currentReadiness)
	legacyReadiness, err := compatibility.InspectRuntime(context.Background(), "compatibility-model")
	if err != nil {
		t.Fatalf("compatibility InspectRuntime: %v", err)
	}
	scopedReadiness, err := root.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "compatibility-model", Operation: "generate",
	})
	if err != nil || !reflect.DeepEqual(legacyReadiness, scopedReadiness.Readiness) {
		t.Fatalf(
			"readiness parity = (legacy %#v, scoped %#v, %v)",
			legacyReadiness,
			scopedReadiness.Readiness,
			err,
		)
	}
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

type compatibilityParityHost struct {
	constructionModelHost
	snapshot modelhost.ReadinessSnapshot
}

func (host *compatibilityParityHost) InspectReadiness(
	context.Context,
	*models.RuntimeConfig,
	string,
) (modelhost.ReadinessSnapshot, error) {
	return host.snapshot, nil
}

func readinessSnapshot(runtime models.Runtime) modelhost.ReadinessSnapshot {
	return modelhost.ReadinessSnapshot{
		Identity: modelhost.Identity{
			Name:                runtime.Identity,
			Locality:            runtime.Locality,
			SupportedOperations: runtime.SupportedOperations,
		},
		ReadinessState: runtime.ReadinessState,
		LifecycleState: runtime.LifecycleState,
		Diagnostics:    runtime.Diagnostics,
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
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	return &Root{runtimeScopes: scopes, catalog: catalog}
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
	if len(result.Models) != 2 {
		t.Fatalf("ListCatalog models = %#v, want two canonical identities", result.Models)
	}
	alpha := result.Models[0]
	if localmodels.CanonicalModelName(alpha.Name) != "ALPHA-MODEL" {
		t.Fatalf("first model = %q, want canonical ALPHA-MODEL identity", alpha.Name)
	}
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
	if len(alpha.Resources) != 1 || alpha.Resources[0].Model == nil ||
		*alpha.Resources[0].Model != "ALPHA-MODEL" {
		t.Fatalf("alpha resources = %#v, want detached model resource", alpha.Resources)
	}
	if alpha.Status != models.StatusReady ||
		alpha.ManagedRuntime.Diagnostics["sourceKind"] != localmodels.ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("alpha status/runtime = %#v, want ready managed-mirror projection", alpha)
	}
	if localmodels.CanonicalModelName(result.Models[1].Name) != "ZETA-MODEL" {
		t.Fatalf("second model = %q, want ZETA-MODEL", result.Models[1].Name)
	}
}

func mutateScopedCatalogResult(result models.ListModelsResult) {
	result.Models[0].Name = "mutated"
	result.Models[0].Operations[0].Name = "mutated"
	result.Models[0].Operations[0].Inputs[0].ContentTypes[0] = "mutated"
	*result.Models[0].Resources[0].Model = "mutated"
	result.Models[0].ManagedRuntime.SupportedOperations[0].Name = "mutated"
	result.Models[0].ManagedRuntime.Diagnostics["sourceKind"] = "mutated"
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

func TestBoundServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &Service{}
	_, err := svc.ListCatalog(ctx, models.ListModelsRequest{})
	assertContractOnlyUnsupported(t, "ListCatalog", err)
	_, err = svc.GetCatalogModel(ctx, models.GetModelRequest{})
	assertContractOnlyUnsupported(t, "GetCatalogModel", err)
	_, err = svc.GetModelReadiness(ctx, models.GetModelReadinessRequest{})
	assertContractOnlyUnsupported(t, "GetModelReadiness", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = svc.EnsureModelHost(ctx, models.EnsureModelHostRequest{})
	assertContractOnlyUnsupported(t, "EnsureModelHost", err)
	_, err = svc.InspectModelHost(ctx, models.InspectModelHostRequest{})
	assertContractOnlyUnsupported(t, "InspectModelHost", err)
	_, err = svc.StopModelHost(ctx, models.StopModelHostRequest{})
	assertContractOnlyUnsupported(t, "StopModelHost", err)
	_, err = svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "AcquireModelLease", err)
	_, err = svc.GetModelLease(ctx, models.GetModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "GetModelLease", err)
	_, err = svc.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "ReleaseModelLease", err)
	_, err = svc.InvokeModelWithLease(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModelWithLease", err)
	_, err = svc.CancelInvocation(ctx, models.CancelInvocationRequest{})
	assertContractOnlyUnsupported(t, "CancelInvocation", err)
}

func TestRuntimeServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &runtimeService{}
	_, err := svc.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = svc.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
}
