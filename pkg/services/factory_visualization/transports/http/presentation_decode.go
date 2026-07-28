package http

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
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
) (factoryvisualization.OpenPresentationRequest, error) {
	req, err := decodeStrictJSON[OpenPresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.OpenPresentationRequest{}, presentationHTTPDecodeError{cause: err}
	}
	return factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryMode(req.Mode),
	}, nil
}

func decodePresentProgressHTTPRequest(
	body io.Reader,
) (factoryvisualization.PresentProgressRequest, error) {
	req, err := decodeStrictJSON[PresentProgressHTTPRequest](body)
	if err != nil {
		return factoryvisualization.PresentProgressRequest{}, presentationHTTPDecodeError{cause: err}
	}
	records := make([]factoryvisualization.ProgressRecord, 0, len(req.Records))
	for _, record := range req.Records {
		records = append(records, factoryvisualization.ProgressRecord{
			Payload: append([]byte(nil), record.Payload...),
		})
	}
	return factoryvisualization.PresentProgressRequest{
		SessionID: factoryvisualization.PresentationSessionID(req.SessionID),
		Records:   records,
	}, nil
}

func decodeFinalizePresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.FinalizePresentationRequest, error) {
	req, err := decodeStrictJSON[FinalizePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.FinalizePresentationRequest{}, presentationHTTPDecodeError{cause: err}
	}
	rootReq := factoryvisualization.FinalizePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(req.SessionID),
	}
	if req.Terminal != nil {
		rootReq.Terminal = &factoryvisualization.TerminalWrite{
			Payload: append([]byte(nil), req.Terminal.Payload...),
		}
	}
	return rootReq, nil
}

func decodeClosePresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.ClosePresentationRequest, error) {
	req, err := decodeStrictJSON[ClosePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ClosePresentationRequest{}, presentationHTTPDecodeError{cause: err}
	}
	return factoryvisualization.ClosePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(req.SessionID),
	}, nil
}
