// Package http owns HTTP adaptation for Factory Session operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Sessions remain here with the owning service.
package http

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// Handler owns the generated HTTP operation implementations for Factory
// Sessions and their session-scoped Factory and Work resources.
type Adapter struct {
	sessionsRoot       factorysessions.Service
	runtime            apisurface.RuntimeAPI
	factoryStatus      apisurface.FactoryStatusAPI
	sessions           apisurface.LiveSessionAPI
	work               apisurface.WorkAPI
	workRead           apisurface.WorkReadAPI
	invocation         apisurface.InvocationAPI
	factoryDefinitions apisurface.FactorySaveAPI
	factoryValidation  factorydefinitions.SubmittedDefinitionValidationOperation
	workflowPreview    factoryruntime.WorkflowPreviewOperation
	durableExecution   apisurface.DurableSessionExecutionAPI
	durableLifecycle   apisurface.DurableSessionLifecycleAPI
	durableListing     apisurface.DurableSessionListingAPI
	durableProjection  apisurface.DurableSessionProjectionAPI
	durableLister      DurableExecutionSessionLister
	liveSessionLister  LiveSessionListReader
	workerPrompts      workers.PromptTemplates
	workService        work.Service
	sessionRequests    RequestPreparation
	logger             *zap.Logger
}

// Dependencies are the exact injected roles used by the Factory Sessions HTTP
// adapter. They are supplied by the already-opened runtime composition.
type Dependencies struct {
	SessionsRoot       factorysessions.Service
	Runtime            apisurface.RuntimeAPI
	FactoryStatus      apisurface.FactoryStatusAPI
	Sessions           apisurface.LiveSessionAPI
	Work               apisurface.WorkAPI
	WorkRead           apisurface.WorkReadAPI
	Invocation         apisurface.InvocationAPI
	FactoryDefinitions apisurface.FactorySaveAPI
	FactoryValidation  factorydefinitions.SubmittedDefinitionValidationOperation
	WorkflowPreview    factoryruntime.WorkflowPreviewOperation
	DurableExecution   apisurface.DurableSessionExecutionAPI
	DurableLifecycle   apisurface.DurableSessionLifecycleAPI
	DurableListing     apisurface.DurableSessionListingAPI
	DurableProjection  apisurface.DurableSessionProjectionAPI
	DurableLister      DurableExecutionSessionLister
	LiveSessionLister  LiveSessionListReader
	WorkerPrompts      workers.PromptTemplates
	WorkService        work.Service
	SessionRequests    RequestPreparation
}

type RequestPreparation interface {
	PrepareStart(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	PrepareControl(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	PrepareApprove(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	PrepareRetryDispatch(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	PrepareListSessions(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	PrepareResult(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	PrepareEventReconnect(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

// NewHandler constructs an inert Factory Sessions HTTP adapter.
func NewHandler(deps Dependencies, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		sessionsRoot: deps.SessionsRoot,
		runtime: deps.Runtime, factoryStatus: deps.FactoryStatus,
		sessions: deps.Sessions, work: deps.Work, workRead: deps.WorkRead,
		invocation:         deps.Invocation,
		factoryDefinitions: deps.FactoryDefinitions, factoryValidation: deps.FactoryValidation,
		workflowPreview:  deps.WorkflowPreview,
		durableExecution: deps.DurableExecution, durableLifecycle: deps.DurableLifecycle,
		durableListing: deps.DurableListing, durableProjection: deps.DurableProjection,
		durableLister: deps.DurableLister, liveSessionLister: deps.LiveSessionLister,
		workerPrompts: deps.WorkerPrompts, workService: deps.WorkService,
		sessionRequests: deps.SessionRequests,
		logger: logger,
	}
}

// WithWorkService returns a copy bound to the supplied Work root for admission
// and content staging/materialization operations.
func (h *Adapter) WithWorkService(service work.Service) *Adapter {
	if h == nil {
		return nil
	}
	bound := *h
	bound.workService = service
	return &bound
}

// Server is retained as a private receiver alias while the moved handler files
// are kept mechanically identical to the established public behavior.
type Handler = Adapter
type Server = Adapter
