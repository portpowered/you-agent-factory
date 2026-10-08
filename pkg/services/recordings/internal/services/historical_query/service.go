// Package historicalquery defines the private Recordings historical read
// capability. Cross-service callers use recordings.HistoricalRecordingQuery.
package historicalquery

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Service reads one immutable recording artifact and reconstructs detached
// state without consulting the live ledger.
type Service interface {
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	// ReadHistoricalEvents validates the selected artifact for derived readers
	// that do not consume a reconstructed world or workstation requests.
	ReadHistoricalEvents(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest, []byte) (recordings.HistoricalRecordingQueryResult, error)
}
