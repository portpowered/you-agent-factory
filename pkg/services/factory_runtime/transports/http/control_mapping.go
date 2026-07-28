package http

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type runtimeControlHTTPResponse struct {
	Outcome factoryruntime.ControlOutcome `json:"outcome"`
}

func controlResponseFromPauseResult(result factoryruntime.PauseResult) runtimeControlHTTPResponse {
	return runtimeControlHTTPResponse{Outcome: result.Outcome}
}

func controlResponseFromResumeResult(result factoryruntime.ResumeResult) runtimeControlHTTPResponse {
	return runtimeControlHTTPResponse{Outcome: result.Outcome}
}

func controlResponseFromTerminateResult(result factoryruntime.TerminateResult) runtimeControlHTTPResponse {
	return runtimeControlHTTPResponse{Outcome: result.Outcome}
}

func terminateRequestFromAPI(req factoryapi.FactorySessionLifecycleControlRequest) factoryruntime.TerminateRequest {
	return factoryruntime.TerminateRequest{Reason: stringValue(req.Reason)}
}
