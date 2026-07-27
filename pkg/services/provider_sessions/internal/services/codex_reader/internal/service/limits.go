package service

const (
	maxCodexWalkCandidates          = 64
	maxCodexDiagnosticMessageLength = 200
	maxCodexUnknownEventLabelLength = 64
)

var (
	maxCodexJSONLLinesPerInspection = 500_000
	maxCodexJSONLBytesPerInspection = int64(128 << 20)
	maxCodexTranscriptEntries       = 50_000
	maxCodexDiagnosticRecords       = 256
)

const (
	diagnosticInvalidJSONEvent        = "invalid JSON event record"
	diagnosticTruncatedJSONEvent      = "truncated JSON event record"
	diagnosticInspectionLineLimit     = "inspection line limit reached"
	diagnosticInspectionByteLimit     = "inspection byte limit reached"
	diagnosticInspectionTranscriptLimit = "inspection transcript limit reached"
	diagnosticInspectionDiagnosticLimit = "inspection diagnostic limit reached"
)

type parseBudget struct {
	linesRead                 int
	bytesRead                 int64
	transcriptEntries         int
	diagnosticRecords         int
	stopParsing               bool
	transcriptFull            bool
	diagnosticsFull           bool
	transcriptLimitReported   bool
}

func (b *parseBudget) recordBytes(n int64) bool {
	if b.stopParsing {
		return false
	}
	b.bytesRead += n
	if b.bytesRead > maxCodexJSONLBytesPerInspection {
		b.stopParsing = true
		return false
	}
	return true
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

func (b *parseBudget) recordedDiagnostic() {
	b.diagnosticRecords++
	if b.diagnosticRecords >= maxCodexDiagnosticRecords {
		b.diagnosticsFull = true
	}
}
