package dispatch

import (
	"io"

	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
)

func commonDecodeRequiredJSON[T any](body io.Reader) (T, error) {
	return common.DecodeRequiredJSON[T](body)
}
