package presentation

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
)

func decodeOpenPresentationHTTPRequest(
	body io.Reader,
) (factoryvisualization.OpenPresentationRequest, error) {
	req, err := common.DecodeStrictJSON[OpenPresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.OpenPresentationRequest{}, err
	}
	return factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryMode(req.Mode),
	}, nil
}

func decodePresentProgressHTTPRequest(
	body io.Reader,
) (factoryvisualization.PresentProgressRequest, error) {
	req, err := common.DecodeStrictJSON[PresentProgressHTTPRequest](body)
	if err != nil {
		return factoryvisualization.PresentProgressRequest{}, err
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
	req, err := common.DecodeStrictJSON[FinalizePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.FinalizePresentationRequest{}, err
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
	req, err := common.DecodeStrictJSON[ClosePresentationHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ClosePresentationRequest{}, err
	}
	return factoryvisualization.ClosePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(req.SessionID),
	}, nil
}
