package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
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

func decodeActivateHTTPRequest(body io.Reader) (factoryvisualization.ActivateRequest, error) {
	req, err := decodeStrictJSON[ActivateHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ActivateRequest{}, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateMode(req.Mode),
	}, nil
}

func decodeJoinHTTPRequest(body io.Reader) (factoryvisualization.JoinRequest, error) {
	_, err := decodeOptionalJSONRequest(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.JoinRequest{}, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.JoinRequest{}, nil
}

func decodeStopDrainHTTPRequest(body io.Reader) (factoryvisualization.StopDrainRequest, error) {
	_, err := decodeOptionalJSONRequest(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.StopDrainRequest{}, lifecycleHTTPDecodeError{cause: err}
	}
	return factoryvisualization.StopDrainRequest{}, nil
}
