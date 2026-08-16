// Package application owns immutable binding of opened product service roles
// to the process-scoped HTTP transport graph assembled by Wire.
package application

import (
	"fmt"
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// RuntimeBinding is the Wire-owned operation that supplies opened owner roles
// to the already-composed HTTP server shell. The application package only
// invokes this operation; it does not construct service adapters or mappings.
type RuntimeBinding func(Binding) (http.Handler, error)

// Handler is the inert HTTP application role constructed by Wire. Runtime
// owner-adapter construction stays in Wire, while this role retains the
// standalone durable-execution compatibility entry point.
type Handler struct {
	runtimeBinding     RuntimeBinding
	validation         factorydefinitions.SubmittedDefinitionValidationOperation
	invocationWorkType factorydefinitions.InvocationWorkTypeService
	sessionRequests    factorysessionshttp.RequestPreparation
}

// Binding is the transport-owned application role set. Factory Sessions
// opens runtime state, while Wire supplies these already-constructed roles to
// the HTTP adapter without publishing an application service table from the
// Sessions contract.
type Binding struct {
	FactoryRuntime     factoryruntime.Service
	FactoryDefinitions factorydefinitions.Service
	WorkflowPreview    factoryruntime.WorkflowPreviewOperation
	FactorySessions    factorysessions.Service
	Recordings         recordings.Service
	LiveControl        factorysessions.LiveControlService
	Work               work.Service
	Models             models.Service
	ModelsScope        models.RuntimeScopeRef
	ModelInvoker       workers.ModelInvoker
	Workers            workers.Service
	ProviderSessions   providersessions.Service
	WorkerSessions     workersessions.ObservationService
	WorkerPrompts      workers.PromptTemplates
	Logger             *zap.Logger
}

func NewHandler(
	runtimeBinding RuntimeBinding,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
) (*Handler, error) {
	if runtimeBinding == nil || validation == nil || invocationWorkType == nil || sessionRequests == nil {
		return nil, fmt.Errorf("construct HTTP handler: runtime binding, validation, invocation work-type policy, and Factory Session request preparation are required")
	}
	return &Handler{
		runtimeBinding: runtimeBinding,
		validation:     validation, invocationWorkType: invocationWorkType,
		sessionRequests: sessionRequests,
	}, nil
}

func (handler *Handler) Bind(opened Binding) (http.Handler, error) {
	if handler == nil || handler.runtimeBinding == nil {
		return nil, fmt.Errorf("bind HTTP handler: Wire runtime binding is required")
	}
	return handler.runtimeBinding(opened)
}

// BindDurableExecution binds the same generated API and embedded dashboard
// server to a standalone JavaScript execution scope. Routes outside that
// scope retain their normal not-configured behavior.
func (handler *Handler) BindDurableExecution(
	execution factorysessionmapping.DurableExecution,
	logger *zap.Logger,
) (http.Handler, error) {
	if handler == nil || handler.sessionRequests == nil || execution == nil {
		return nil, fmt.Errorf("bind durable execution HTTP handler: process-scoped handler and execution are required")
	}
	durable := factorysessionmapping.NewDurableAPI(execution)
	sessionsHandler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		DurableExecution: durable, DurableLifecycle: durable,
		DurableListing: durable, DurableProjection: durable,
		DurableLister: execution, FactoryValidation: handler.validation,
		InvocationWorkType: handler.invocationWorkType,
		SessionRequests:    handler.sessionRequests,
	}, logger)
	return transporthttp.NewServer(sessionsHandler, nil, nil, nil, nil, logger).Handler(), nil
}
