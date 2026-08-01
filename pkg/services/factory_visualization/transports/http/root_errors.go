package http

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/errors"
)

// Keep the established package-local seams available to existing transport
// tests while the error policy lives in the focused errors package.
func visualizationRootErrorResponse(err error) (int, any, bool) {
	return transporterrors.RootErrorResponse(err)
}

func visualizationRequestContextErrorResponse(err error) (int, any, bool) {
	return common.RequestContextErrorResponse(err)
}
