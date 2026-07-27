package cursor

// Named inspection limits bound store traversal, SQLite reads, blob decoding,
// reconstruction work, and public diagnostics for one Cursor session inspection.
const (
	LimitStoreWalkEntries    = "store_walk_entries"
	LimitStoreCandidates     = "store_candidates"
	LimitQueriedRows         = "queried_rows"
	LimitBlobBytes           = "blob_bytes"
	LimitInspectionBytes     = "inspection_bytes"
	LimitProtobufNesting     = "protobuf_nesting"
	LimitProtobufDecodeWork  = "protobuf_decode_work"
	LimitTranscriptFacts     = "transcript_facts"
	LimitParseDiagnostics    = "parse_diagnostics"
	LimitDiagnosticMessage   = "diagnostic_message_length"
)

const (
	maxStoreWalkEntries    = 100_000
	maxStoreCandidates     = 16
	maxQueriedRows         = 50_000
	maxBlobBytes           = 4 * 1024 * 1024
	maxInspectionBytes     = 64 * 1024 * 1024
	maxProtobufNesting     = 16
	maxProtobufDecodeWork  = 512
	maxTranscriptFacts     = 10_000
	maxParseDiagnostics    = 64
	maxDiagnosticMessage   = 256
)

var testLimitOverrides struct {
	storeWalkEntries    int
	storeCandidates     int
	queriedRows         int
	blobBytes           int
	inspectionBytes     int
	protobufNesting     int
	protobufDecodeWork  int
	transcriptFacts     int
	parseDiagnostics    int
}

func effectiveLimit(override, fallback int) int {
	if override > 0 {
		return override
	}
	return fallback
}
