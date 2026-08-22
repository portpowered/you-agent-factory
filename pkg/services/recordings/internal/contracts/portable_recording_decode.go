package contracts

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/jsoncompat"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

// PortableRecordingDecodeDiagnostics contains safe metadata produced while
// decoding one Factory Session recording. It records ignored paths only; the
// ignored values are never retained for logging or persistence.
type PortableRecordingDecodeDiagnostics struct {
	IgnoredJSONPaths []string
}

// Paths returns a detached, deterministic copy of ignored JSON paths.
func (diagnostics PortableRecordingDecodeDiagnostics) Paths() []string {
	return jsoncompat.SortedUniquePaths(diagnostics.IgnoredJSONPaths)
}

func collectPortableRecordingDecodePaths(
	document []byte,
	recording PortableRecording,
	paths []string,
) ([]string, error) {
	if recording.WorkerHistory == nil || recording.WorkerHistory.WorkerPortableRecording == nil {
		return jsoncompat.SortedUniquePaths(paths), nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(document, &root); err != nil {
		return nil, err
	}
	rawHistory, ok := rawObjectField(root, "workerHistory")
	if !ok {
		return jsoncompat.SortedUniquePaths(paths), nil
	}
	var history map[string]json.RawMessage
	if err := json.Unmarshal(rawHistory, &history); err != nil {
		return nil, err
	}
	workerDocument := make(map[string]json.RawMessage, len(history))
	for key, value := range history {
		if strings.EqualFold(key, "availability") || strings.EqualFold(key, "reason") {
			continue
		}
		workerDocument[key] = value
	}
	encoded, err := json.Marshal(workerDocument)
	if err != nil {
		return nil, err
	}
	_, workerDiagnostics, err := (workerrecording.WorkerRecordingCodec{}).
		DecodeWorkerPortableRecordingWithDiagnostics(encoded)
	if err != nil {
		return nil, err
	}
	workerPaths := jsoncompat.PrefixPaths("$.workerHistory", workerDiagnostics.Paths())
	return jsoncompat.SortedUniquePaths(append(paths, workerPaths...)), nil
}

func rawObjectField(object map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	for key, value := range object {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}
