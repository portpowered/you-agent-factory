package lifecycle

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
)

func decodeActivateHTTPRequest(body io.Reader) (factoryvisualization.ActivateRequest, error) {
	req, err := common.DecodeStrictJSON[ActivateHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ActivateRequest{}, err
	}
	return factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateMode(req.Mode),
	}, nil
}

func decodeJoinHTTPRequest(body io.Reader) (factoryvisualization.JoinRequest, error) {
	_, err := common.DecodeOptionalJSONRequest(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.JoinRequest{}, err
	}
	return factoryvisualization.JoinRequest{}, nil
}

func decodeStopDrainHTTPRequest(body io.Reader) (factoryvisualization.StopDrainRequest, error) {
	_, err := common.DecodeOptionalJSONRequest(body, func() struct{} { return struct{}{} })
	if err != nil {
		return factoryvisualization.StopDrainRequest{}, err
	}
	return factoryvisualization.StopDrainRequest{}, nil
}
