package codex

import (
	"fmt"
	"strconv"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxRecordBytes          = 1 << 20
	maxDetailBytes          = 1024
	maxFinalContentBytes    = 8 << 20
	maxCodexStreamBytes     = int64(512 << 20)
	maxCodexStreamLines     = int64(1_000_000)
	maxCodexStreamRecords   = int64(1_000_000)
	maxCodexTranscriptFacts = 4096
	maxCodexDiagnosticFacts = 256
	maxCodexRetainedText    = int64(8 << 20)
)

const (
	inspectionLimitCategoryMetadata         = "inspection_limit_category"
	inspectionLimitConfiguredMetadata       = "inspection_limit_configured"
	inspectionLimitObservedMetadata         = "inspection_limit_observed"
	inspectionLimitLineMetadata             = "inspection_limit_line"
	inspectionSourceBytesMetadata           = "inspection_source_bytes"
	inspectionLineCountMetadata             = "inspection_line_count"
	inspectionRecordCountMetadata           = "inspection_record_count"
	inspectionTranscriptTruncatedMetadata   = "inspection_transcript_truncated"
	inspectionDiagnosticsTruncatedMetadata  = "inspection_diagnostics_truncated"
	inspectionRetainedTextTruncatedMetadata = "inspection_retained_text_truncated"
)

type streamLimit struct {
	category   string
	configured int64
	observed   int64
	line       int64
}

func (limit *streamLimit) message(session *providers.SessionRef) string {
	sessionID := "unspecified"
	if session != nil && session.ID != "" {
		sessionID = session.ID
	}
	return fmt.Sprintf(
		"Codex rollout inspection reached %s limit for provider session %q (configured %d, observed %d, line %d)",
		limit.category,
		sessionID,
		limit.configured,
		limit.observed,
		limit.line,
	)
}

func (limit *streamLimit) failure(
	session *providers.SessionRef,
	diagnostics *providers.ExecuteDiagnostics,
) *providers.ExecuteFailure {
	if limit == nil {
		return nil
	}
	return &providers.ExecuteFailure{
		Kind:        providers.ExecuteFailureKindDependency,
		Message:     limit.message(session),
		SessionRef:  cloneSessionRef(session),
		Diagnostics: diagnostics,
	}
}

func cloneSessionRef(reference *providers.SessionRef) *providers.SessionRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}

func inspectionMetadataValue(value int64) string {
	return strconv.FormatInt(value, 10)
}
