package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
)

type lifecycleHTTPDecodeError struct {
	cause error
}

func (e lifecycleHTTPDecodeError) Error() string {
	return e.cause.Error()
}

func (e lifecycleHTTPDecodeError) Unwrap() error {
	return e.cause
}

func decodeActivateHTTPRequest(body io.Reader) (factoryvisualization.ActivateRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[ActivateHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ActivateRequest{}, decoded.Diagnostics, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.ActivateRequest{Mode: factoryvisualization.ActivateMode(decoded.Value.Mode)}, decoded.Diagnostics, nil
}

func decodeJoinHTTPRequest(body io.Reader) (factoryvisualization.JoinRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalJSONWithDiagnostics(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.JoinRequest{}, decoded.Diagnostics, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.JoinRequest{}, decoded.Diagnostics, nil
}

func decodeStopDrainHTTPRequest(body io.Reader) (factoryvisualization.StopDrainRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalJSONWithDiagnostics(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.StopDrainRequest{}, decoded.Diagnostics, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.StopDrainRequest{}, decoded.Diagnostics, nil
}
