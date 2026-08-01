package checkpoint

import (
	"io"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
)

func decodeCaptureCheckpointRequest(body io.Reader) (factoryruntime.CaptureCheckpointRequest, error) {
	req, err := common.DecodeRequiredJSON[runtimeCaptureCheckpointHTTPRequest](body)
	if err != nil {
		return factoryruntime.CaptureCheckpointRequest{}, err
	}
	return captureCheckpointRequestFromHTTP(req), nil
}

func decodeLoadCheckpointRequest(body io.Reader) (factoryruntime.LoadCheckpointRequest, error) {
	req, err := common.DecodeRequiredJSON[runtimeLoadCheckpointHTTPRequest](body)
	if err != nil {
		return factoryruntime.LoadCheckpointRequest{}, err
	}
	return loadCheckpointRequestFromHTTP(req), nil
}

func decodeRestoreCheckpointRequest(body io.Reader) (factoryruntime.RestoreCheckpointRequest, error) {
	req, err := common.DecodeRequiredJSON[runtimeRestoreCheckpointHTTPRequest](body)
	if err != nil {
		return factoryruntime.RestoreCheckpointRequest{}, err
	}
	return restoreCheckpointRequestFromHTTP(req), nil
}
