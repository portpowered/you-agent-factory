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
		func(_ context.Context, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
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
		func(context.Context, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
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
