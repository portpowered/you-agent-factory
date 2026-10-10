package http

import (
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
	return prepare.PrepareListSessions(raw)
}

func decodeGetFactorySessionRequest(sessionID factoryapi.SessionID) string {
	return string(sessionID)
}
