package conductor

import (
	"context"
	"errors"
	"sync"

	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// structuredResponseWriter is the conductor-owned response writer/closer. It
// stamps conductor correlation onto every draft, preserves emission order to
// the next writer, stops immediately on write failure, and rejects late
// writes or closes after the writer is closed. Leaf Workers Draft validation
// and lifecycle rules remain composed through inference.ExecuteInvocation.
type structuredResponseWriter struct {
	mu           sync.Mutex
	invocationID string
	next         inference.ResponseWriter
	closed       bool
	terminalErr  error
}

func newStructuredResponseWriter(invocationID string, next inference.ResponseWriter) *structuredResponseWriter {
	return &structuredResponseWriter{invocationID: invocationID, next: next}
}

func (w *structuredResponseWriter) WriteEvent(ctx context.Context, event inference.EventDraft) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		err := newWriterClosedError("response event cannot be written after the conductor response writer is closed")
		w.terminalErr = errors.Join(w.terminalErr, err)
		return err
	}
	stamped, err := stampCorrelation(w.invocationID, event)
	if err != nil {
		w.closed = true
		w.terminalErr = err
		return err
	}
	if err := w.next.WriteEvent(ctx, stamped); err != nil {
		w.closed = true
		w.terminalErr = err
		return err
	}
	return nil
}

func (w *structuredResponseWriter) Close(ctx context.Context, completion inference.Completion) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		err := newWriterClosedError("response completion may be closed exactly once")
		w.terminalErr = errors.Join(w.terminalErr, err)
		return err
	}
	w.closed = true
	if err := w.next.Close(ctx, sanitizeCompletion(completion)); err != nil {
		w.terminalErr = err
		return err
	}
	return nil
}

type correlatingIntegration struct {
	inference.Integration
}

func (i correlatingIntegration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	return i.Integration.Invoke(ctx, request, newStructuredResponseWriter(request.InvocationID(), writer))
}

func stampCorrelation(invocationID string, event inference.EventDraft) (inference.EventDraft, error) {
	draft := event.Draft()
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:              invocationID,
		Kind:               draft.Kind,
		Phase:              draft.Phase,
		Provenance:         draft.Provenance,
		Payload:            draft.Payload,
		TurnID:             draft.TurnID,
		ItemID:             draft.ItemID,
		ParentItemID:       draft.ParentItemID,
		ProviderSessionRef: draft.ProviderSessionRef,
	})
}

type writerClosedError struct {
	message string
}

func (e *writerClosedError) Error() string { return e.message }

func newWriterClosedError(message string) error {
	return &writerClosedError{message: message}
}
