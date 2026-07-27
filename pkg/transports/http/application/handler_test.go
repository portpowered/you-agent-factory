package application

import (
	"context"
	"net/http"
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

func (*runtimeRole) GetEngineStateSnapshot(context.Context) (*factoryruntime.LegacyEngineObservation, error) {
	return nil, nil
}
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

func (statusProjectorRole) ProjectFactoryStatus(*factoryruntime.LegacyEngineObservation) factoryruntime.FactoryStatus {
	return factoryruntime.FactoryStatus{}
}

type contentStagingRole struct{ work.ContentStagingService }
type contentPreparationRole struct{ work.ContentPreparation }
type workRequestPreparationRole struct{ work.RequestPreparationService }

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

func TestNewHandlerRejectsMissingStableOperations(t *testing.T) {
	t.Parallel()

	mappings, err := mappingcomposition.NewHTTPBinder(statusProjectorRole{}, &contentPreparationRole{})
	if err != nil {
		t.Fatalf("NewHTTPBinder: %v", err)
	}
	validation := &validationRole{}
	modelsContent := &contentPreparationRole{}
	contentStaging := &contentStagingRole{}
	requestPreparation := &workRequestPreparationRole{}
	sessionRequests := &requestPreparationRole{}
	for name, construct := range map[string]func() (*Handler, error){
		"mappings": func() (*Handler, error) {
			return NewHandler(nil, modelsContent, validation, contentStaging, requestPreparation, sessionRequests)
		},
		"models content preparation": func() (*Handler, error) {
			return NewHandler(mappings, nil, validation, contentStaging, requestPreparation, sessionRequests)
		},
		"validation": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, nil, contentStaging, requestPreparation, sessionRequests)
		},
		"content staging": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, nil, requestPreparation, sessionRequests)
		},
		"request preparation": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, contentStaging, nil, sessionRequests)
		},
		"session requests": func() (*Handler, error) {
			return NewHandler(mappings, modelsContent, validation, contentStaging, requestPreparation, nil)
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
