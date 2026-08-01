// Package http owns HTTP adaptation for Factory Session operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Sessions remain here with the owning service.
package http

import (
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// Handler owns the generated HTTP operation implementations for Factory
// Sessions and their session-scoped Factory and Work resources.
type Adapter struct {
	WorkHTTP

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
	workRoot           work.Service
	workHTTPFactory    WorkHTTPFactory
	sessionRequests    RequestPreparation
	logger             *zap.Logger
}

// WorkHTTP is the generated protocol surface implemented by Work HTTP. The
// Factory Sessions adapter accepts this surface so the top-level transport can
// compose the Work implementation without a peer transport import here.
type WorkHTTP interface {
	ListWorkBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.ListWorkBySessionIdParams)
	SubmitWorkBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	UpsertWorkRequestBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID, string)
	StageSubmitWorkFileBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetWorkBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.WorkOrTokenID)
	MoveWorkBySessionId(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.WorkOrTokenID)
}

// WorkHTTPFactory is an optional compatibility seam for focused transport
// tests that replace only the Work admission/content role. Production
// composition should inject a completed WorkHTTP instead.
type WorkHTTPFactory func(
	primary work.Service,
	admission work.Service,
	submission apisurface.WorkAPI,
	read apisurface.WorkReadAPI,
) WorkHTTP

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
)

const (
	// SubmitWorkItemTypeMetadataKey records the structured submission item kind.
	SubmitWorkItemTypeMetadataKey = submitWorkItemTypeMetadataKey
	// SubmitWorkFileNameMetadataKey records the original structured file name.
	SubmitWorkFileNameMetadataKey = submitWorkFileNameMetadataKey
)

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
	WorkHTTP           WorkHTTP
	WorkHTTPFactory    WorkHTTPFactory
	WorkService        work.Service
	// WorkRoot is the complete Work root used by the Work-owned HTTP adapter
	// when the process composition can provide it. WorkService remains the
	// compatibility admission/content role used by focused transport tests and
	// standalone durable bindings.
	WorkRoot        work.Service
	SessionRequests RequestPreparation
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
	handler := &Adapter{
		sessionsRoot: deps.SessionsRoot,
		runtime:      deps.Runtime, factoryStatus: deps.FactoryStatus,
		sessions: deps.Sessions, work: deps.Work, workRead: deps.WorkRead,
		invocation:         deps.Invocation,
		factoryDefinitions: deps.FactoryDefinitions, factoryValidation: deps.FactoryValidation,
		workflowPreview:  deps.WorkflowPreview,
		durableExecution: deps.DurableExecution, durableLifecycle: deps.DurableLifecycle,
		durableListing: deps.DurableListing, durableProjection: deps.DurableProjection,
		durableLister: deps.DurableLister, liveSessionLister: deps.LiveSessionLister,
		workerPrompts:   deps.WorkerPrompts,
		WorkHTTP:        deps.WorkHTTP,
		workHTTPFactory: deps.WorkHTTPFactory,
		workService:     deps.WorkService, workRoot: deps.WorkRoot,
		sessionRequests: deps.SessionRequests,
		logger:          logger,
	}
	if handler.WorkHTTP == nil {
		handler.bindWorkHTTP()
	}
	return handler
}

// WithWorkService returns a copy bound to the supplied Work root for admission
// and content staging/materialization operations.
func (h *Adapter) WithWorkService(service work.Service) *Adapter {
	if h == nil {
		return nil
	}
	bound := *h
	bound.workService = service
	bound.workRoot = nil
	bound.bindWorkHTTP()
	return &bound
}

// WithWorkHTTP returns a copy bound to a completed Work HTTP protocol adapter.
// It is used by composition and focused transport tests that already own the
// Work adapter construction.
func (h *Adapter) WithWorkHTTP(adapter WorkHTTP) *Adapter {
	if h == nil {
		return nil
	}
	bound := *h
	bound.WorkHTTP = adapter
	return &bound
}

func (h *Adapter) bindWorkHTTP() {
	if h == nil || h.workHTTPFactory == nil {
		return
	}
	h.WorkHTTP = h.workHTTPFactory(h.workRoot, h.workService, h.work, h.workRead)
}

// Handler and Server aliases retain the generated transport's established
// receiver names while the injected Work routes are promoted below them.
type Handler = Adapter
type Server = Adapter
