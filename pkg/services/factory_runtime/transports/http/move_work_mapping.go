package http

import (
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func moveWorkRequestFromAPI(workID string, req factoryapi.MoveWorkRequest) factoryruntime.MoveWorkRequest {
	return factoryruntime.MoveWorkRequest{
		WorkID:    workID,
		StateName: strings.TrimSpace(req.StateName),
		Source:    factoryruntime.WorkMoveSourceAPI,
		RequestID: strings.TrimSpace(stringValue(req.RequestId)),
	}
}

func workResponseFromMoveResult(result factoryruntime.MoveWorkResult) factoryapi.Work {
	work := factoryapi.Work{
		WorkId:       stringPtrIfNotEmpty(result.WorkID),
		WorkTypeName: stringPtrIfNotEmpty(result.WorkTypeID),
	}
	if result.ToState != "" {
		work.State = &factoryapi.WorkState{Name: result.ToState}
	}
	return work
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
