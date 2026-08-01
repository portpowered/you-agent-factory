package service

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
)

const (
	// DefaultProgressQueueCapacity is the bounded backlog for best-effort outputs.
	DefaultProgressQueueCapacity = contracts.DefaultProgressQueueCapacity
)

// Output serializes encoded presentation records onto one transport writer.
type Output = contracts.Output

func appendPresentationLine(payload []byte) []byte {
	return contracts.AppendLine(payload)
}

func isPresentationClosedErr(err error) bool {
	return errors.Is(err, contracts.ErrOutputClosed)
}

func isPresentationBackpressureErr(err error) bool {
	return errors.Is(err, contracts.ErrBacklogFull)
}
