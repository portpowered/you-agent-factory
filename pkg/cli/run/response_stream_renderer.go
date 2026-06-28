package run

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
)

const (
	responseStreamProgressPrefix        = "[you:progress] "
	responseStreamPrimaryResultHeader   = "--- primary result ---"
	responseStreamInvocationOutcomeHeader = "--- invocation outcome ---"
	maxHumanProgressLineBytes           = 1024

	responseStreamJSONRecordProgress      = "progress"
	responseStreamJSONRecordStreamGap     = "stream_gap"
	responseStreamJSONRecordCompaction    = "compaction"
	responseStreamJSONRecordPrimaryResult = "primary_result"
)

// responseStreamRenderer consumes internal SessionResponseStream segments and
// writes ordered progress output followed by the final invocation result.
type responseStreamRenderer interface {
	responseStreamEventSink
	renderedProgress() bool
	writeFinalInvocationResult(result apisurface.FactoryInvocationResult) error
}

func newResponseStreamRenderer(output io.Writer, jsonMode bool) responseStreamRenderer {
	if jsonMode {
		return newJSONResponseStreamRenderer(output)
	}
	return newHumanResponseStreamRenderer(output)
}

// humanResponseStreamRenderer prints ordered internal SessionResponseStream
// progress to stdout and keeps the final invocation primary result visually
// separate from transient progress output.
type humanResponseStreamRenderer struct {
	mu             sync.Mutex
	output         io.Writer
	lastSequence   map[string]int64
	progressLines  int
	progressSeen   bool
}

func newHumanResponseStreamRenderer(output io.Writer) *humanResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &humanResponseStreamRenderer{
		output:       output,
		lastSequence: make(map[string]int64),
	}
}

func (r *humanResponseStreamRenderer) onStreamSegment(result factorysessions.SessionResponseStreamReadResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if result.BehindRetainedWindow {
		r.writeProgressLineLocked("earlier progress unavailable (stream resumed behind retained window)")
	}
	if result.Compaction != nil {
		r.writeProgressLineLocked(formatCompactionNotice(*result.Compaction))
	}
	for _, event := range result.Events {
		r.renderEventLocked(event)
	}
}

func (r *humanResponseStreamRenderer) renderedProgress() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.progressSeen
}

func (r *humanResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	if result.Status == factoryapi.InvocationTerminalStatusCompleted {
		text, err := invocationPrimaryResultText(result.PrimaryResult)
		if err != nil {
			return err
		}
		return r.writePrimaryResult(text)
	}
	return r.writeInvocationOutcome(result)
}

func (r *humanResponseStreamRenderer) writeInvocationOutcome(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	lines := formatHumanInvocationOutcomeLines(result)
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.progressSeen {
		if _, err := fmt.Fprintln(r.output); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(r.output, responseStreamInvocationOutcomeHeader); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(r.output, line); err != nil {
			return err
		}
	}
	return nil
}

func formatHumanInvocationOutcomeLines(result apisurface.FactoryInvocationResult) []string {
	lines := []string{
		"status: " + string(result.Status),
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	return lines
}

func (r *humanResponseStreamRenderer) writePrimaryResult(text string) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.progressSeen {
		if _, err := fmt.Fprintln(r.output); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(r.output, responseStreamPrimaryResultHeader); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(r.output, text)
	return err
}

func (r *humanResponseStreamRenderer) renderEventLocked(event responsestream.Event) {
	dispatchKey := strings.TrimSpace(event.DispatchID)
	if dispatchKey == "" {
		dispatchKey = "_"
	}
	if event.Sequence > 0 && event.Sequence <= r.lastSequence[dispatchKey] {
		return
	}
	if event.Sequence > 0 {
		r.lastSequence[dispatchKey] = event.Sequence
	}

	switch event.Kind {
	case responsestream.EventKindCompactionSignal:
		if event.Compaction != nil {
			r.writeProgressLineLocked(formatCompactionNotice(*event.Compaction))
		}
		return
	case responsestream.EventKindStreamCompleted, responsestream.EventKindStreamFailed:
		return
	case responsestream.EventKindResponseFragment:
		return
	case responsestream.EventKindProgressFragment:
		if !humanProgressRenderableType(event.Type) {
			return
		}
		payload := boundedHumanProgressPayload(event.Payload)
		if payload == "" {
			return
		}
		r.writeProgressLineLocked(payload)
	}
}

func humanProgressRenderableType(eventType responsestream.EventType) bool {
	switch eventType {
	case responsestream.EventTypeStarted,
		responsestream.EventTypeProgress,
		responsestream.EventTypeFailed,
		responsestream.EventTypeCanceled,
		responsestream.EventTypeUnknown:
		return true
	default:
		return false
	}
}

func boundedHumanProgressPayload(payload string) string {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return ""
	}
	if maxHumanProgressLineBytes <= 0 || len([]byte(trimmed)) <= maxHumanProgressLineBytes {
		return trimmed
	}
	bytes := []byte(trimmed)
	return strings.TrimSpace(string(bytes[:maxHumanProgressLineBytes])) + "..."
}

func formatCompactionNotice(summary responsestream.CompactionSummary) string {
	reason := strings.TrimSpace(string(summary.Reason))
	if reason == "" {
		reason = "compacted"
	}
	if summary.DroppedSequenceCount > 0 {
		return fmt.Sprintf(
			"stream %s (%d earlier events omitted)",
			strings.ToLower(reason),
			summary.DroppedSequenceCount,
		)
	}
	return fmt.Sprintf("stream %s", strings.ToLower(reason))
}

func (r *humanResponseStreamRenderer) writeProgressLineLocked(payload string) {
	if strings.TrimSpace(payload) == "" {
		return
	}
	r.progressSeen = true
	r.progressLines++
	_, _ = fmt.Fprintf(r.output, "%s%s\n", responseStreamProgressPrefix, payload)
}

// jsonResponseStreamRenderer emits newline-delimited JSON records for internal
// SessionResponseStream progress and the final invocation result.
type jsonResponseStreamRenderer struct {
	mu           sync.Mutex
	output       io.Writer
	lastSequence map[string]int64
	progressSeen bool
}

func newJSONResponseStreamRenderer(output io.Writer) *jsonResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &jsonResponseStreamRenderer{
		output:       output,
		lastSequence: make(map[string]int64),
	}
}

func (r *jsonResponseStreamRenderer) onStreamSegment(result factorysessions.SessionResponseStreamReadResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if result.BehindRetainedWindow {
		r.writeRecordLocked(responseStreamJSONStreamGapRecord{
			RecordType: responseStreamJSONRecordStreamGap,
			Reason:     "behind_retained_window",
		})
	}
	if result.Compaction != nil {
		r.writeCompactionLocked(*result.Compaction)
	}
	for _, event := range result.Events {
		r.renderEventLocked(event)
	}
}

func (r *jsonResponseStreamRenderer) renderedProgress() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.progressSeen
}

func (r *jsonResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	return r.writeRecord(responseStreamJSONPrimaryResultRecord{
		RecordType:  responseStreamJSONRecordPrimaryResult,
		Invocation:  apisurface.InvocationResponseFromResult(result),
	})
}

func (r *jsonResponseStreamRenderer) renderEventLocked(event responsestream.Event) {
	switch event.Kind {
	case responsestream.EventKindCompactionSignal:
		if event.Compaction != nil {
			r.writeCompactionLocked(*event.Compaction)
		}
		return
	case responsestream.EventKindStreamCompleted, responsestream.EventKindStreamFailed:
		return
	case responsestream.EventKindResponseFragment:
		return
	case responsestream.EventKindProgressFragment:
		if !humanProgressRenderableType(event.Type) {
			return
		}
		if strings.TrimSpace(event.Payload) == "" {
			return
		}
		dispatchKey := strings.TrimSpace(event.DispatchID)
		if dispatchKey == "" {
			dispatchKey = "_"
		}
		if event.Sequence > 0 && event.Sequence <= r.lastSequence[dispatchKey] {
			return
		}
		if event.Sequence > 0 {
			r.lastSequence[dispatchKey] = event.Sequence
		}
		record := responseStreamJSONProgressRecord{
			RecordType: responseStreamJSONRecordProgress,
			Sequence:   event.Sequence,
			Kind:       string(event.Kind),
			EventType:  string(event.Type),
			Payload:    event.Payload,
		}
		if dispatchID := strings.TrimSpace(event.DispatchID); dispatchID != "" {
			record.DispatchID = &dispatchID
		}
		r.writeRecordLocked(record)
	}
}

func (r *jsonResponseStreamRenderer) writeCompactionLocked(summary responsestream.CompactionSummary) {
	r.writeRecordLocked(responseStreamJSONCompactionRecord{
		RecordType:            responseStreamJSONRecordCompaction,
		Reason:                string(summary.Reason),
		DroppedSequenceCount:  summary.DroppedSequenceCount,
		FirstRetainedSequence: summary.FirstRetainedSequence,
		LastDroppedSequence:   summary.LastDroppedSequence,
	})
}

func (r *jsonResponseStreamRenderer) writeRecord(record any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeRecordLocked(record)
}

func (r *jsonResponseStreamRenderer) writeRecordLocked(record any) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal response-stream JSON record: %w", err)
	}
	if r.progressSeen || recordTypeOf(record) != responseStreamJSONRecordPrimaryResult {
		r.progressSeen = true
	}
	_, err = fmt.Fprintln(r.output, string(encoded))
	return err
}

func recordTypeOf(record any) string {
	switch typed := record.(type) {
	case responseStreamJSONProgressRecord:
		return typed.RecordType
	case responseStreamJSONStreamGapRecord:
		return typed.RecordType
	case responseStreamJSONCompactionRecord:
		return typed.RecordType
	case responseStreamJSONPrimaryResultRecord:
		return typed.RecordType
	default:
		return ""
	}
}

type responseStreamJSONProgressRecord struct {
	RecordType string  `json:"recordType"`
	Sequence   int64   `json:"sequence,omitempty"`
	DispatchID *string `json:"dispatchId,omitempty"`
	Kind       string  `json:"kind"`
	EventType  string  `json:"eventType"`
	Payload    string  `json:"payload"`
}

type responseStreamJSONStreamGapRecord struct {
	RecordType string `json:"recordType"`
	Reason     string `json:"reason"`
}

type responseStreamJSONCompactionRecord struct {
	RecordType            string `json:"recordType"`
	Reason                string `json:"reason"`
	DroppedSequenceCount  int    `json:"droppedSequenceCount,omitempty"`
	FirstRetainedSequence int64  `json:"firstRetainedSequence,omitempty"`
	LastDroppedSequence   int64  `json:"lastDroppedSequence,omitempty"`
}

type responseStreamJSONPrimaryResultRecord struct {
	RecordType string                         `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}
