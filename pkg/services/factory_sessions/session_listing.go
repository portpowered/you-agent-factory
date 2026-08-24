package factorysessions

// ScopedLiveSessionSummary is the detached, representation-neutral live row
// returned after Factory Sessions has merged workspace and durable live rows.
type ScopedLiveSessionSummary struct {
	ID               string
	FactoryDir       string
	FolderPath       string
	Project          string
	IsDefault        bool
	Target           TargetRef
	Runtime          *RuntimeProjection
	NormalizedTarget *RuntimeLogicalTarget
}

// ScopedSessionListResult is the complete owner-projected result consumed by
// customer transports. Transports map it but do not reapply scope or merge
// policy.
type ScopedSessionListResult struct {
	Scope            SessionListScope
	LiveSessions     []ScopedLiveSessionSummary
	DurableSessions  []DurableSessionListSummary
	RecordedSessions []RecordedSessionListSummary
}

// RecordedSessionListSource identifies the read-only source represented by a
// recorded-history list row. The source is explicit so clients do not need to
// infer provenance from the presence of an artifact reference.
type RecordedSessionListSource string

const (
	RecordedSessionListSourceHistory RecordedSessionListSource = "recorded-history"

	RecordedSessionListFormatV1JSON  = "V1_JSON"
	RecordedSessionListFormatV2JSONL = "V2_JSONL"
)

// RecordedSessionListSummary is the detached Factory Sessions projection of a
// Recordings-owned history artifact. SessionID is the canonical Factory
// Session identity; ArtifactReference is a safe root-relative reference and
// never a host filesystem authority.
type RecordedSessionListSummary struct {
	SessionID         string
	Source            RecordedSessionListSource
	ArtifactReference string
	Format            string
}
