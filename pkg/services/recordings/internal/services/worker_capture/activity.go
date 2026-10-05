package worker_capture

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerSessionCatalogEntry is a rebuildable identity index, never an execution authority.
type WorkerSessionCatalogEntry struct {
	Version               int    `json:"version"`
	WorkerSessionID       string `json:"workerSessionId"`
	RecordingID           string `json:"recordingId"`
	RecordingGenerationID string `json:"recordingGenerationId"`
	Origin                string `json:"origin"`
	FactorySessionID      string `json:"factorySessionId,omitempty"`
	OwnerEpoch            string `json:"ownerEpoch"`
	CommittedPosition     uint64 `json:"committedPosition"`
}

// WorkerCapturedRecord separates host commit metadata from source-native payloads.
type WorkerCapturedRecord struct {
	Record        events.Record
	CapturedAt    *time.Time
	Truncated     bool
	OriginalBytes int64
	ReturnedBytes int64
	ArtifactRef   string
}

type WorkerCapturedActivityRequest struct {
	WorkerSessionID string
	Limit           int
	NextToken       string
	BoundPayload    bool
}

// WorkerCapturedActivityPage contains only an accepted prefix of the journal.
type WorkerCapturedActivityPage struct {
	Catalog      WorkerSessionCatalogEntry
	Health       WorkerRecordingStatus
	HealthReason string
	Opening      events.Record
	Terminal     *WorkerRecordingTerminal
	Records      []WorkerCapturedRecord
	TokenUsage   *workers.UsagePayload
	NextToken    string
}
