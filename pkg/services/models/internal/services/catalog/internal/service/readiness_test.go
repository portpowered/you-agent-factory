package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestGetModelReadinessReportsCurrentDetachedTransitions(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-readiness-transitions")
	privateRef := openReadinessScope(t, scopes)
	facts := readinessTransitions()
	queryIndex := 0
	service, err := catalogwire.NewService(
		scopes,
		func(_ context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
			if detail.Name != "scoped-model" {
				t.Fatalf("readiness detail name = %q, want canonical scoped-model", detail.Name)
			}
			current := facts[queryIndex%len(facts)]
			queryIndex++
			return current, nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	request := models.GetModelReadinessRequest{
		Scope: publicScope(t, privateRef), Name: " SCOPED-MODEL ", Operation: "generate",
	}

	for index, want := range facts {
		got, queryErr := service.GetModelReadiness(context.Background(), request)
		if queryErr != nil {
			t.Fatalf("GetModelReadiness transition %d: %v", index, queryErr)
		}
		assertCurrentReadiness(t, got, want)
		mutateReadinessResult(got)
	}

	again, err := service.GetModelReadiness(context.Background(), request)
	if err != nil {
		t.Fatalf("GetModelReadiness after caller mutation: %v", err)
	}
	assertCurrentReadiness(t, again, facts[0])
	if queryIndex != len(facts)+1 {
		t.Fatalf("readiness query count = %d, want %d", queryIndex, len(facts)+1)
	}
}

func TestGetModelReadinessValidatesIdentityAndOperationBeforeQuery(t *testing.T) {
	t.Parallel()

	scopes := newRuntimeScopes(t, "catalog-readiness-validation")
	privateRef := openReadinessScope(t, scopes)
	queryCount := 0
	service, err := catalogwire.NewService(
		scopes,
		func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
			queryCount++
			return models.Runtime{
				ReadinessState: models.ReadinessStateUnsupported,
				LifecycleState: models.LifecycleStateNotApplicable,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	scope := publicScope(t, privateRef)

	tests := []struct {
		name    string
		request models.GetModelReadinessRequest
		want    error
	}{
		{
			name:    "empty identity",
			request: models.GetModelReadinessRequest{Scope: scope},
			want:    models.ErrNotFound,
		},
		{
			name:    "unknown identity",
			request: models.GetModelReadinessRequest{Scope: scope, Name: "missing"},
			want:    models.ErrNotFound,
		},
		{
			name: "unsupported operation",
			request: models.GetModelReadinessRequest{
				Scope: scope, Name: "scoped-model", Operation: "embed",
			},
			want: models.ErrUnsupportedOperation,
		},
	}
	for _, test := range tests {
		if _, queryErr := service.GetModelReadiness(context.Background(), test.request); !errors.Is(queryErr, test.want) {
			t.Errorf("%s error = %v, want %v", test.name, queryErr, test.want)
		}
	}
	if queryCount != 0 {
		t.Fatalf("invalid readiness requests called query %d times, want 0", queryCount)
	}

	got, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: scope, Name: "scoped-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("supported readiness: %v", err)
	}
	if got.Readiness.ReadinessState != models.ReadinessStateUnsupported ||
		got.Readiness.LifecycleState != models.LifecycleStateNotApplicable {
		t.Fatalf("supported non-ready result = %#v, want UNSUPPORTED/NOT_APPLICABLE", got)
	}
	if queryCount != 1 {
		t.Fatalf("supported readiness query count = %d, want 1", queryCount)
	}
}

func openReadinessScope(
	t *testing.T,
	scopes runtimescopes.Service,
) runtimescopes.Reference {
	t.Helper()
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			summarizer := catalogWorker("summarizer", "scoped-model", "summarize")
			summarizer.Operations[0].Inputs[0].Required = true
			generator := catalogWorker("generator", "SCOPED-MODEL", "generate")
			generator.Operations[0].Inputs[0].Required = true
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{
					summarizer,
					generator,
				},
				Resources: []models.RuntimeResource{{
					Name: "model-cache", Type: models.RuntimeResourceTypeModel,
					Model: "SCOPED-MODEL", Backend: "GGUF", Provider: "MODELSCOPE",
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("open readiness scope: %v", err)
	}
	return privateRef
}

func readinessTransitions() []models.Runtime {
	return []models.Runtime{
		readinessFact(models.ReadinessStateMissing, models.LifecycleStateNotInstalled, "missing"),
		readinessFact(models.ReadinessStateLoading, models.LifecycleStateInstalling, "loading"),
		readinessFact(models.ReadinessStateReady, models.LifecycleStateInstalled, "ready"),
		readinessFact(models.ReadinessStateFailed, models.LifecycleStateNotInstalled, "failed"),
		readinessFact(models.ReadinessStateUnsupported, models.LifecycleStateNotApplicable, "unsupported"),
	}
}

func readinessFact(
	readiness models.ReadinessState,
	lifecycle models.LifecycleState,
	transition string,
) models.Runtime {
	required := true
	return models.Runtime{
		Identity:       "query-owned-alias",
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		SupportedOperations: []models.Operation{{
			Name: "query-owned-operation",
			Inputs: []models.OperationSlot{{
				Name: "input", ContentTypes: []string{"QUERY"}, Required: &required,
			}},
		}},
		Diagnostics: map[string]string{
			"transition": transition,
			"revision":   "current-revision",
		},
	}
}

func assertCurrentReadiness(
	t *testing.T,
	got models.GetModelReadinessResult,
	want models.Runtime,
) {
	t.Helper()
	if got.ModelName != "scoped-model" || got.Readiness.Identity != "scoped-model" {
		t.Fatalf("readiness identity = (%q, %q), want canonical scoped-model", got.ModelName, got.Readiness.Identity)
	}
	if got.Readiness.ReadinessState != want.ReadinessState ||
		got.Readiness.LifecycleState != want.LifecycleState {
		t.Fatalf(
			"readiness state = (%s, %s), want (%s, %s)",
			got.Readiness.ReadinessState,
			got.Readiness.LifecycleState,
			want.ReadinessState,
			want.LifecycleState,
		)
	}
	if got.Readiness.Locality != models.LocalityLocal {
		t.Fatalf("readiness locality = %q, want LOCAL", got.Readiness.Locality)
	}
	if operationNames := readinessOperationNames(got.Readiness); !reflect.DeepEqual(
		operationNames,
		[]string{"generate", "summarize"},
	) {
		t.Fatalf("readiness operations = %#v, want catalog operations", operationNames)
	}
	required := got.Readiness.SupportedOperations[0].Inputs[0].Required
	if required == nil || !*required {
		t.Fatalf("readiness required input = %v, want detached true pointer", required)
	}
	if got.Readiness.Diagnostics["sourceId"] != "managed-mirror:SCOPED-MODEL" ||
		got.Readiness.Diagnostics["transition"] != want.Diagnostics["transition"] ||
		got.Readiness.Diagnostics["revision"] != "current-revision" {
		t.Fatalf("readiness diagnostics = %#v, want source and current facts", got.Readiness.Diagnostics)
	}
}

func readinessOperationNames(runtime models.Runtime) []string {
	names := make([]string, len(runtime.SupportedOperations))
	for index, operation := range runtime.SupportedOperations {
		names[index] = operation.Name
	}
	return names
}

func mutateReadinessResult(result models.GetModelReadinessResult) {
	result.ModelName = "mutated"
	result.Readiness.Identity = "mutated"
	result.Readiness.SupportedOperations[0].Name = "mutated"
	result.Readiness.SupportedOperations[0].Inputs[0].ContentTypes[0] = "mutated"
	*result.Readiness.SupportedOperations[0].Inputs[0].Required = false
	result.Readiness.Diagnostics["sourceId"] = "mutated"
	result.Readiness.Diagnostics["transition"] = "mutated"
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

type catalogAvailabilityCase struct {
	name             string
	readiness        models.ReadinessState
	lifecycle        models.LifecycleState
	snapshotLocality models.Locality
	wantStatus       models.Status
	wantLoadState    models.LoadState
}

func catalogAvailabilityCases() []catalogAvailabilityCase {
	return []catalogAvailabilityCase{
		{
			name: "ready installed", readiness: models.ReadinessStateReady,
			lifecycle:  models.LifecycleStateInstalled,
			wantStatus: models.StatusReady, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "ready loaded", readiness: models.ReadinessStateReady,
			lifecycle:  models.LifecycleStateLoaded,
			wantStatus: models.StatusReady, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "missing", readiness: models.ReadinessStateMissing,
			lifecycle:  models.LifecycleStateNotInstalled,
			wantStatus: models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "loading", readiness: models.ReadinessStateLoading,
			lifecycle:  models.LifecycleStateLoading,
			wantStatus: models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "failed", readiness: models.ReadinessStateFailed,
			lifecycle:  models.LifecycleStateInstalled,
			wantStatus: models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "unsupported", readiness: models.ReadinessStateUnsupported,
			lifecycle:  models.LifecycleStateNotApplicable,
			wantStatus: models.StatusUnavailable, wantLoadState: models.LoadStateNotApplicable,
		},
		{
			name: "ready not installed", readiness: models.ReadinessStateReady,
			lifecycle:        models.LifecycleStateNotInstalled,
			snapshotLocality: models.LocalityCloud,
			wantStatus:       models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "ready installing", readiness: models.ReadinessStateReady,
			lifecycle:        models.LifecycleStateInstalling,
			snapshotLocality: models.LocalityCloud,
			wantStatus:       models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "ready loading", readiness: models.ReadinessStateReady,
			lifecycle:        models.LifecycleStateLoading,
			snapshotLocality: models.LocalityCloud,
			wantStatus:       models.StatusUnavailable, wantLoadState: models.LoadStateUnloaded,
		},
		{
			name: "ready not applicable", readiness: models.ReadinessStateReady,
			lifecycle:        models.LifecycleStateNotApplicable,
			snapshotLocality: models.LocalityCloud,
			wantStatus:       models.StatusUnavailable, wantLoadState: models.LoadStateNotApplicable,
		},
	}
}

// Retains the retired private catalog's availability and cache observations at
// the scoped Catalog owner. Readiness is a controlled external observation.
func TestCatalogAvailabilityAndCacheFactsRemainDetachedAcrossListAndDetail(t *testing.T) {
	t.Parallel()
	for _, test := range catalogAvailabilityCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scopes := newRuntimeScopes(t, "catalog-availability")
			scope := publicScope(t, openReadinessScope(t, scopes))
			revision, bytes := "rev-truth", int64(17)
			locality := test.snapshotLocality
			if locality == "" {
				locality = models.LocalityLocal
			}
			current := models.Runtime{
				ReadinessState: test.readiness, LifecycleState: test.lifecycle,
				Locality: locality, Revision: &revision, CacheBytes: &bytes,
				Diagnostics: map[string]string{"hostFact": "preserved"},
			}
			service, err := catalogwire.NewService(scopes,
				func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
					return current, nil
				})
			if err != nil {
				t.Fatal(err)
			}
			list, err := service.ListCatalog(t.Context(), models.ListModelsRequest{Scope: scope})
			if err != nil {
				t.Fatal(err)
			}
			detail, err := service.GetCatalogModel(t.Context(), models.GetModelRequest{Scope: scope, Name: "scoped-model"})
			if err != nil {
				t.Fatal(err)
			}
			listed := catalogNamedSummary(t, list, "scoped-model")
			if !reflect.DeepEqual(listed, detail.Model.Summary) {
				t.Fatalf("list/detail differ: %#v / %#v", listed, detail.Model.Summary)
			}
			assertCatalogCacheProjection(t, detail.Model, test)
			// Mutating returned collections, cache pointers and maps cannot change the
			// next observation or the readiness collaborator's owned snapshot.
			*listed.ManagedRuntime.Revision = "mutated"
			*listed.ManagedRuntime.CacheBytes = 999
			listed.ManagedRuntime.Diagnostics["hostFact"] = "mutated"
			detail.Model.Diagnostics["hostFact"] = "mutated"
			detail.Model.Operations[0].Name = "mutated"
			again, err := service.GetCatalogModel(t.Context(), models.GetModelRequest{Scope: scope, Name: "scoped-model"})
			if err != nil {
				t.Fatal(err)
			}
			assertCatalogCacheProjection(t, again.Model, test)
			if again.Model.Operations[0].Name != "generate" {
				t.Fatal("caller mutation changed catalog operation")
			}
			listAgain, err := service.ListCatalog(t.Context(), models.ListModelsRequest{Scope: scope})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(catalogNamedSummary(t, listAgain, "scoped-model"), again.Model.Summary) {
				t.Fatal("caller mutation changed collection")
			}
		})
	}
}

func assertCatalogCacheProjection(t *testing.T, detail models.Detail, test catalogAvailabilityCase) {
	t.Helper()
	if detail.Status != test.wantStatus || detail.LoadState != test.wantLoadState {
		t.Fatalf("availability = %s/%s, want %s/%s", detail.Status, detail.LoadState, test.wantStatus, test.wantLoadState)
	}
	runtime := detail.ManagedRuntime
	if runtime.Revision == nil || *runtime.Revision != "rev-truth" || runtime.CacheBytes == nil || *runtime.CacheBytes != 17 {
		t.Fatalf("cache facts = %#v, want rev-truth/17", runtime)
	}
	if runtime.Diagnostics["hostFact"] != "preserved" || detail.Diagnostics["hostFact"] != "preserved" {
		t.Fatalf("diagnostics = %#v/%#v, want hostFact", runtime.Diagnostics, detail.Diagnostics)
	}
	if runtime.ReadinessState != test.readiness || runtime.LifecycleState != test.lifecycle {
		t.Fatalf("runtime facts = %#v, want %s/%s", runtime, test.readiness, test.lifecycle)
	}
}

func catalogNamedSummary(t *testing.T, list models.ListModelsResult, name string) models.Summary {
	t.Helper()
	for _, summary := range list.Models {
		if summary.Name == name {
			return summary
		}
	}
	t.Fatalf("catalog has no model %q: %#v", name, list)
	return models.Summary{}
}
