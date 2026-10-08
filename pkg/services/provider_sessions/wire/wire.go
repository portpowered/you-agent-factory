// Package wire constructs the captured-only Provider Sessions service.
package wire

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService requires the selected profile's captured activity reader.
func NewService(captured recordings.WorkerCapturedActivityReader) (providersessions.Service, error) {
	return internalservice.New(captured)
}
