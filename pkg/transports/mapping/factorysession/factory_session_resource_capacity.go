package factorysession

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const resourceCapacityOperation = "resource.capacity.set"

// ResourceCapacityRequestFromAPI maps the public capacity body and stable path
// identity into the shared live-change request contract.
func ResourceCapacityRequestFromAPI(
	resourceID string,
	request factoryapi.FactorySessionResourceCapacityRequest,
) (factorysessions.LiveChangeRequest, error) {
	value, err := json.Marshal(request.Capacity)
	if err != nil {
		return factorysessions.LiveChangeRequest{}, fmt.Errorf("encode resource capacity: %w", err)
	}
	return factorysessions.LiveChangeRequest{
		RequestID:        strings.TrimSpace(request.RequestId),
		ExpectedRevision: request.ExpectedRevision,
		Operation:        resourceCapacityOperation,
		TargetID:         strings.TrimSpace(resourceID),
		RequestedValue:   value,
		Reason:           strings.TrimSpace(derefString(request.Reason)),
	}, nil
}

// ResourceCapacityResponseToAPI maps a detached live-change result to the
// public capacity response. The runtime result carries all accounting fields,
// including replay-safe values retained by the canonical change event.
func ResourceCapacityResponseToAPI(result factorysessions.LiveChangeResult) factoryapi.FactorySessionResourceCapacityResponse {
	capacity := result.ResourceCapacity
	response := factoryapi.FactorySessionResourceCapacityResponse{
		SessionId: result.SessionID,
		RequestId: result.RequestID,
		ChangeId:  result.ChangeID,
		Revision:  result.NewRevision,
		Outcome:   factoryapi.FactorySessionResourceCapacityOutcome(result.Outcome),
	}
	if capacity == nil {
		return response
	}
	response.ResourceId = capacity.ResourceID
	response.PreviousCapacity = capacity.PreviousCapacity
	response.RequestedCapacity = capacity.RequestedCapacity
	response.EffectiveCapacity = capacity.EffectiveCapacity
	response.InUseCount = capacity.InUseCount
	response.AvailableCount = capacity.AvailableCount
	response.MinimumCapacity = capacity.MinimumCapacity
	if name := strings.TrimSpace(capacity.ResourceName); name != "" {
		response.ResourceName = &name
	}
	return response
}

func ResourceCapacityResponseLinks(sessionID string) *factoryapi.FactorySessionResourceCapacityLinks {
	base := "/factory-sessions/" + strings.TrimSpace(sessionID)
	events := base + "/events"
	status := base + "/status"
	return &factoryapi.FactorySessionResourceCapacityLinks{
		Session: &base,
		Events:  &events,
		Status:  &status,
	}
}

// ResourceCapacityErrorDetailsToAPI maps the safe accounting attached to a
// capacity-in-use rejection. Runtime tokens and other implementation details
// never cross this boundary.
func ResourceCapacityErrorDetailsToAPI(result *factoryruntime.ResourceCapacityResult) *factoryapi.FactorySessionResourceCapacityErrorDetails {
	if result == nil {
		return nil
	}
	return &factoryapi.FactorySessionResourceCapacityErrorDetails{
		ResourceId:        result.ResourceID,
		CurrentCapacity:   result.PreviousCapacity,
		RequestedCapacity: result.RequestedCapacity,
		InUseCount:        result.InUseCount,
		AvailableCount:    result.AvailableCount,
		MinimumCapacity:   result.MinimumCapacity,
	}
}
