package http

import (
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
	sessionsRoot    factorysessions.Service
	liveControl     factorysessions.LiveControlService
	sessionDeletion factorysessions.LiveDeletionService
	sessionRequests RequestPreparation
}
type ReadHandler struct {
	responseWriter
	sessionsRoot      factorysessions.Service
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
	sessionsRoot    factorysessions.Service
	sessions        apisurface.LiveSessionAPI
	factoryStatus   apisurface.FactoryStatusAPI
	sessionRequests RequestPreparation
}

func NewLifecycleHandler(root factorysessions.Service, control factorysessions.LiveControlService, deletion factorysessions.LiveDeletionService, requests RequestPreparation, logger *zap.Logger) *LifecycleHandler {
	return &LifecycleHandler{responseWriter{logger}, root, control, deletion, requests}
}
func NewReadHandler(root factorysessions.Service, live apisurface.LiveSessionAPI, liveList LiveSessionListReader, requests RequestPreparation, logger *zap.Logger) *ReadHandler {
	return &ReadHandler{responseWriter{logger}, root, live, liveList, requests}
}
func NewAuthoringHandler(current apisurface.CurrentFactoryReader, definitions apisurface.FactorySaveAPI, validation factorydefinitions.SubmittedDefinitionValidationOperation, preview factoryruntime.WorkflowPreviewOperation, prompts workers.PromptTemplates, logger *zap.Logger) *AuthoringHandler {
	return &AuthoringHandler{responseWriter{logger}, current, definitions, validation, preview, prompts}
}
func NewInvocationHandler(invocation apisurface.InvocationAPI, logger *zap.Logger) *InvocationHandler {
	return &InvocationHandler{responseWriter{logger}, invocation}
}
func NewObservationHandler(root factorysessions.Service, live apisurface.LiveSessionAPI, status apisurface.FactoryStatusAPI, requests RequestPreparation, logger *zap.Logger) *ObservationHandler {
	return &ObservationHandler{responseWriter{logger}, root, live, status, requests}
}
func NewDurableLifecycleHandler(root factorysessions.Service, requests RequestPreparation, logger *zap.Logger) *LifecycleHandler {
	return &LifecycleHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}
func NewDurableReadHandler(root factorysessions.Service, requests RequestPreparation, logger *zap.Logger) *ReadHandler {
	return &ReadHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}
func NewDurableAuthoringHandler(validation factorydefinitions.SubmittedDefinitionValidationOperation, logger *zap.Logger) *AuthoringHandler {
	return &AuthoringHandler{responseWriter: responseWriter{logger}, factoryValidation: validation}
}
func NewUnavailableInvocationHandler(logger *zap.Logger) *InvocationHandler {
	return &InvocationHandler{responseWriter: responseWriter{logger}}
}
func NewDurableObservationHandler(root factorysessions.Service, requests RequestPreparation, logger *zap.Logger) *ObservationHandler {
	return &ObservationHandler{responseWriter: responseWriter{logger}, sessionsRoot: root, sessionRequests: requests}
}
