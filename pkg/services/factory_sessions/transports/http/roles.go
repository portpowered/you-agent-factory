package http

import (
	"context"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type responseWriter struct{ logger *zap.Logger }

type LifecycleHandler struct {
	responseWriter
	sessionsRoot    sessionLifecycleOperations
	liveControl     liveControlOperations
	sessionDeletion factorysessions.LiveDeletionService
	sessionRequests RequestPreparation
}
type ReadHandler struct {
	responseWriter
	sessionsRoot      sessionReadOperations
	sessions          apisurface.LiveSessionAPI
	liveSessionLister LiveSessionListReader
	sessionRequests   RequestPreparation
}
type AuthoringHandler struct {
	responseWriter
	currentFactory     apisurface.CurrentFactoryReader
	factoryDefinitions apisurface.FactorySaveAPI
	factoryValidation  factorydefinitions.SubmittedDefinitionValidationOperation
	workflowPreview    factoryruntime.WorkflowPreviewOperation
	workerPrompts      workers.PromptTemplates
}
type InvocationHandler struct {
	responseWriter
	invocation apisurface.InvocationAPI
}
type ObservationHandler struct {
	responseWriter
	sessionsRoot    sessionResponseOperations
	sessions        apisurface.LiveSessionAPI
	factoryStatus   apisurface.FactoryStatusAPI
	sessionRequests RequestPreparation
}

func NewLifecycleHandler(root sessionLifecycleOperations, control liveControlOperations, deletion factorysessions.LiveDeletionService, requests RequestPreparation, logger *zap.Logger) *LifecycleHandler {
	return &LifecycleHandler{responseWriter{logger}, root, control, deletion, requests}
}
func NewReadHandler(root sessionReadOperations, live apisurface.LiveSessionAPI, liveList LiveSessionListReader, requests RequestPreparation, logger *zap.Logger) *ReadHandler {
	return &ReadHandler{responseWriter{logger}, root, live, liveList, requests}
}
func NewAuthoringHandler(current apisurface.CurrentFactoryReader, definitions apisurface.FactorySaveAPI, validation factorydefinitions.SubmittedDefinitionValidationOperation, preview factoryruntime.WorkflowPreviewOperation, prompts workers.PromptTemplates, logger *zap.Logger) *AuthoringHandler {
	return &AuthoringHandler{responseWriter{logger}, current, definitions, validation, preview, prompts}
}
func NewInvocationHandler(invocation apisurface.InvocationAPI, logger *zap.Logger) *InvocationHandler {
	return &InvocationHandler{responseWriter{logger}, invocation}
}
func NewObservationHandler(root sessionResponseOperations, live apisurface.LiveSessionAPI, status apisurface.FactoryStatusAPI, requests RequestPreparation, logger *zap.Logger) *ObservationHandler {
	return &ObservationHandler{responseWriter{logger}, root, live, status, requests}
}
func NewDurableLifecycleHandler(root sessionLifecycleOperations, requests RequestPreparation, logger *zap.Logger) *LifecycleHandler {
	return &LifecycleHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}
func NewDurableReadHandler(root sessionReadOperations, requests RequestPreparation, logger *zap.Logger) *ReadHandler {
	return &ReadHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}
func NewDurableAuthoringHandler(validation factorydefinitions.SubmittedDefinitionValidationOperation, logger *zap.Logger) *AuthoringHandler {
	return &AuthoringHandler{responseWriter: responseWriter{logger}, factoryValidation: validation}
}
func NewUnavailableInvocationHandler(logger *zap.Logger) *InvocationHandler {
	return &InvocationHandler{responseWriter: responseWriter{logger}}
}
func NewDurableObservationHandler(root sessionResponseOperations, requests RequestPreparation, logger *zap.Logger) *ObservationHandler {
	return &ObservationHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}

// Each transport capability consumes owner-root request and detached result
// contracts, without the legacy root's engine-bearing read context.
type sessionLifecycleOperations interface {
	Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error)
	Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error)
	List(context.Context, factorysessions.SessionListRequest) (factorysessions.SessionListResult, error)
	Control(context.Context, factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error)
	ReadSessionDetail(context.Context, string) (factorysessions.SessionDetail, error)
	ApplyLiveChange(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error)
}
type sessionReadOperations interface {
	Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error)
	ReadSessionDetail(context.Context, string) (factorysessions.SessionDetail, error)
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}
type sessionResponseOperations interface {
	SubscribeResponses(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error)
}
type liveControlOperations interface {
	ReadSessionDetail(context.Context, string) (factorysessions.SessionDetail, error)
	PauseLiveFactorySession(context.Context, string, factorysessions.LiveControlRequest) (factorysessions.LiveControlResult, error)
	ResumeLiveFactorySession(context.Context, string, factorysessions.LiveControlRequest) (factorysessions.LiveControlResult, error)
}
