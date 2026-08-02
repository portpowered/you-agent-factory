package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	mappingcomposition "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

type validationRole struct {
	factorydefinitions.SubmittedDefinitionValidationOperation
}

type runtimeRole struct{ factoryruntime.Service }

func (*runtimeRole) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (*runtimeRole) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

type definitionRole struct{ factorydefinitions.Service }
type sessionRole struct{ factorysessions.Service }

func (*sessionRole) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{
			FactorySessionID: "session-alpha",
			Session: &factorysessions.ScopedLiveSessionSummary{
				ID: "session-alpha",
			},
		},
	}, nil
}

type durableLifecycleRole struct {
	factorysessionmapping.DurableLifecycleAPI
}
type workRole struct{ work.Service }
type modelRole struct{ models.Service }
type workerRole struct{ workers.Service }
type providerSessionRole struct{ providersessions.Service }

func newProviderSessionsHTTPHandler() *providersessionshttp.Handler {
	return providersessionshttp.NewHandler(
		providersessionshttp.NewAdapter(&providerSessionRole{}), zap.NewNop(),
	)
}

type requestPreparationRole struct {
	factorysessionshttp.RequestPreparation
}
type statusProjectorRole struct{}

func (statusProjectorRole) ProjectFactoryStatusFromObservation(observation factoryruntime.Observation) factoryruntime.FactoryStatus {
	return factoryruntime.FactoryStatusFromObservation(observation)
}

type contentStagingRole struct{ work.ContentStagingService }
type contentPreparationRole struct{ work.ContentPreparation }
type workRequestPreparationRole struct{ work.RequestPreparationService }
type invocationWorkTypeRole struct {
	factorydefinitions.InvocationWorkTypeService
}

func TestHandlerBindsOpenedRolesWithoutReconstructingStableGraph(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler, err := NewHandler(
		mappings,
		newProviderSessionsHTTPHandler(),
		&contentPreparationRole{},
		&validationRole{},
		&invocationWorkTypeRole{},
		&contentStagingRole{},
		&workRequestPreparationRole{},
		&requestPreparationRole{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	opened := factorysessions.RuntimeHTTPServices{
		FactoryRuntime: &runtimeRole{}, FactoryDefinitions: &definitionRole{},
		FactorySessions: &sessionRole{}, Work: &workRole{}, Models: &modelRole{},
		Workers: &workerRole{}, ProviderSessions: &providerSessionRole{},
	}
	opened.Logger = zap.NewNop()
	first, err := handler.Bind(opened)
	if err != nil || first == nil {
		t.Fatalf("first Bind = (%T, %v), want handler", first, err)
	}
	second, err := handler.Bind(opened)
	if err != nil || second == nil {
		t.Fatalf("second Bind = (%T, %v), want handler", second, err)
	}
	if first == second {
		t.Fatal("Bind reused session-owned handler state")
	}
}

func TestHandlerBindsSessionsRootAtApplicationEdge(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler, err := NewHandler(
		mappings,
		newProviderSessionsHTTPHandler(),
		&contentPreparationRole{},
		&validationRole{},
		&invocationWorkTypeRole{},
		&contentStagingRole{},
		&workRequestPreparationRole{},
		&requestPreparationRole{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	opened := factorysessions.RuntimeHTTPServices{
		FactoryRuntime: &runtimeRole{}, FactoryDefinitions: &definitionRole{},
		FactorySessions: &sessionRole{}, Work: &workRole{}, Models: &modelRole{},
		Workers: &workerRole{}, ProviderSessions: &providerSessionRole{},
		Logger: zap.NewNop(),
	}
	bound, err := handler.Bind(opened)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil)
	bound.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"session-alpha"`) {
		t.Fatalf("response = %d %s, want Sessions root response", response.Code, response.Body.String())
	}
}

func TestBindRejectsMissingProcessScopedHandler(t *testing.T) {
	t.Parallel()

	if bound, err := (*Handler)(nil).Bind(factorysessions.RuntimeHTTPServices{}); err == nil || bound != nil {
		t.Fatalf("Bind = (%T, %v), want missing process-scoped handler error", bound, err)
	}
}

func TestBindRejectsMissingModelsBinding(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler := &Handler{mappings: mappings, modelsContent: &contentPreparationRole{}}
	if bound, err := handler.Bind(factorysessions.RuntimeHTTPServices{}); err == nil || bound != nil {
		t.Fatalf("Bind = (%T, %v), want missing Models binding error", bound, err)
	}
}

func TestNewHandlerRejectsMissingStableOperations(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	validation := &validationRole{}
	invocationWorkType := &invocationWorkTypeRole{}
	modelsContent := &contentPreparationRole{}
	contentStaging := &contentStagingRole{}
	requestPreparation := &workRequestPreparationRole{}
	sessionRequests := &requestPreparationRole{}
	for name, construct := range map[string]func() (*Handler, error){
		"mappings": func() (*Handler, error) {
			return NewHandler(nil, newProviderSessionsHTTPHandler(), modelsContent, validation, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"Provider Sessions HTTP handler": func() (*Handler, error) {
			return NewHandler(mappings, nil, modelsContent, validation, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"models content preparation": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), nil, validation, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"validation": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), modelsContent, nil, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"invocation work type": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), modelsContent, validation, nil, contentStaging, requestPreparation, sessionRequests)
		},
		"content staging": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), modelsContent, validation, invocationWorkType, nil, requestPreparation, sessionRequests)
		},
		"request preparation": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), modelsContent, validation, invocationWorkType, contentStaging, nil, sessionRequests)
		},
		"session requests": func() (*Handler, error) {
			return NewHandler(mappings, newProviderSessionsHTTPHandler(), modelsContent, validation, invocationWorkType, contentStaging, requestPreparation, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if handler, err := construct(); err == nil || handler != nil {
				t.Fatalf("NewHandler = (%T, %v), want missing dependency error", handler, err)
			}
		})
	}
}

func TestHandlerBindsStandaloneDurableExecution(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler, err := NewHandler(
		mappings,
		newProviderSessionsHTTPHandler(),
		&contentPreparationRole{},
		&validationRole{},
		&invocationWorkTypeRole{},
		&contentStagingRole{},
		&workRequestPreparationRole{},
		&requestPreparationRole{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	bound, err := handler.BindDurableExecution(
		&sessionRole{},
		&durableLifecycleRole{},
		zap.NewNop(),
	)
	if err != nil || bound == nil {
		t.Fatalf("BindDurableExecution = (%T, %v), want handler", bound, err)
	}
}

func TestHandlerRejectsIncompleteDurableExecutionBinding(t *testing.T) {
	t.Parallel()

	valid := &Handler{sessionRequests: &requestPreparationRole{}}
	execution := &sessionRole{}
	lifecycle := &durableLifecycleRole{}
	for name, bind := range map[string]func() (http.Handler, error){
		"handler": func() (http.Handler, error) {
			return (*Handler)(nil).BindDurableExecution(execution, lifecycle, zap.NewNop())
		},
		"execution": func() (http.Handler, error) { return valid.BindDurableExecution(nil, lifecycle, zap.NewNop()) },
		"lifecycle": func() (http.Handler, error) { return valid.BindDurableExecution(execution, nil, zap.NewNop()) },
	} {
		t.Run(name, func(t *testing.T) {
			if bound, err := bind(); err == nil || bound != nil {
				t.Fatalf("BindDurableExecution = (%T, %v), want missing dependency error", bound, err)
			}
		})
	}
}
