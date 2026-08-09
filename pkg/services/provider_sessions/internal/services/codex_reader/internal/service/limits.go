package service

const (
	maxCodexWalkCandidates          = 64
	maxCodexDiagnosticMessageLength = 200
	maxCodexUnknownEventLabelLength = 64
	maxCodexJSONLReadBufferSize     = 32 << 10
	maxCodexRetainedFieldBytes      = 256 << 10
)

var (
	maxCodexJSONLLinesPerInspection = 500_000
	maxCodexJSONLBytesPerInspection = int64(128 << 20)
	maxCodexJSONLLineBytes          = int64(1 << 20)
	maxCodexTranscriptEntries       = 50_000
	maxCodexRetainedTextBytes       = int64(8 << 20)
	maxCodexDiagnosticRecords       = 256
)

const (
	diagnosticInvalidJSONEvent            = "invalid JSON event record"
	diagnosticTruncatedJSONEvent          = "truncated JSON event record"
	diagnosticInspectionLineLimit         = "inspection line limit reached"
	diagnosticInspectionByteLimit         = "inspection byte limit reached"
	diagnosticInspectionRecordLimit       = "inspection record limit reached"
	diagnosticInspectionTranscriptLimit   = "inspection transcript limit reached"
	diagnosticInspectionRetainedTextLimit = "inspection retained-output limit reached"
	diagnosticInspectionDiagnosticLimit   = "inspection diagnostic limit reached"
)

type parseBudget struct {
	linesRead                 int
	bytesRead                 int64
	transcriptEntries         int
	retainedTextBytes         int64
	diagnosticRecords         int
	stopParsing               bool
	transcriptFull            bool
	retainedTextFull          bool
	diagnosticsFull           bool
	transcriptLimitReported   bool
	retainedTextLimitReported bool
	limitCategory             string
	limitConfigured           int64
	limitObserved             int64
	limitLine                 int
}

func (b *parseBudget) beginLine() bool {
	if b.stopParsing {
		return false
	}
	if b.linesRead >= maxCodexJSONLLinesPerInspection {
		b.stopParsing = true
		return false
	}
	b.linesRead++
	return true
}

func (b *parseBudget) canRetainTranscript() bool {
	return !b.transcriptFull && b.transcriptEntries < maxCodexTranscriptEntries
}

func (b *parseBudget) retainedTranscript() {
	b.transcriptEntries++
	if b.transcriptEntries >= maxCodexTranscriptEntries {
		b.transcriptFull = true
	}
}

func (b *parseBudget) retainText(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	bounded := truncateCodexText(value, maxCodexRetainedFieldBytes)
	truncated := bounded != value
	remaining := maxCodexRetainedTextBytes - b.retainedTextBytes
	if remaining <= 0 {
		b.retainedTextFull = true
		return "", true
	}
	if int64(len(bounded)) > remaining {
		bounded = truncateCodexText(bounded, remaining)
		truncated = true
	}
	b.retainedTextBytes += int64(len(bounded))
	if b.retainedTextBytes >= maxCodexRetainedTextBytes {
		b.retainedTextFull = true
	}
	return bounded, truncated
}

func (b *parseBudget) canRecordDiagnostic() bool {
	return !b.diagnosticsFull && b.diagnosticRecords < maxCodexDiagnosticRecords
}

func (b *parseBudget) recordedDiagnostic() {
	b.diagnosticRecords++
	if b.diagnosticRecords >= maxCodexDiagnosticRecords {
		b.diagnosticsFull = true
	}
}

func (b *parseBudget) setLimit(category string, configured, observed int64, line int) {
	b.stopParsing = true
	b.limitCategory = category
	b.limitConfigured = configured
	b.limitObserved = observed
	b.limitLine = line
}
