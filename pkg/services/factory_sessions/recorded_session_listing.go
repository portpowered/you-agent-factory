package factorysessions

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
