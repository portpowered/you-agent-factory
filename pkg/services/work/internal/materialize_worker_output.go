package internal

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// materializeWorkerOutput keeps the application service's private method
// narrow while the Work root owns the pure proposal materialization operation.
func materializeWorkerOutput(
	ctx context.Context,
	request work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	return work.MaterializeWorkerOutput(ctx, request)
}
