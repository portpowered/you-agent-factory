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
