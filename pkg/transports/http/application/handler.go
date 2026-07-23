// Package application owns immutable binding of opened product service roles
// to the process-scoped HTTP transport graph assembled by Wire.
package application

import (
	"fmt"
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	mappingcomposition "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
)

// Handler is the complete inert HTTP binding operation constructed by Wire.
// Bind supplies the opened runtime roles to runtime-bound mapping and protocol
// views before injecting those completed views into the top-level server.
type Handler struct {
	mappings           *mappingcomposition.HTTPBinder
	modelsContent      work.ContentPreparation
	validation         factorydefinitions.SubmittedDefinitionValidationOperation
	contentStaging     work.ContentStagingService
	requestPreparation work.RequestPreparationService
	sessionRequests    factorysessionshttp.RequestPreparation
}

func NewHandler(
	mappings *mappingcomposition.HTTPBinder,
	modelsContent work.ContentPreparation,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
) (*Handler, error) {
	if mappings == nil || modelsContent == nil || validation == nil ||
		contentStaging == nil || requestPreparation == nil || sessionRequests == nil {
		return nil, fmt.Errorf("construct HTTP handler: mappings, service handlers, validation, Work operations, and Factory Session operations are required")
	}
	return &Handler{
		mappings: mappings, modelsContent: modelsContent, validation: validation,
		contentStaging: contentStaging, requestPreparation: requestPreparation,
		sessionRequests: sessionRequests,
	}, nil
}

func (handler *Handler) Bind(opened factorysessions.RuntimeHTTPServices) (http.Handler, error) {
	if handler == nil || handler.mappings == nil || handler.modelsContent == nil {
		return nil, fmt.Errorf("bind HTTP handler: process-scoped handler is required")
	}
	modelsAdapter := modelshttp.NewAdapter(opened.Models, opened.Workers, handler.modelsContent)
	modelsHandler := modelshttp.NewHandler(modelsAdapter, opened.Logger)
	if modelsHandler == nil {
		return nil, fmt.Errorf("bind HTTP handler: Models service, invoker, content preparation, and logger are required")
	}
	mapped, err := handler.mappings.Bind(
		opened.FactoryRuntime, opened.FactoryDefinitions, opened.FactorySessions,
		opened.SessionInvocation, opened.SessionExecution, opened.Work,
	)
	if err != nil {
		return nil, err
	}
	sessionsHandler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		Runtime: mapped.Runtime, FactoryStatus: mapped.FactoryStatus,
		Sessions: mapped.Sessions, Work: mapped.Work, WorkRead: mapped.WorkRead,
		Invocation: mapped.Invocation, FactoryDefinitions: mapped.FactoryDefinitions,
		FactoryValidation: handler.validation, WorkflowPreview: opened.WorkflowPreview,
		DurableExecution: mapped.Durable, DurableLifecycle: mapped.Durable,
		DurableListing: mapped.Durable, DurableProjection: mapped.Durable,
		DurableLister:     opened.SessionExecution,
		LiveSessionLister: factorysessionshttp.ReadProjectionSessionListReader{Reader: opened.FactorySessions},
		WorkerPrompts:     opened.WorkerPrompts, ContentStaging: handler.contentStaging,
		RequestPreparation: handler.requestPreparation, SessionRequests: handler.sessionRequests,
	}, opened.Logger)
	server := transporthttp.NewServer(sessionsHandler, modelsHandler, opened.ProviderSessions, opened.Logger)
	return server.Handler(), nil
}
