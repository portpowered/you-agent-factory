package cursor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

var errInspectionCanceled = providersessions.ErrOperationCanceled

type inspection struct {
	ctx             context.Context
	bytesRead       int64
	walkEntries     int
	candidates      int
	rowsQueried     int
	transcriptFacts int
	malformedBlobs  int
	malformedMeta   int
	unknownRecords  int
	protobufWork    int
	exhaustedLimit  string
	diagnostics     []providersessions.LineError
	stopReconstruct bool
}

func newInspection(ctx context.Context) *inspection {
	if ctx == nil {
		ctx = context.Background()
	}
	return &inspection{ctx: ctx}
}

func (ins *inspection) checkCanceled() error {
	if ins == nil {
		return nil
	}
	select {
	case <-ins.ctx.Done():
		return errInspectionCanceled
	default:
		return nil
	}
}

func (ins *inspection) recordWalkEntry() error {
	if ins == nil {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	ins.walkEntries++
	if ins.walkEntries > effectiveLimit(testLimitOverrides.storeWalkEntries, maxStoreWalkEntries) {
		ins.exhaustedLimit = LimitStoreWalkEntries
		return ins.limitError(LimitStoreWalkEntries)
	}
	return nil
}

func (ins *inspection) recordCandidate() error {
	if ins == nil {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	ins.candidates++
	if ins.candidates > effectiveLimit(testLimitOverrides.storeCandidates, maxStoreCandidates) {
		ins.exhaustedLimit = LimitStoreCandidates
		return ins.limitError(LimitStoreCandidates)
	}
	return nil
}

func (ins *inspection) recordRow() error {
	if ins == nil {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	ins.rowsQueried++
	if ins.rowsQueried > effectiveLimit(testLimitOverrides.queriedRows, maxQueriedRows) {
		ins.exhaustedLimit = LimitQueriedRows
		return ins.limitError(LimitQueriedRows)
	}
	return nil
}

func (ins *inspection) consumeBytes(size int) error {
	if ins == nil || size <= 0 {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	if int64(size) > int64(effectiveLimit(testLimitOverrides.blobBytes, maxBlobBytes)) {
		return ins.limitError(LimitBlobBytes)
	}
	ins.bytesRead += int64(size)
	if ins.bytesRead > int64(effectiveLimit(testLimitOverrides.inspectionBytes, maxInspectionBytes)) {
		ins.exhaustedLimit = LimitInspectionBytes
		return ins.limitError(LimitInspectionBytes)
	}
	return nil
}

func (ins *inspection) recordProtobufWork() error {
	if ins == nil {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	ins.protobufWork++
	if ins.protobufWork > effectiveLimit(testLimitOverrides.protobufDecodeWork, maxProtobufDecodeWork) {
		ins.exhaustedLimit = LimitProtobufDecodeWork
		return ins.limitError(LimitProtobufDecodeWork)
	}
	return nil
}

func (ins *inspection) recordTranscriptFact() error {
	if ins == nil {
		return nil
	}
	if err := ins.checkCanceled(); err != nil {
		return err
	}
	ins.transcriptFacts++
	if ins.transcriptFacts > effectiveLimit(testLimitOverrides.transcriptFacts, maxTranscriptFacts) {
		ins.exhaustedLimit = LimitTranscriptFacts
		ins.stopReconstruct = true
		return ins.limitError(LimitTranscriptFacts)
	}
	return nil
}

func (ins *inspection) recordMalformedBlob(position int) {
	if ins == nil {
		return
	}
	ins.malformedBlobs++
	ins.addDiagnostic("cursor_malformed_blob", position)
}

func (ins *inspection) recordMalformedMeta(position int) {
	if ins == nil {
		return
	}
	ins.malformedMeta++
	ins.addDiagnostic("cursor_malformed_meta", position)
}

func (ins *inspection) recordUnknownRecord(position int) {
	if ins == nil {
		return
	}
	ins.unknownRecords++
	ins.addDiagnostic("cursor_unknown_record", position)
}

func (ins *inspection) recordLimitDiagnostic(limit string) {
	if ins == nil {
		return
	}
	ins.addDiagnostic("cursor_limit_exceeded", 0, limit)
}

func (ins *inspection) addDiagnostic(class string, position int, extra ...string) {
	if ins == nil {
		return
	}
	if len(ins.diagnostics) >= effectiveLimit(testLimitOverrides.parseDiagnostics, maxParseDiagnostics) {
		if ins.exhaustedLimit == "" {
			ins.exhaustedLimit = LimitParseDiagnostics
		}
		return
	}
	message := sanitizeDiagnosticMessage(class, position, extra...)
	ins.diagnostics = append(ins.diagnostics, providersessions.LineError{
		LineNumber: position,
		Message:    message,
	})
}

func (ins *inspection) limitError(limit string) error {
	if ins != nil {
		ins.recordLimitDiagnostic(limit)
	}
	return fmt.Errorf("%w: %s", providersessions.ErrResourceLimitExceeded, limit)
}

func (ins *inspection) canceled() bool {
	return ins != nil && errors.Is(ins.checkCanceled(), errInspectionCanceled)
}

func (ins *inspection) mergeStats(stats *SessionParseStats) {
	if ins == nil || stats == nil {
		return
	}
	stats.MalformedBlobCount += ins.malformedBlobs
	stats.MalformedMetaCount += ins.malformedMeta
	stats.UnavailableBlobCount += ins.unknownRecords
}

func (ins *inspection) parseErrors(fallback []providersessions.LineError) []providersessions.LineError {
	if ins == nil || len(ins.diagnostics) == 0 {
		return fallback
	}
	out := make([]providersessions.LineError, len(ins.diagnostics))
	copy(out, ins.diagnostics)
	return out
}

// Named inspection limits bound store traversal, SQLite reads, blob decoding,
// reconstruction work, and public diagnostics for one Cursor session inspection.
const (
	LimitStoreWalkEntries   = "store_walk_entries"
	LimitStoreCandidates    = "store_candidates"
	LimitQueriedRows        = "queried_rows"
	LimitBlobBytes          = "blob_bytes"
	LimitInspectionBytes    = "inspection_bytes"
	LimitProtobufNesting    = "protobuf_nesting"
	LimitProtobufDecodeWork = "protobuf_decode_work"
	LimitTranscriptFacts    = "transcript_facts"
	LimitParseDiagnostics   = "parse_diagnostics"
	LimitDiagnosticMessage  = "diagnostic_message_length"
)

const (
	maxStoreWalkEntries   = 100_000
	maxStoreCandidates    = 16
	maxQueriedRows        = 50_000
	maxBlobBytes          = 4 * 1024 * 1024
	maxInspectionBytes    = 64 * 1024 * 1024
	maxProtobufNesting    = 16
	maxProtobufDecodeWork = 512
	maxTranscriptFacts    = 10_000
	maxParseDiagnostics   = 64
	maxDiagnosticMessage  = 256
)

var testLimitOverrides struct {
	storeWalkEntries   int
	storeCandidates    int
	queriedRows        int
	blobBytes          int
	inspectionBytes    int
	protobufNesting    int
	protobufDecodeWork int
	transcriptFacts    int
	parseDiagnostics   int
}

func effectiveLimit(override, fallback int) int {
	if override > 0 {
		return override
	}
	return fallback
}

func sanitizeDiagnosticMessage(class string, position int, extra ...string) string {
	class = strings.TrimSpace(class)
	if class == "" {
		class = "cursor_record_error"
	}
	message := class
	if position > 0 {
		message = fmt.Sprintf("%s at row %d", class, position)
	}
	if len(extra) > 0 && strings.TrimSpace(extra[0]) != "" {
		message = fmt.Sprintf("%s (%s)", message, strings.TrimSpace(extra[0]))
	}
	return truncateDiagnosticMessage(message)
}

func truncateDiagnosticMessage(message string) string {
	if len(message) <= maxDiagnosticMessage {
		return message
	}
	if maxDiagnosticMessage <= 3 {
		return message[:maxDiagnosticMessage]
	}
	return message[:maxDiagnosticMessage-3] + "..."
}

func sanitizeStructuralError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "cursor session store could not be read"
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "select "),
		strings.Contains(lower, "pragma "),
		strings.Contains(lower, "sqlite"),
		strings.Contains(lower, "\\"),
		strings.Contains(lower, "/"),
		strings.Contains(lower, ":"):
		return "cursor session store could not be read"
	default:
		return truncateDiagnosticMessage(trimmed)
	}
}
