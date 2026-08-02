// Package application owns immutable binding of opened product service roles
// to the process-scoped HTTP transport graph assembled by Wire.
package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	mappingcomposition "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// Handler is the complete inert HTTP binding operation constructed by Wire.
// Bind supplies the opened runtime roles to runtime-bound mapping and protocol
// views before injecting those completed views into the top-level server.
type Handler struct {
	mappings           *mappingcomposition.HTTPBinder
	modelsContent      work.ContentPreparation
	validation         factorydefinitions.SubmittedDefinitionValidationOperation
	invocationWorkType factorydefinitions.InvocationWorkTypeService
	contentStaging     work.ContentStagingService
	requestPreparation work.RequestPreparationService
	sessionRequests    factorysessionshttp.RequestPreparation
}

func NewHandler(
	mappings *mappingcomposition.HTTPBinder,
	modelsContent work.ContentPreparation,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
) (*Handler, error) {
	if mappings == nil || modelsContent == nil || validation == nil || invocationWorkType == nil ||
		contentStaging == nil || requestPreparation == nil || sessionRequests == nil {
		return nil, fmt.Errorf("construct HTTP handler: mappings, service handlers, validation, invocation work-type policy, Work operations, and Factory Session operations are required")
	}
	return &Handler{
		mappings: mappings, modelsContent: modelsContent, validation: validation,
		invocationWorkType: invocationWorkType,
		contentStaging:     contentStaging, requestPreparation: requestPreparation,
		sessionRequests: sessionRequests,
	}, nil
}

func (handler *Handler) Bind(opened factorysessions.RuntimeHTTPServices) (http.Handler, error) {
	if handler == nil || handler.mappings == nil || handler.modelsContent == nil {
		return nil, fmt.Errorf("bind HTTP handler: process-scoped handler is required")
	}
	modelsAdapter := modelshttp.NewAdapter(
		opened.Models,
		opened.Workers,
		handler.modelsContent,
		opened.ModelsScope,
	)
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
		SessionsRoot: opened.FactorySessions,
		Runtime:      mapped.Runtime, FactoryStatus: mapped.FactoryStatus,
		Sessions: mapped.Sessions, Work: mapped.Work, WorkRead: mapped.WorkRead,
		Invocation: mapped.Invocation, FactoryDefinitions: mapped.FactoryDefinitions,
		FactoryValidation: handler.validation, WorkflowPreview: opened.WorkflowPreview,
		DurableExecution: mapped.Durable, DurableLifecycle: mapped.Durable,
		DurableListing: mapped.Durable, DurableProjection: mapped.Durable,
		DurableLister:      opened.SessionExecution,
		LiveSessionLister:  factorysessionshttp.ReadProjectionSessionListReader{Reader: opened.FactorySessions},
		WorkerPrompts:      opened.WorkerPrompts,
		InvocationWorkType: handler.invocationWorkType,
		SessionRequests:    handler.sessionRequests,
	}, opened.Logger)
	workHandler := workhttp.NewAdapterWithSessionScope(opened.Work, func(ctx context.Context, sessionID string) error {
		_, err := mapped.FactoryDefinitions.GetCurrentFactoryForSession(ctx, sessionID)
		return err
	}).WithDefaultWorkTypeResolver(defaultWorkTypeResolver(mapped.FactoryDefinitions, handler.invocationWorkType))
	server := transporthttp.NewServer(sessionsHandler, workHandler, modelsHandler, opened.ProviderSessions, opened.Logger)
	return server.Handler(), nil
}

func defaultWorkTypeResolver(
	definitions apisurface.FactorySaveAPI,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
) func(context.Context, string) (string, error) {
	return func(ctx context.Context, sessionID string) (string, error) {
		if definitions == nil || invocationWorkType == nil {
			return "", nil
		}
		namedFactory, err := definitions.GetCurrentFactoryForSession(ctx, sessionID)
		if err != nil {
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) || errors.Is(err, apisurface.ErrCurrentFactoryNotFound) {
				return "", nil
			}
			return "", err
		}
		factoryConfig, err := factoryconfigmapping.FactoryConfigFromOpenAPI(namedFactory)
		if err != nil {
			return "", err
		}
		defaultWorkTypeID, err := invocationWorkType.DefaultWorkType(&factoryConfig)
		if err != nil {
			return "", nil
		}
		return defaultWorkTypeID, nil
	}
}

// BindDurableExecution binds the same generated API and embedded dashboard
// server to a standalone JavaScript execution scope. Routes outside that
// scope retain their normal not-configured behavior.
func (handler *Handler) BindDurableExecution(
	execution factorysessions.ExecutionService,
	lifecycle factorysessionmapping.DurableLifecycleAPI,
	logger *zap.Logger,
) (http.Handler, error) {
	if handler == nil || handler.sessionRequests == nil || execution == nil || lifecycle == nil {
		return nil, fmt.Errorf("bind durable execution HTTP handler: process-scoped handler and execution are required")
	}
	durable := factorysessionmapping.NewDurableAPI(execution, lifecycle)
	sessionsHandler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		DurableExecution: durable, DurableLifecycle: durable,
		DurableListing: durable, DurableProjection: durable,
		DurableLister: execution, FactoryValidation: handler.validation,
		InvocationWorkType: handler.invocationWorkType,
		SessionRequests:    handler.sessionRequests,
	}, logger)
	return transporthttp.NewServer(sessionsHandler, nil, nil, nil, logger).Handler(), nil
}
