package worker_capture

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerSessionCatalogEntry is a rebuildable identity index, never an execution authority.
type WorkerSessionCatalogEntry struct {
	WorkName              string `json:"workName,omitempty"`
	Version               int    `json:"version"`
	WorkerSessionID       string `json:"workerSessionId"`
	RecordingID           string `json:"recordingId"`
	OriginatingArtifact   string `json:"originatingArtifact,omitempty"`
	RecordingGenerationID string `json:"recordingGenerationId"`
	Origin                string `json:"origin"`
	FactorySessionID      string `json:"factorySessionId,omitempty"`
	OwnerEpoch            string `json:"ownerEpoch"`
	CommittedPosition     uint64 `json:"committedPosition"`
}

// WorkerCapturedCatalogRequest pages detached capture identities, not live handles.
// Membership changes invalidate continuation; the fleet query owns observation snapshots.
type WorkerCapturedCatalogRequest struct {
	Limit     int
	NextToken string
	// FactorySessionID restricts membership and its generation to one Factory
	// scope. Empty retains the complete profile catalog.
	FactorySessionID string
	// RequireCompleteMembership fails closed when damage or ambiguity prevents
	// proving association absence or uniqueness. Default reads retain healthy histories.
	RequireCompleteMembership bool
	// PreparedSummariesOnly refuses unprepared retained data instead of loading
	// journals. Ordinary association/capability queries use this after activation.
	PreparedSummariesOnly bool
}

type WorkerCapturedCatalogPage struct {
	Items        []WorkerCapturedCatalogItem
	GenerationID string
	NextToken    string
}

// WorkerCapturedCatalogItem contains only committed summary facts. MetadataRecords
// retain the latest usage and supported session fields, plus the terminal record;
// output/progress history belongs to ReadWorkerCapturedActivity. CapturedAt contains
// stamps for the opening and selected metadata only. Missing legacy stamps stay absent.
type WorkerCapturedCatalogItem struct {
	// SuccessorWorkerSessionID is derived from a committed successor opening in
	// the same profile and Factory scope; it never changes the source journal.
	SuccessorWorkerSessionID string
	// OwnerLost establishes that an unfinished capture's recorded host epoch
	// differs from this store's current epoch. Legacy captures cannot prove loss.
	OwnerLost       bool
	Catalog         WorkerSessionCatalogEntry
	Opening         events.Record
	MetadataRecords []events.Record
	CapturedAt      map[string]time.Time
	Terminal        *WorkerRecordingTerminal
	Health          WorkerRecordingStatus
	HealthReason    string
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
	SuccessorWorkerSessionID string
	// OwnerLost proves an unfinished capture belongs to an earlier known host epoch.
	OwnerLost    bool
	Catalog      WorkerSessionCatalogEntry
	Health       WorkerRecordingStatus
	HealthReason string
	Opening      events.Record
	Terminal     *WorkerRecordingTerminal
	Records      []WorkerCapturedRecord
	TokenUsage   *workers.UsagePayload
	NextToken    string
}
