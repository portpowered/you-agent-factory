package wire

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerworkattribution "github.com/portpowered/infinite-you/pkg/services/recordings/internal/worker_work_attribution"
	"os"
)

// NewWorkerWorkAttributionReader binds the same profile's captured catalog and
// canonical-history owner. Legacy writers without reads cannot enrich names.
func NewWorkerWorkAttributionReader(writer recordings.WorkerRecordingWriter, query HistoricalQueryOwner, currentBoard workerworkattribution.CurrentBoardArtifact, readFile recordings.RecordingReadFile, revision func(string) (string, error)) recordings.WorkerWorkAttributionReader {
	captures, _ := writer.(workerworkattribution.CaptureReader)
	if captures == nil {
		captures = uncapturedWorkerActivity{}
	}
	return workerworkattribution.New(captures, workerworkattribution.NewArtifactHistoryReader(revision, query, currentBoard, readFile))
}

type uncapturedWorkerActivity struct{}

func (uncapturedWorkerActivity) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	return recordings.WorkerCapturedActivityPage{}, os.ErrNotExist
}
