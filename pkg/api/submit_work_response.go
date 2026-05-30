package api

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func submitWorkResponseFromResult(result interfaces.WorkRequestSubmitResult) factoryapi.SubmitWorkResponse {
	response := factoryapi.SubmitWorkResponse{TraceId: result.TraceID}
	if result.WorkID != "" {
		response.WorkId = &result.WorkID
	}
	if result.Name != "" {
		response.Name = &result.Name
	}
	if result.WorkTypeName != "" {
		response.WorkTypeName = &result.WorkTypeName
	}
	return response
}
