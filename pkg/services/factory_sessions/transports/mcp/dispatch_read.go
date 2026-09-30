package factorysession

import (
	"context"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// listDispatches reads recording history and, while its artifact is
// unavailable, the same process session's canonical dispatch projection.
func listDispatches(ctx context.Context, sessions factorysessions.Service, service RecordingsInspection, input ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.ListFactorySessionDispatchesResponse](ctx); done {
		return response
	}
	if err := validateDispatchStatus(input.Status); err != nil {
		envelope := readErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	if strings.TrimSpace(input.SessionID) == "" {
		envelope := readErrorEnvelope(input.SessionID, &factorysessions.ExecutionValidationError{Field: "sessionId", Message: "sessionId is required"})
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	if service == nil && sessions == nil {
		envelope := readErrorEnvelope(input.SessionID, recordings.ErrServiceUnavailable)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	result, err := listFactorySessionDispatches(ctx, service, input)
	if err != nil && sessions != nil && canUseLiveDispatchFallback(err) {
		query := factorysessions.DispatchQueryRequest{SessionID: input.SessionID, Filters: factorysessions.DispatchFilters{Phase: input.Phase, Status: factorysessions.DispatchStatus(input.Status)}}
		live, liveErr := sessions.QueryDispatches(ctx, query)
		if liveErr == nil {
			result = liveDispatchesResponse(input, live)
			err = nil
		} else {
			err = liveErr
		}
	}
	if err != nil {
		envelope := readErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Result: &result}
}
