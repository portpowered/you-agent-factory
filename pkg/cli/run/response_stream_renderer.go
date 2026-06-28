package run

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
)

const (
	responseStreamProgressPrefix      = "[you:progress] "
	responseStreamPrimaryResultHeader = "--- primary result ---"
	maxHumanProgressLineBytes         = 1024
)

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
