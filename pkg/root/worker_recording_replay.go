package root

import (
	"context"
	"encoding/json"
	"fmt"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type workerRecordingReaderAdapter struct {
	reader processcontract.WorkerRecordingReader
}

func (adapter workerRecordingReaderAdapter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	payload, err := adapter.reader.LoadWorkerRecording(ctx, recordingID)
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("decode Worker recording snapshot: %w", err)
	}
	return snapshot, nil
}

// WorkerRecordingReaderFromProcess returns the Recordings-owned read
// capability carried by the process. The process retains only this selected
// capability; it does not expose the application service graph or capture
// writer.
func WorkerRecordingReaderFromProcess(
	process *initializerapplication.Process,
) recordings.WorkerRecordingReader {
	if process == nil {
		return nil
	}
	reader := process.WorkerRecordingReader()
	if reader == nil {
		return nil
	}
	return workerRecordingReaderAdapter{reader: reader}
}
