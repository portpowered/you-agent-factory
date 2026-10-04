package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestServiceOwnsLeaseAndLocalModelInvocation(t *testing.T) {
	worker := modelRuntimeWorker{
		Name: "voice-local", Type: models.RuntimeWorkerTypeModel,
		Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []modelRuntimeResource{{Name: "voice-cache", Capacity: 1}},
	}
	cfg := &testFactoryConfig{
		Name: "test", Workers: []modelRuntimeWorker{worker},
		Resources: []modelRuntimeResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND", Capacity: 1,
		}},
	}
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), cfg)
	host := &leaseTestHost{}
	runtime := &leaseTestRuntime{}
	resources, err := localmodels.NewResourceLimiter(modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scopeA, err := (models.RuntimeScopeRef{}).Parse("factory-session:executor:a")
	if err != nil {
		t.Fatal(err)
	}
	scopeB, err := (models.RuntimeScopeRef{}).Parse("factory-session:executor:b")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releaseA, err := resources.Acquire(ctx, scopeA, loaded, &worker)
	if err != nil || releaseA == nil {
		t.Fatalf("reserve scope A: %v", err)
	}
	defer releaseA()
	execution, err := newLocalExecutor(
		func() *modelRuntimeConfig { return loaded },
		host, leaseTestAssets{}, runtime, resources, modelseffects.LocalRuntimeHooks{},
		time.Now,
	)
	if err != nil {
		t.Fatalf("new local execution: %v", err)
	}

	result, err := execution.InvokeLocal(ctx, models.LocalInvocationRequest{
		Scope:  scopeB,
		Holder: "dispatch-1",
		Worker: models.LocalWorker{
			Name: worker.Name, Type: worker.Type, Model: worker.Model,
			ModelLocality: worker.ModelLocality,
			Resources:     []models.LocalResource{{Name: "voice-cache", Capacity: 1}},
		},
		Resources: []models.LocalResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND", Capacity: 1,
		}},
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Handled || result.Content != "local" || host.acquires != 1 || host.releases != 1 || runtime.loads != 1 {
		t.Fatalf("result=%q acquire/release=%d/%d loads=%d", result.Content, host.acquires, host.releases, runtime.loads)
	}
	// Invocation reserved B, progressed while A was held, and released B.
	releaseB, err := resources.Acquire(ctx, scopeB, loaded, &worker)
	if err != nil || releaseB == nil {
		t.Fatalf("capacity after invocation: %v", err)
	}
	releaseB()
}

func TestLocalExecutorRequiresLeaseCollaborators(t *testing.T) {
	t.Parallel()
	var nilHost *leaseTestHost
	var nilAssets *leaseTestAssets
	var nilRuntime *leaseTestRuntime
	for _, test := range []struct {
		name    string
		host    modelhost.Host
		assets  localmodels.AssetPuller
		runtime localmodels.Runtime
	}{
		{"missing host", nil, leaseTestAssets{}, &leaseTestRuntime{}},
		{"typed nil host", nilHost, leaseTestAssets{}, &leaseTestRuntime{}},
		{"missing assets", &leaseTestHost{}, nil, &leaseTestRuntime{}},
		{"typed nil assets", &leaseTestHost{}, nilAssets, &leaseTestRuntime{}},
		{"missing runtime", &leaseTestHost{}, leaseTestAssets{}, nil},
		{"typed nil runtime", &leaseTestHost{}, leaseTestAssets{}, nilRuntime},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor, err := newLocalExecutor(func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
				test.host, test.assets, test.runtime, mustResourceLimiter(t), modelseffects.LocalRuntimeHooks{}, time.Now)
			if executor != nil || !errors.Is(err, ErrInvalidDependencies) {
				t.Fatalf("constructor = %v, %v; want invalid dependencies", executor, err)
			}
		})
	}
}

func TestLocalExecutorReleasesLeaseAndCapacityOnFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected execution failure")
	for _, test := range []struct {
		name                                  string
		cacheErr, loadErr, invokeErr, wantErr error
		leases, loads, invokes                int
	}{
		{"cache", failure, nil, nil, failure, 1, 0, 0},
		{"load", nil, failure, nil, failure, 1, 2, 0},
		{"invoke", nil, nil, failure, failure, 1, 1, 2},
		{"cancel", nil, nil, context.Canceled, context.Canceled, 1, 1, 2},
		{"unsupported", nil, nil, nil, nil, 0, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:executor:" + test.name)
			if err != nil {
				t.Fatal(err)
			}
			request := models.LocalInvocationRequest{
				Scope: scope, Holder: "dispatch-1",
				Worker: models.LocalWorker{Name: "voice-local", Type: models.RuntimeWorkerTypeModel,
					Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
					Resources: []models.LocalResource{{Name: "voice-cache", Capacity: 1}}},
				Resources: []models.LocalResource{{Name: "voice-cache", Type: models.RuntimeResourceTypeModel,
					Model: "voice", Backend: "TEST", LoadPolicy: "ON_DEMAND", Capacity: 1}},
			}
			config, worker := localExecutionConfiguration(request)
			host := &leaseTestHost{}
			assets := executorFailureAssets{err: test.cacheErr}
			runtime := &executorFailureRuntime{supported: test.leases != 0, loadErr: test.loadErr, invokeErr: test.invokeErr}
			resources := mustResourceLimiter(t)
			executor, err := newLocalExecutor(func() *models.RuntimeConfig { return config },
				host, assets, runtime, resources, modelseffects.LocalRuntimeHooks{}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for attempt := 0; attempt < 2; attempt++ {
				result, err := executor.InvokeLocal(ctx, request)
				if !errors.Is(err, test.wantErr) || result.Handled != (test.leases != 0) || result.Content != "" {
					t.Fatalf("attempt %d: result=%#v error=%v, want %v", attempt, result, err, test.wantErr)
				}
				wantLeases := (attempt + 1) * test.leases
				if host.acquires != wantLeases || host.releases != wantLeases {
					t.Fatalf("attempt %d: acquired/released=%d/%d, want %d each", attempt, host.acquires, host.releases, wantLeases)
				}
				release, err := resources.Acquire(ctx, scope, config, worker)
				if err != nil || release == nil {
					t.Fatalf("capacity after attempt %d: %v", attempt, err)
				}
				release()
			}
			if runtime.loads != test.loads || runtime.invokes != test.invokes {
				t.Fatalf("loads/invokes=%d/%d, want %d/%d", runtime.loads, runtime.invokes, test.loads, test.invokes)
			}
		})
	}
}

type executorFailureAssets struct {
	leaseTestAssets
	err error
}

func (a executorFailureAssets) ResolveModelCache(ctx context.Context, config *models.RuntimeConfig,
	worker *models.RuntimeWorker) (localmodels.CacheLayout, error) {
	if a.err != nil {
		return localmodels.CacheLayout{}, a.err
	}
	return a.leaseTestAssets.ResolveModelCache(ctx, config, worker)
}

type executorFailureRuntime struct {
	supported          bool
	loadErr, invokeErr error
	loads, invokes     int
}

func (r *executorFailureRuntime) Supports(models.RuntimeResource, *models.RuntimeWorker) bool {
	return r.supported
}
func (r *executorFailureRuntime) Load(context.Context, localmodels.LoadRequest) (localmodels.Handle, error) {
	r.loads++
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	return r, nil
}
func (r *executorFailureRuntime) Invoke(context.Context, localmodels.InvocationRequest) (localmodels.InvocationResponse, error) {
	r.invokes++
	return localmodels.InvocationResponse{}, r.invokeErr
}

func TestRootInvokeLocalUsesBoundRuntimeAndReleasesLease(t *testing.T) {
	t.Parallel()

	worker := modelRuntimeWorker{
		Name: "voice-local", Type: models.RuntimeWorkerTypeModel,
		Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []modelRuntimeResource{{Name: "voice-cache"}},
	}
	cfg := &testFactoryConfig{
		Name: "test", Workers: []modelRuntimeWorker{worker},
		Resources: []modelRuntimeResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND",
		}},
	}
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), cfg)
	host := &leaseTestHost{}
	runtime := &leaseTestRuntime{}
	assets := leaseTestAssets{}
	bound, err := newRuntimeWithHostEdges(
		models.RuntimeScopeRef{},
		func() *modelRuntimeConfig { return loaded },
		zap.NewNop(), time.Now, nil, nil, nil, modelseffects.LocalRuntimeHooks{},
		assets, runtime, mustResourceLimiter(t), nil, host,
	)
	if err != nil {
		t.Fatalf("new bound runtime: %v", err)
	}

	scopes, err := runtimescopeswire.NewService(func() string { return "root-local-invocation-test" })
	if err != nil {
		t.Fatalf("runtime scopes: %v", err)
	}
	ref, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return loaded },
	})
	if err != nil {
		t.Fatalf("open runtime scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}

	root := &Root{
		runtimeScopes: scopes,
		runtimeByScope: map[models.RuntimeScopeRef]models.Service{
			scope: bound,
		},
	}
	result, err := root.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Scope:  scope,
		Holder: "dispatch-1",
		Worker: models.LocalWorker{
			Name: worker.Name, Type: worker.Type, Model: worker.Model,
			ModelLocality: worker.ModelLocality,
			Resources:     []models.LocalResource{{Name: "voice-cache"}},
		},
		Resources: []models.LocalResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND",
		}},
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Root.InvokeLocal: %v", err)
	}
	if !result.Handled || result.Content != "local" || host.acquires != 1 || host.releases != 1 || runtime.loads != 1 {
		t.Fatalf("result=%#v acquire/release=%d/%d loads=%d", result, host.acquires, host.releases, runtime.loads)
	}
}

type leaseTestHost struct{ acquires, releases int }

func (*leaseTestHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}
func (*leaseTestHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}
func (*leaseTestHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, nil
}
func (h *leaseTestHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	h.acquires++
	return modelhost.Lease{ID: "lease-1", Endpoint: "http://local"}, nil
}
func (h *leaseTestHost) ReleaseLease(context.Context, string) error {
	h.releases++
	return nil
}
func (*leaseTestHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
}

type leaseTestAssets struct{}

func (leaseTestAssets) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (leaseTestAssets) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (leaseTestAssets) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{ModelName: "voice", CachePath: "cache"}, nil
}
func (leaseTestAssets) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{Supported: true, Installed: true}, nil
}

type leaseTestRuntime struct{ loads int }

func (*leaseTestRuntime) Supports(modelRuntimeResource, *modelRuntimeWorker) bool {
	return true
}
func (r *leaseTestRuntime) Load(context.Context, localmodels.LoadRequest) (localmodels.Handle, error) {
	r.loads++
	return leaseTestHandle{}, nil
}

type leaseTestHandle struct{}

func (leaseTestHandle) Invoke(context.Context, localmodels.InvocationRequest) (localmodels.InvocationResponse, error) {
	return localmodels.InvocationResponse{Content: "local"}, nil
}
func TestResolveModelReferenceAppliesOperatorPrecedenceAndDetachesResult(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("a", 40)
	source := "hf://operator/repository/model.gguf@" + digest
	backend := "operator-backend"
	loadPolicy := models.LoadPolicyOnDemand
	operations := []string{models.OperationTTS}
	overlays := map[string]models.ModelOverlay{
		" LLM ": {
			Source:     &source,
			Backend:    &backend,
			LoadPolicy: &loadPolicy,
			Operations: operations,
		},
	}

	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "llm"},
		overlays,
		nil,
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if result.Definition.Name != models.BuiltInModelNameLLM || result.Definition.Backend != backend {
		t.Fatalf("definition identity/policy = %#v", result.Definition)
	}
	if result.Definition.Source != source {
		t.Fatalf("definition source = %q, want %q", result.Definition.Source, source)
	}
	if len(result.Definition.Operations) != 1 || result.Definition.Operations[0].Name != models.OperationTTS {
		t.Fatalf("definition operations = %#v, want TTS", result.Definition.Operations)
	}
	if result.Provenance.Kind != models.ModelReferenceSourceNamed ||
		result.Provenance.SourceKind != models.ModelReferenceSourceHuggingFace ||
		result.Provenance.ImmutableRevision != digest {
		t.Fatalf("provenance = %#v", result.Provenance)
	}

	updatedSource := "hf://operator/updated/model.gguf@" + digest
	*overlays[" LLM "].Source = updatedSource
	result.Definition.Operations[0].Name = "MUTATED"
	second, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "llm"},
		overlays,
		nil,
	)
	if err != nil {
		t.Fatalf("second resolveModelReference: %v", err)
	}
	if second.Definition.Source != updatedSource {
		t.Fatalf("second definition source = %q, want updated input source", second.Definition.Source)
	}
	if second.Definition.Operations[0].Name != models.OperationTTS {
		t.Fatalf("second definition operations = %#v, want detached TTS", second.Definition.Operations)
	}
}

func TestResolveModelReferenceAddsOperatorModel(t *testing.T) {
	t.Parallel()

	backend := "custom-backend"
	loadPolicy := models.LoadPolicyOnDemand
	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "custom-model"},
		map[string]models.ModelOverlay{
			"custom-model": {
				Source:     stringPointer("./custom-model.gguf"),
				Backend:    &backend,
				LoadPolicy: &loadPolicy,
				Operations: []string{models.OperationASR},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if result.Definition.Name != "custom-model" || result.Definition.Backend != backend ||
		result.Definition.Source != "local://path" {
		t.Fatalf("definition = %#v", result.Definition)
	}
	if result.Provenance.Kind != models.ModelReferenceSourceNamed ||
		result.Provenance.SourceKind != models.ModelReferenceSourceLocalPath {
		t.Fatalf("provenance = %#v", result.Provenance)
	}
	if len(result.Definition.Operations) != 1 || result.Definition.Operations[0].Name != models.OperationASR {
		t.Fatalf("operations = %#v, want ASR", result.Definition.Operations)
	}
}

func TestResolveModelReferenceResolvesAllBuiltInsWithImmutableRevisionLookup(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("b", 64)
	var calls []string
	resolver := func(ctx context.Context, source string) (string, error) {
		calls = append(calls, source)
		return digest, nil
	}
	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		result, err := resolveModelReference(
			context.Background(),
			models.ModelReference{NameOrURI: definition.Name},
			nil,
			resolver,
		)
		if err != nil {
			t.Fatalf("resolve built-in %q: %v", definition.Name, err)
		}
		if result.Definition.Name != definition.Name || result.Readiness != models.ReadinessStateMissing {
			t.Fatalf("built-in %q result = %#v", definition.Name, result)
		}
		if result.Provenance.SourceKind != models.ModelReferenceSourceHuggingFace ||
			result.Provenance.ImmutableRevision == "" {
			t.Fatalf("built-in %q provenance = %#v", definition.Name, result.Provenance)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("revision resolver calls = %d, want 0 for pinned built-ins", len(calls))
	}
}

func TestResolveModelReferenceAcceptsSupportedSourceFormsWithoutLeakingPaths(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("c", 40)
	absolutePath := filepath.Join(t.TempDir(), "weights.gguf")
	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	fileURI := (&url.URL{Scheme: "file", Path: uriPath}).String()
	tests := []struct {
		name string
		ref  string
		kind models.ModelReferenceSourceKind
	}{
		{name: "relative bare", ref: "weights.gguf", kind: models.ModelReferenceSourceLocalPath},
		{name: "relative path", ref: "./weights.gguf", kind: models.ModelReferenceSourceLocalPath},
		{name: "absolute path", ref: absolutePath, kind: models.ModelReferenceSourceLocalPath},
		{name: "file URI", ref: fileURI, kind: models.ModelReferenceSourceFileURI},
		{
			name: "hugging face",
			ref:  "hf://owner/repository/model.gguf@" + digest,
			kind: models.ModelReferenceSourceHuggingFace,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := resolveModelReference(
				context.Background(), models.ModelReference{NameOrURI: test.ref}, nil, nil,
			)
			if err != nil {
				t.Fatalf("resolveModelReference: %v", err)
			}
			if result.Provenance.Kind != test.kind || result.Provenance.SourceKind != test.kind {
				t.Fatalf("provenance = %#v, want %q", result.Provenance, test.kind)
			}
			if strings.Contains(fmt.Sprintf("%#v", result), absolutePath) {
				t.Fatalf("resolved result leaked local path: %#v", result)
			}
		})
	}
}

func TestResolveModelReferenceResolvesUnpinnedRevisionAndPreservesProvenance(t *testing.T) {
	t.Parallel()

	immutable := strings.Repeat("d", 64)
	var requested string
	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@main"},
		nil,
		func(ctx context.Context, source string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			requested = source
			return immutable, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if requested != "hf://owner/repository/model.gguf@main" {
		t.Fatalf("revision lookup source = %q", requested)
	}
	if result.Provenance.Revision != "main" || result.Provenance.ImmutableRevision != immutable {
		t.Fatalf("provenance = %#v", result.Provenance)
	}
	if result.Definition.Source != "hf://owner/repository/model.gguf@"+immutable {
		t.Fatalf("resolved source = %q", result.Definition.Source)
	}
}

func TestResolveModelReferenceClassifiesUnknownAndInvalidEntries(t *testing.T) {
	t.Parallel()

	_, err := resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "missing"},
		map[string]models.ModelOverlay{
			"custom": {Source: stringPointer("./custom.gguf"), Backend: stringPointer("backend"), LoadPolicy: loadPolicyPointer(), Operations: []string{models.OperationOMNI}},
		}, nil,
	)
	var unknown *models.InvocationFailure
	if !errors.As(err, &unknown) || !errors.Is(err, models.ErrModelReferenceUnknown) {
		t.Fatalf("unknown error = %v, want typed unknown reference", err)
	}
	if !reflect.DeepEqual(unknown.ValidNames, []string{"asr", "custom", "embed", "llm", "tts"}) {
		t.Fatalf("valid names = %#v", unknown.ValidNames)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "broken"},
		map[string]models.ModelOverlay{"broken": {}}, nil,
	)
	var configuration models.ModelConfigurationFailure
	if !errors.As(err, &configuration) || configuration.ModelName != "broken" || configuration.Field != "source" {
		t.Fatalf("configuration error = %v, want broken/source", err)
	}
	if _, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: models.BuiltInModelNameLLM},
		map[string]models.ModelOverlay{"broken": {}}, nil,
	); err != nil {
		t.Fatalf("unrelated valid built-in failed: %v", err)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "bad name"},
		map[string]models.ModelOverlay{"bad name": {}}, nil,
	)
	if !errors.As(err, &configuration) || configuration.ModelName != "bad name" || configuration.Field != "name" {
		t.Fatalf("invalid name error = %v, want bad name/name", err)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "http://private.example/model.gguf"}, nil, nil,
	)
	if !errors.Is(err, models.ErrModelReferenceInvalid) || strings.Contains(err.Error(), "private.example") {
		t.Fatalf("invalid source error = %v, want redacted typed error", err)
	}
}

func TestResolveModelReferenceHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := resolveModelReference(
		ctx,
		models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@main"}, nil,
		func(context.Context, string) (string, error) {
			called = true
			return strings.Repeat("e", 40), nil
		},
	)
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled resolve = (%v, called=%t), want context.Canceled without lookup", err, called)
	}
}

func TestRootResolveModelReferenceUsesDetachedScopeOverlay(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "model-resolution-scope-test" })
	if err != nil {
		t.Fatalf("runtime scopes: %v", err)
	}
	root := &Root{
		runtimeScopes: scopes,
		resolveHuggingFaceRevision: func(context.Context, string) (string, error) {
			return strings.Repeat("f", 40), nil
		},
	}
	source := "hf://operator/repository/model.gguf@" + strings.Repeat("a", 40)
	expectedSource := source
	backend := "detached-backend"
	overlays := map[string]models.ModelOverlay{
		"llm": {Source: &source, Backend: &backend, LoadPolicy: loadPolicyPointer(), Operations: []string{models.OperationOMNI}},
	}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			OperatorModels: overlays,
			Runtime:        models.RuntimeConfig{},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	*overlays["llm"].Source = "http://mutated.example/model.gguf"
	result, err := root.ResolveModelReference(context.Background(), models.ResolveModelReferenceRequest{
		Scope:     opened.Scope,
		Reference: models.ModelReference{NameOrURI: "llm"},
	})
	if err != nil {
		t.Fatalf("ResolveModelReference: %v", err)
	}
	if result.Resolved.Definition.Source != expectedSource || result.Resolved.Definition.Backend != backend {
		t.Fatalf("resolved detached scope = %#v", result.Resolved)
	}
}

func stringPointer(value string) *string { return &value }

func loadPolicyPointer() *models.LoadPolicy {
	policy := models.LoadPolicyOnDemand
	return &policy
}

func TestRootInvokeLocalScopedExecutionUsesSelectedRuntime(t *testing.T) {
	t.Parallel()
	runtime := &decliningScopedRuntime{}
	args := newRootConstructionArgs(t)
	args.localRuntime = runtime
	root, err := args.build()
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.workers) != 0 {
		t.Fatal("construction called local runtime")
	}
	scopes := make([]models.RuntimeScopeRef, 2)
	for index := range scopes {
		opened, err := root.OpenRuntimeScope(t.Context(), models.OpenRuntimeScopeRequest{})
		if err != nil {
			t.Fatal(err)
		}
		scopes[index] = opened.Scope
	}
	for index, name := range []string{"scope-a", "scope-b", "scope-a", "scope-b"} {
		result, err := root.InvokeLocal(t.Context(), models.LocalInvocationRequest{
			Scope: scopes[index%2],
			Worker: models.LocalWorker{Name: name, Type: models.RuntimeWorkerTypeModel,
				Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
				Resources: []models.LocalResource{{Name: "voice-cache"}}},
			Resources: []models.LocalResource{{Name: "voice-cache", Type: models.RuntimeResourceTypeModel,
				Model: "voice", Backend: "TEST", LoadPolicy: "ON_DEMAND"}},
		})
		if err != nil || result.Handled {
			t.Fatalf("selected runtime decline: result=%#v error=%v", result, err)
		}
	}
	for _, scope := range scopes {
		if _, err := root.CloseRuntimeScope(t.Context(), models.CloseRuntimeScopeRequest{Scope: scope}); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(runtime.workers, []string{"scope-a", "scope-b", "scope-a", "scope-b"}) {
		t.Fatalf("selected runtime workers=%v", runtime.workers)
	}
}

type decliningScopedRuntime struct{ workers []string }

func (r *decliningScopedRuntime) Supports(_ models.RuntimeResource, worker *models.RuntimeWorker) bool {
	r.workers = append(r.workers, worker.Name)
	return false
}
func (*decliningScopedRuntime) Load(context.Context, localmodels.LoadRequest) (localmodels.Handle, error) {
	panic("declined runtime must not load")
}

func mustResourceLimiter(t *testing.T) *localmodels.ResourceLimiter {
	t.Helper()
	resources, err := localmodels.NewResourceLimiter(modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return resources
}

func TestRootCloseRuntimeScopeRetiresSharedResourcesAndPreservesPeer(t *testing.T) {
	t.Parallel()
	args := newRootConstructionArgs(t)
	root, err := args.build()
	if err != nil {
		t.Fatal(err)
	}
	config := models.RuntimeConfig{Resources: []models.RuntimeResource{{
		Name: "voice", Type: models.RuntimeResourceTypeModel, Model: "voice", Capacity: 1,
		Backend: "TEST", LoadPolicy: "ON_DEMAND",
	}}}
	worker := models.RuntimeWorker{ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []models.RuntimeResource{{Name: "voice", Capacity: 1}}}
	open := func() models.RuntimeScopeRef {
		result, err := root.OpenRuntimeScope(t.Context(), models.OpenRuntimeScopeRequest{
			Config: models.RuntimeScopeConfig{Runtime: config},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.Scope
	}
	scopeA, scopeB := open(), open()
	releaseA, err := args.resources.Acquire(t.Context(), scopeA, &config, &worker)
	if err != nil || releaseA == nil {
		t.Fatalf("reserve A: %v", err)
	}
	closed, err := root.CloseRuntimeScope(t.Context(), models.CloseRuntimeScopeRequest{Scope: scopeA})
	if err != nil || !closed.Closed {
		t.Fatalf("close A: %#v, %v", closed, err)
	}
	releaseA()
	if release, err := args.resources.Acquire(t.Context(), scopeA, &config, &worker); release != nil || !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("reserve closed A: %v, has release=%t", err, release != nil)
	}
	for range 3 {
		release, err := args.resources.Acquire(t.Context(), scopeB, &config, &worker)
		if err != nil || release == nil {
			t.Fatalf("reserve peer B: %v", err)
		}
		release()
	}
}
