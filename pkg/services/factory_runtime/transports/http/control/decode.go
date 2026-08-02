package control

import (
	"io"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func decodeTerminateControlRequest(body io.Reader) (factoryruntime.TerminateRequest, error) {
	req, err := common.DecodeOptionalJSON[factoryapi.FactorySessionLifecycleControlRequest](body)
	if err != nil {
		return factoryruntime.TerminateRequest{}, err
	}
	return terminateRequestFromAPI(req), nil
}

func decodeMoveWorkRequestBody(body io.Reader) (factoryapi.MoveWorkRequest, error) {
	return common.DecodeRequiredJSON[factoryapi.MoveWorkRequest](body)
}
