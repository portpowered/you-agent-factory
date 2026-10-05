package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
)

func NewLogsService(reader recordings.WorkerCapturedActivityReader, logger logging.Logger) (workersessions.LogsService, error) {
	return internalservice.NewLogReader(reader, logger)
}
