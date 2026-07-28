package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

type observeHTTPDecodeError struct {
	cause error
}

func (e observeHTTPDecodeError) Error() string {
	return e.cause.Error()
}

func (e observeHTTPDecodeError) Unwrap() error {
	return e.cause
}

func decodeObserveHTTPRequest(body io.Reader) (factoryvisualization.ObserveRequest, error) {
	req, err := decodeStrictJSON[ObserveHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ObserveRequest{}, observeHTTPDecodeError{cause: err}
	}
	rootReq := factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveMode(req.Mode),
	}
	if req.Reconnect != nil {
		rootReq.Reconnect = &factoryvisualization.ObserveReconnectCursor{
			AfterEventID:  req.Reconnect.AfterEventID,
			AfterSequence: req.Reconnect.AfterSequence,
		}
	}
	return rootReq, nil
}
