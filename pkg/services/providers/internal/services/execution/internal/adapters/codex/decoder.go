package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type decoder struct {
	pending          []byte
	discardLine      bool
	flushed          bool
	sessionID        string
	finalContent     string
	usage            *usageRecord
	progress         []providers.ExecuteProgress
	declaredFailure  *providers.ExecuteFailure
	declaredKnown    bool
	decodeErr        error
	observeSession   providers.SessionObserver
	sourceBytes      int64
	lineCount        int64
	recordCount      int64
	progressCount    int
	diagnosticCount  int
	retainedText     int64
	limit            *streamLimit
	skippedRecord    *streamLimit
	recordSkips      int64
	transcriptFull   bool
	diagnosticsFull  bool
	retainedTextFull bool
}

type recordEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
	Usage    *usageRecord    `json:"usage"`
	Error    *errorRecord    `json:"error"`
	Message  string          `json:"message"`
}

type errorRecord struct {
	Type    string `json:"type"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type usageRecord struct {
	InputTokens           int64  `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	OutputTokens          int64  `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
}

type itemEnvelope struct {
	ID               string             `json:"id"`
	Type             string             `json:"type"`
	Text             string             `json:"text"`
	Message          string             `json:"message"`
	Command          string             `json:"command"`
	AggregatedOutput string             `json:"aggregated_output"`
	Status           string             `json:"status"`
	Changes          []fileChangeRecord `json:"changes"`
	Server           string             `json:"server"`
	Tool             string             `json:"tool"`
	Query            string             `json:"query"`
	Items            []planItem         `json:"items"`
}

type fileChangeRecord struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type planItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

func newDecoder(observeSession providers.SessionObserver) *decoder {
	return &decoder{observeSession: observeSession}
}

func (decoder *decoder) observe(chunk []byte) error {
	if decoder.flushed {
		return errors.New("codex stream received output after finalization")
	}
	if decoder.limit != nil || len(chunk) == 0 {
		return nil
	}
	remaining := maxCodexStreamBytes - decoder.sourceBytes
	if remaining <= 0 {
		decoder.markResourceLimitAtLine("bytes", maxCodexStreamBytes, maxCodexStreamBytes+1, decoder.lineCount+1)
		return nil
	}
	if int64(len(chunk)) > remaining {
		chunk = chunk[:int(remaining)]
		decoder.sourceBytes += remaining
		decoder.consume(chunk)
		if decoder.limit == nil {
			decoder.markResourceLimitAtLine("bytes", maxCodexStreamBytes, maxCodexStreamBytes+1, decoder.lineCount+1)
		}
		return nil
	}
	decoder.sourceBytes += int64(len(chunk))
	decoder.consume(chunk)
	return nil
}

func (decoder *decoder) consume(chunk []byte) {
	for len(chunk) > 0 {
		if decoder.limit != nil {
			return
		}
		if decoder.discardLine {
			newline := bytes.IndexByte(chunk, '\n')
			if newline < 0 {
				return
			}
			decoder.discardLine = false
			chunk = chunk[newline+1:]
			continue
		}
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if len(decoder.pending)+len(chunk) > maxRecordBytes {
				recordBytes := len(decoder.pending) + len(chunk)
				decoder.pending = nil
				decoder.discardLine = true
				decoder.skipOversizedRecord(int64(recordBytes))
				return
			}
			decoder.pending = append(decoder.pending, chunk...)
			return
		}
		if len(decoder.pending)+newline > maxRecordBytes {
			decoder.skipOversizedRecord(int64(len(decoder.pending) + newline))
		} else {
			decoder.pending = append(decoder.pending, chunk[:newline]...)
			if decoder.beginRecord() {
				decoder.decodeRecord(decoder.pending)
			}
		}
		decoder.pending = decoder.pending[:0]
		chunk = chunk[newline+1:]
	}
}

func (decoder *decoder) flush() error {
	if decoder.flushed {
		return errors.New("codex stream finalized more than once")
	}
	decoder.flushed = true
	if decoder.limit != nil {
		decoder.pending = nil
		return nil
	}
	if decoder.discardLine {
		decoder.pending = nil
		decoder.discardLine = false
		return nil
	}
	if len(bytes.TrimSpace(decoder.pending)) > 0 {
		if decoder.beginRecord() {
			decoder.decodeRecord(decoder.pending)
		}
	}
	decoder.pending = nil
	return nil
}

func (decoder *decoder) final() (string, *providers.SessionRef, error) {
	if !decoder.flushed {
		return "", nil, errors.New("codex stream was not finalized")
	}
	if decoder.finalContent == "" {
		return "", nil, errors.New("codex stream did not contain a completed agent message")
	}
	return decoder.finalContent, decoder.sessionRef(), nil
}

func (decoder *decoder) sessionRef() *providers.SessionRef {
	if decoder.sessionID == "" {
		return nil
	}
	return &providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       decoder.sessionID,
	}
}

func (decoder *decoder) progressFacts() []providers.ExecuteProgress {
	progress := make([]providers.ExecuteProgress, len(decoder.progress))
	for index := range decoder.progress {
		progress[index] = decoder.progress[index].Clone()
	}
	return progress
}

func (decoder *decoder) diagnostics() *providers.ExecuteDiagnostics {
	metadata := map[string]string{
		inspectionSourceBytesMetadata: inspectionMetadataValue(decoder.sourceBytes),
		inspectionLineCountMetadata:   inspectionMetadataValue(decoder.lineCount),
		inspectionRecordCountMetadata: inspectionMetadataValue(decoder.recordCount),
	}
	if decoder.usage != nil {
		metadata[usageInputTokensMetadata] = strconv.FormatInt(decoder.usage.InputTokens, 10)
		metadata[usageOutputTokensMetadata] = strconv.FormatInt(decoder.usage.OutputTokens, 10)
		if decoder.usage.CachedInputTokens != nil {
			metadata[usageCachedInputTokensMetadata] = strconv.FormatInt(*decoder.usage.CachedInputTokens, 10)
		}
		if decoder.usage.ReasoningOutputTokens != nil {
			metadata[usageReasoningOutputTokensMetadata] = strconv.FormatInt(*decoder.usage.ReasoningOutputTokens, 10)
		}
	}
	if decoder.transcriptFull {
		metadata[inspectionTranscriptTruncatedMetadata] = "true"
	}
	if decoder.diagnosticsFull {
		metadata[inspectionDiagnosticsTruncatedMetadata] = "true"
	}
	if decoder.retainedTextFull {
		metadata[inspectionRetainedTextTruncatedMetadata] = "true"
	}
	if limit := decoder.inspectionLimit(); limit != nil {
		metadata[inspectionLimitCategoryMetadata] = limit.category
		metadata[inspectionLimitConfiguredMetadata] = inspectionMetadataValue(limit.configured)
		metadata[inspectionLimitObservedMetadata] = inspectionMetadataValue(limit.observed)
		metadata[inspectionLimitLineMetadata] = inspectionMetadataValue(limit.line)
	}
	if decoder.recordSkips > 0 {
		metadata[inspectionRecordsSkippedMetadata] = inspectionMetadataValue(decoder.recordSkips)
	}
	return &providers.ExecuteDiagnostics{
		Progress: decoder.progressFacts(),
		Metadata: metadata,
	}
}

func (decoder *decoder) resourceFailure() *providers.ExecuteFailure {
	if decoder.limit == nil {
		return nil
	}
	return decoder.limit.failure(decoder.sessionRef(), decoder.diagnostics())
}

// inspectionLimit reports the fact that bounds stream inspection: a hard
// stream limit when one tripped, otherwise the first skipped oversized record.
func (decoder *decoder) inspectionLimit() *streamLimit {
	if decoder.limit != nil {
		return decoder.limit
	}
	return decoder.skippedRecord
}

// skippedRecordFailure classifies an execution whose final agent decision was
// unrecoverable after one or more oversized records were skipped. It keeps the
// pre-existing record-limit dependency classification for that terminal case.
func (decoder *decoder) skippedRecordFailure() *providers.ExecuteFailure {
	if decoder.skippedRecord == nil {
		return nil
	}
	return decoder.skippedRecord.failure(decoder.sessionRef(), decoder.diagnostics())
}

func (decoder *decoder) beginRecord() bool {
	decoder.lineCount++
	decoder.recordCount++
	switch {
	case decoder.lineCount > maxCodexStreamLines:
		decoder.markResourceLimit("lines", maxCodexStreamLines, decoder.lineCount)
	case decoder.recordCount > maxCodexStreamRecords:
		decoder.markResourceLimit("records", maxCodexStreamRecords, decoder.recordCount)
	default:
		return true
	}
	return false
}

func (decoder *decoder) decodeRecord(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var record recordEnvelope
	if json.Unmarshal(raw, &record) != nil {
		decoder.markDecodeFailure("malformed_json")
		return
	}
	switch record.Type {
	case "thread.started":
		if !decoder.observeSessionID(record.ThreadID) {
			decoder.markDecodeFailure("malformed_thread")
			return
		}
		decoder.addProgress("session.started", "started", nil)
	case "turn.started":
		decoder.addProgress("turn.started", "started", nil)
	case "turn.completed":
		if record.Usage == nil {
			decoder.markDecodeFailure("malformed_usage")
			return
		}
		usage := *record.Usage
		decoder.usage = &usage
		detail, _ := json.Marshal(record.Usage)
		decoder.addProgress("usage.updated", string(detail), nil)
		decoder.addProgress("turn.completed", "completed", nil)
	case "turn.failed":
		if record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
			decoder.markDecodeFailure("malformed_turn_failure")
			return
		}
		decoder.declareFailure(*record.Error)
	case "error":
		if strings.TrimSpace(record.Message) == "" {
			decoder.markDecodeFailure("malformed_error")
			return
		}
		decoder.declareFailure(errorRecord{Message: record.Message})
	case "item.started", "item.updated", "item.completed":
		decoder.decodeItem(record.Type, record.Item)
	default:
		decoder.addDiagnostic("unsupported_event")
	}
}

func (decoder *decoder) observeSessionID(raw string) bool {
	sessionID := strings.TrimSpace(raw)
	if sessionID == "" {
		return false
	}
	if decoder.sessionID != "" {
		return true
	}
	decoder.sessionID = sessionID
	if decoder.observeSession != nil {
		decoder.observeSession(providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		})
	}
	return true
}

func (decoder *decoder) decodeItem(nativeType string, raw json.RawMessage) {
	var item itemEnvelope
	if json.Unmarshal(raw, &item) != nil || strings.TrimSpace(item.ID) == "" {
		decoder.markDecodeFailure("malformed_item")
		return
	}
	item.ID = boundedDetail(item.ID)
	phase := itemPhase(nativeType, item.Status)
	metadata := map[string]string{
		"item_id":      item.ID,
		"native_event": nativeType,
	}
	switch item.Type {
	case "agent_message":
		phase = messagePhase(phase)
		detail := decoder.retainedDetail(item.Text)
		if detail == "" {
			decoder.markDecodeFailure("malformed_message")
			return
		}
		decoder.addProgress("message."+phase, detail, metadata)
		if nativeType == "item.completed" {
			final, truncated := decoder.retainFinalContent(item.Text)
			if truncated {
				decoder.markResourceLimit("retained_output", maxFinalContentBytes, int64(len(item.Text)))
				return
			}
			decoder.finalContent = final
		}
	case "reasoning":
		decoder.addProgress(
			"reasoning."+messagePhase(phase),
			decoder.retainedDetail(item.Text),
			metadata,
		)
	case "todo_list":
		decoder.addProgress("plan."+phase, decoder.retainedDetail(planDetail(item.Items)), metadata)
	case "file_change":
		decoder.addProgress("file_change."+phase, decoder.retainedDetail(fileChangeDetail(item.Changes)), metadata)
	case "command_execution":
		decoder.addProgress("tool."+phase, decoder.retainedDetail(
			firstNonEmpty(item.AggregatedOutput, item.Command),
		), toolMetadata(metadata, item.ID, "command_execution"))
	case "mcp_tool_call", "collab_tool_call":
		name := strings.Trim(strings.TrimSpace(item.Server)+"/"+strings.TrimSpace(item.Tool), "/")
		decoder.addProgress("tool."+phase, decoder.retainedDetail(item.Message),
			toolMetadata(metadata, item.ID, firstNonEmpty(name, item.Type)))
	case "web_search":
		decoder.addProgress("tool."+phase, decoder.retainedDetail(item.Query),
			toolMetadata(metadata, item.ID, "web_search"))
	default:
		decoder.addDiagnostic("unsupported_item")
	}
}

func (decoder *decoder) addProgress(
	phase string,
	detail string,
	metadata map[string]string,
) {
	if decoder.progressCount >= maxCodexTranscriptFacts {
		if !decoder.transcriptFull {
			decoder.transcriptFull = true
			decoder.addDiagnostic("transcript_limit")
		}
		return
	}
	decoder.progress = append(decoder.progress, providers.ExecuteProgress{
		Phase:    phase,
		Detail:   detail,
		Metadata: cloneMetadata(metadata),
	})
	decoder.progressCount++
}

func (decoder *decoder) addDiagnostic(code string) {
	decoder.addDiagnosticEntry("Codex stream record was omitted", map[string]string{
		"code": boundedDetail(code),
	})
}

func (decoder *decoder) addDiagnosticEntry(detail string, metadata map[string]string) {
	if decoder.diagnosticCount >= maxCodexDiagnosticFacts {
		decoder.diagnosticsFull = true
		return
	}
	decoder.diagnosticCount++
	if decoder.progressCount >= maxCodexTranscriptFacts {
		decoder.transcriptFull = true
		return
	}
	decoder.progress = append(decoder.progress, providers.ExecuteProgress{
		Phase:    "diagnostic",
		Detail:   detail,
		Metadata: metadata,
	})
	decoder.progressCount++
}

// skipOversizedRecord records that one over-limit JSONL record was discarded
// without buffering it, then lets decoding continue with the next record. An
// oversized mid-stream record (for example one tool result carrying a huge
// aggregated command output) must not fail an otherwise-clean execution: the
// stream's later final agent message remains authoritative. Nothing over
// maxRecordBytes is ever retained, so the memory-safety bound is unchanged.
func (decoder *decoder) skipOversizedRecord(observed int64) {
	decoder.lineCount++
	decoder.recordSkips++
	if decoder.skippedRecord == nil {
		decoder.skippedRecord = &streamLimit{
			category:   "record",
			configured: maxRecordBytes,
			observed:   observed,
			line:       decoder.lineCount,
		}
	}
	decoder.addDiagnosticEntry(
		"Codex stream record exceeded the record limit and was skipped",
		map[string]string{
			"code":         "record_skipped",
			"record_bytes": inspectionMetadataValue(observed),
			"line":         inspectionMetadataValue(decoder.lineCount),
		},
	)
}

func (decoder *decoder) markDecodeFailure(code string) {
	decoder.addDiagnostic(code)
	if decoder.decodeErr == nil {
		decoder.decodeErr = errors.New("Codex stream could not be decoded safely")
	}
}

func (decoder *decoder) markResourceLimit(category string, configured, observed int64) {
	decoder.markResourceLimitAtLine(category, configured, observed, decoder.lineCount)
}

func (decoder *decoder) markResourceLimitAtLine(category string, configured, observed, line int64) {
	if decoder.limit != nil {
		return
	}
	if line == 0 {
		line = 1
	}
	decoder.limit = &streamLimit{
		category:   category,
		configured: configured,
		observed:   observed,
		line:       line,
	}
	decoder.addDiagnostic("inspection_limit")
	decoder.decodeErr = errors.New("Codex stream inspection reached a configured safety limit")
}

func (decoder *decoder) retainedDetail(value string) string {
	value = boundedDetail(value)
	if value == "" {
		return ""
	}
	remaining := maxCodexRetainedText - decoder.retainedText
	if remaining <= 0 {
		decoder.retainedTextFull = true
		return ""
	}
	if int64(len(value)) > remaining {
		value = boundedDetail(value[:int(remaining)])
		decoder.retainedTextFull = true
	}
	decoder.retainedText += int64(len(value))
	return value
}

func (decoder *decoder) retainFinalContent(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if len(value) > maxFinalContentBytes {
		return boundedDetail(value[:maxFinalContentBytes]), true
	}
	return value, false
}

func (decoder *decoder) declareFailure(record errorRecord) {
	failure := classifyDeclaredFailure(record)
	markUnrecognizedProviderRefusal(&failure)
	known := failure.Kind != providers.ExecuteFailureKindUnknown
	if decoder.declaredFailure == nil || known || !decoder.declaredKnown {
		decoder.declaredFailure = &failure
	}
	decoder.declaredKnown = decoder.declaredKnown || known
}

func classifyDeclaredFailure(record errorRecord) providers.ExecuteFailure {
	message := strings.ToLower(strings.TrimSpace(record.Message))
	nativeType := strings.ToLower(strings.TrimSpace(record.Type))
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case nativeType == "authentication_error",
		nativeType == "permission_error",
		record.Status == 401, record.Status == 403,
		strings.HasPrefix(message, "unexpected status 401"),
		strings.HasPrefix(message, "unexpected status 403"):
		kind = providers.ExecuteFailureKindAuthentication
	case nativeType == "invalid_request_error",
		record.Status == 400,
		strings.HasPrefix(message, "unexpected status 400"):
		kind = providers.ExecuteFailureKindInvalidRequest
	case nativeType == "rate_limit_error",
		nativeType == "overloaded_error",
		record.Status == 429,
		strings.HasPrefix(message, "unexpected status 429"),
		strings.HasPrefix(message, "you've hit your usage limit"),
		strings.HasPrefix(message, "selected model is at capacity"):
		kind = providers.ExecuteFailureKindThrottled
	case record.Status == 408,
		strings.HasPrefix(message, "context deadline exceeded"),
		strings.HasPrefix(message, "command timed out"),
		strings.HasPrefix(message, "request timed out"),
		strings.HasPrefix(message, "provider timeout"):
		kind = providers.ExecuteFailureKindTimeout
	case nativeType == "api_error",
		nativeType == "server_error",
		record.Status >= 500 && record.Status <= 599:
		kind = providers.ExecuteFailureKindDependency
	}
	return providers.ExecuteFailure{
		Kind:    kind,
		Message: declaredFailureMessage(kind),
	}
}

func declaredFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return "Codex authentication failed."
	case providers.ExecuteFailureKindInvalidRequest:
		return "Codex rejected the request as invalid"
	case providers.ExecuteFailureKindThrottled:
		return "Codex is temporarily unavailable due to usage or capacity limits"
	case providers.ExecuteFailureKindTimeout:
		return "Codex request timed out."
	case providers.ExecuteFailureKindDependency:
		return "Codex encountered a temporary server error"
	case providers.ExecuteFailureKindSessionNotFound:
		return "Codex does not recognize the referenced Provider Session as live"
	default:
		return "Codex reported a terminal error"
	}
}

func itemPhase(nativeType string, status string) string {
	switch nativeType {
	case "item.started":
		return "started"
	case "item.updated":
		return "updated"
	}
	switch strings.TrimSpace(status) {
	case "failed":
		return "failed"
	case "declined":
		return "canceled"
	default:
		return "completed"
	}
}

func messagePhase(phase string) string {
	if phase == "updated" {
		return "delta"
	}
	return phase
}

func toolMetadata(base map[string]string, id string, name string) map[string]string {
	metadata := cloneMetadata(base)
	metadata["correlation_id"] = id
	metadata["tool_name"] = boundedDetail(name)
	return metadata
}

func planDetail(items []planItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		status := "pending"
		if item.Completed {
			status = "completed"
		}
		parts = append(parts, status+":"+strings.TrimSpace(item.Text))
	}
	return boundedDetail(strings.Join(parts, "; "))
}

func fileChangeDetail(changes []fileChangeRecord) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Path != "" || change.Kind != "" {
			parts = append(parts, strings.TrimSpace(change.Kind)+" "+strings.TrimSpace(change.Path))
		}
	}
	return boundedDetail(strings.Join(parts, "; "))
}

func boundedDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxDetailBytes {
		return value
	}
	return strings.TrimSpace(value[:maxDetailBytes])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
