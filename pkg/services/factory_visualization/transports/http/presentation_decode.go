package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
)

type presentationHTTPDecodeError struct {
	cause error
}

func (e presentationHTTPDecodeError) Error() string {
	return e.cause.Error()
}

func (e presentationHTTPDecodeError) Unwrap() error {
	return e.cause
}

func decodeOpenPresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.OpenPresentationRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[OpenPresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.OpenPresentationRequest{}, decoded.Diagnostics, presentationHTTPDecodeError{cause: err}
	}
	return factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryMode(decoded.Value.Mode),
	}, decoded.Diagnostics, nil
}

func decodePresentProgressHTTPRequest(
	body io.Reader,
) (factoryvisualization.PresentProgressRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[PresentProgressHTTPRequest](body)
	if err != nil {
		return factoryvisualization.PresentProgressRequest{}, decoded.Diagnostics, presentationHTTPDecodeError{cause: err}
	}
	records := make([]factoryvisualization.ProgressRecord, 0, len(decoded.Value.Records))
	for _, record := range decoded.Value.Records {
		records = append(records, factoryvisualization.ProgressRecord{
			Payload: append([]byte(nil), record.Payload...),
		})
	}
	return factoryvisualization.PresentProgressRequest{
		SessionID: factoryvisualization.PresentationSessionID(decoded.Value.SessionID),
		Records:   records,
	}, decoded.Diagnostics, nil
}

func decodeFinalizePresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.FinalizePresentationRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[FinalizePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.FinalizePresentationRequest{}, decoded.Diagnostics, presentationHTTPDecodeError{cause: err}
	}
	rootReq := factoryvisualization.FinalizePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(decoded.Value.SessionID),
	}
	if decoded.Value.Terminal != nil {
		rootReq.Terminal = &factoryvisualization.TerminalWrite{
			Payload: append([]byte(nil), decoded.Value.Terminal.Payload...),
		}
	}
	return rootReq, decoded.Diagnostics, nil
}

func decodeClosePresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.ClosePresentationRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[ClosePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ClosePresentationRequest{}, decoded.Diagnostics, presentationHTTPDecodeError{cause: err}
	}
	return factoryvisualization.ClosePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(decoded.Value.SessionID),
	}, decoded.Diagnostics, nil
}
