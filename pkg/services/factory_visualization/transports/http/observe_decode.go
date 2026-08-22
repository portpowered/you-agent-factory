package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
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

func decodeObserveHTTPRequest(body io.Reader) (factoryvisualization.ObserveRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[ObserveHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ObserveRequest{}, decoded.Diagnostics, observeHTTPDecodeError{cause: err}
	}
	rootReq := factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveMode(decoded.Value.Mode),
	}
	if decoded.Value.Reconnect != nil {
		rootReq.Reconnect = &factoryvisualization.ObserveReconnectCursor{
			AfterEventID:  decoded.Value.Reconnect.AfterEventID,
			AfterSequence: decoded.Value.Reconnect.AfterSequence,
		}
	}
	return rootReq, decoded.Diagnostics, nil
}
