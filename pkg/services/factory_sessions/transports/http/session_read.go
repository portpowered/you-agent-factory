package http

import (
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func decodeListFactorySessionsRequest(
	params factoryapi.ListFactorySessionsParams,
	prepare RequestPreparation,
) (factorysessions.ListSessionsRequest, error) {
	raw, err := factorysession.ListSessionsRequestFromAPI(params)
	if err != nil {
		return factorysessions.ListSessionsRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareListSessions(raw)
}

func decodeGetFactorySessionRequest(sessionID factoryapi.SessionID) string {
	return string(sessionID)
}

func normalizeListSessionsScope(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	if request.Scope == "" {
		request.Scope = factorysessions.DefaultSessionListScope
	}
	switch request.Scope {
	case factorysessions.SessionListScopeLive,
		factorysessions.SessionListScopePersisted,
		factorysessions.SessionListScopeAll:
		return request, nil
	default:
		return factorysessions.ListSessionsRequest{}, &factorysessions.ExecutionValidationError{
			Field:   "scope",
			Message: fmt.Sprintf("scope must be live, persisted, or all (got %q)", request.Scope),
		}
	}
}
