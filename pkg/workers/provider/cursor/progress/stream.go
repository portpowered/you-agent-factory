package progress

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	factoryresponseevents "github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

const progressFragmentKind = "PROGRESS_FRAGMENT"

// ProgressFragment is the Cursor-owned progress-stream payload published at the
// provider boundary before shared orchestration maps it into session fragments.
type ProgressFragment struct {
	DispatchID        string
	Kind              string
	Payload           string
	ExternalEventType string
	CanonicalDraft    factoryresponseevents.Draft
	HasCanonicalDraft bool
}

// ProgressPublisher receives Cursor-owned progress fragments for one invocation.
type ProgressPublisher func(fragment ProgressFragment)

// IsCommand reports whether command names the Cursor CLI executable.
func IsCommand(command string) bool {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/")))
	return base == string(modelprovider.Cursor) ||
		base == string(modelprovider.Cursor)+".exe" ||
		base == string(modelprovider.Cursor)+".cmd"
}

// ResponseEventStream observes Cursor subprocess stdout/stderr and publishes
// provider-neutral structured response drafts and bounded diagnostics.
type ResponseEventStream struct {
	dispatchID string
	decoder    *cursorpkg.ResponseEventDecoder
	publisher  ProgressPublisher
	logger     logging.Logger
}

// NewResponseEventStream constructs a Cursor-owned stdout/stderr observer for one invocation.
func NewResponseEventStream(
	dispatchID string,
	publisher ProgressPublisher,
	logger logging.Logger,
) *ResponseEventStream {
	dispatchID = strings.TrimSpace(dispatchID)
	return &ResponseEventStream{
		dispatchID: dispatchID,
		decoder: cursorpkg.NewResponseEventDecoder(adapter.DecoderContext{
			DispatchID: dispatchID,
		}),
		publisher: publisher,
		logger:    logging.EnsureLogger(logger),
	}
}

// Observe accepts one subprocess output chunk for the given stream name.
func (s *ResponseEventStream) Observe(ctx context.Context, stream string, chunk []byte) {
	decoded, err := s.decoder.Observe(ctx, adapter.Observation{
		Stream: adapter.OutputStream(stream),
		Chunk:  append([]byte(nil), chunk...),
	})
	s.publish(decoded)
	if err != nil {
		s.logger.Error("cursor response-event decoder observation failed", "error", err)
	}
}

// Flush processes trailing decode state and closes unresolved tool lifecycle records.
func (s *ResponseEventStream) Flush(ctx context.Context, reason adapter.FlushReason) {
	decoded, err := s.decoder.Flush(ctx, adapter.FlushContext{Reason: reason})
	s.publish(decoded)
	if err != nil {
		s.logger.Error("cursor response-event decoder flush failed", "error", err)
	}
}

// FlushReason maps subprocess completion context to a decoder flush reason.
func FlushReason(ctx context.Context, exitCode int, err error) adapter.FlushReason {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return adapter.FlushReasonCanceled
	}
	if err != nil || exitCode != 0 {
		return adapter.FlushReasonTerminated
	}
	return adapter.FlushReasonCompleted
}

func (s *ResponseEventStream) publish(decoded adapter.DecodeResult) {
	for _, draft := range decoded.Drafts {
		cloned := draft
		cloned.Payload = append(json.RawMessage(nil), draft.Payload...)
		s.publisher(ProgressFragment{
			DispatchID:        s.dispatchID,
			HasCanonicalDraft: true,
			CanonicalDraft:    cloned,
		})
	}
	for _, diagnostic := range decoded.Diagnostics {
		s.publisher(ProgressFragment{
			DispatchID:        s.dispatchID,
			Kind:              progressFragmentKind,
			Payload:           diagnostic.Message,
			ExternalEventType: diagnostic.Code,
		})
	}
}
