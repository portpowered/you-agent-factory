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
type sessionRole struct {
	factorysessions.Service
	listSessions func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

func (role *sessionRole) ListSessions(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	if role.listSessions == nil {
		return factorysessions.ListSessionsResult{}, nil
	}
	return role.listSessions(ctx, request)
}

type invocationRole struct {
	factorysessionmapping.SessionInvoker
}
type executionRole struct {
	factorysessions.ExecutionService
}
type durableLifecycleRole struct {
	factorysessionmapping.DurableLifecycleAPI
}
type workRole struct{ work.Service }
type modelRole struct{ models.Service }
type workerRole struct{ workers.Service }
type providerSessionRole struct{ providersessions.Service }
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

func (role *requestPreparationRole) PrepareListSessions(
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsRequest, error) {
	return request, nil
}

func TestHandlerBindsOpenedRolesWithoutReconstructingStableGraph(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler, err := NewHandler(
		mappings,
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
		FactorySessions: &sessionRole{}, SessionInvocation: &invocationRole{},
		SessionExecution: &executionRole{}, Work: &workRole{}, Models: &modelRole{},
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

func TestHandlerBindForwardsSessionsRootToHTTPTransport(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	handler, err := NewHandler(
		mappings,
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
	root := &sessionRole{
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopeAll {
				t.Fatalf("root durable listing scope = %q, want all", request.Scope)
			}
			return factorysessions.ListSessionsResult{
				DurableSessions: []factorysessions.DurableSessionListSummary{{
					SessionID: "root-session",
					Status:    factorysessions.LifecycleStatusSucceeded,
				}},
			}, nil
		},
	}
	opened := factorysessions.RuntimeHTTPServices{
		FactoryRuntime: &runtimeRole{}, FactoryDefinitions: &definitionRole{},
		FactorySessions: root, SessionInvocation: &invocationRole{},
		SessionExecution: &executionRole{}, Work: &workRole{}, Models: &modelRole{},
		Workers: &workerRole{}, ProviderSessions: &providerSessionRole{},
		Logger: zap.NewNop(),
	}

	bound, err := handler.Bind(opened)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=persisted", nil)
	bound.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want root-backed listing", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "root-session") {
		t.Fatalf("body = %s; want root-session from opened Factory Sessions root", response.Body.String())
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
			return NewHandler(nil, modelsContent, validation, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"models content preparation": func() (*Handler, error) {
			return NewHandler(mappings, nil, validation, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"validation": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, nil, invocationWorkType, contentStaging, requestPreparation, sessionRequests)
		},
		"invocation work type": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, nil, contentStaging, requestPreparation, sessionRequests)
		},
		"content staging": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, invocationWorkType, nil, requestPreparation, sessionRequests)
		},
		"request preparation": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, invocationWorkType, contentStaging, nil, sessionRequests)
		},
		"session requests": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, invocationWorkType, contentStaging, requestPreparation, nil)
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
		&executionRole{},
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
	execution := &executionRole{}
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
