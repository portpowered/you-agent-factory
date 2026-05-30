package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// MoveWork applies a synchronous operator relocation on the current service-owned runtime.
func (fs *FactoryService) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.OperatorMoveResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.MoveWork(ctx, workID, stateName, source, requestID)
}
