package observe

import (
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
)

func decodeObserveHTTPRequest(body io.Reader) (factoryvisualization.ObserveRequest, error) {
	req, err := common.DecodeStrictJSON[ObserveHTTPRequest](body)
	if err != nil {
		return factoryvisualization.ObserveRequest{}, err
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
