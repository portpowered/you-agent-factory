package cursor

import (
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const cursorDiagnosticInvalidResult = "cursor_invalid_result"

func (d *ResponseEventDecoder) decodeResult(record cursorStreamRecord) (adapter.DecodeResult, error) {
	providerRef := d.providerRef(record.SessionID)
	if record.Subtype == ResultSubtypeSuccess && !record.IsError {
		if canonicalProviderSession(string(modelprovider.Cursor), record.SessionID) == nil {
			return cursorDiagnostic(cursorDiagnosticInvalidResult, "Cursor success result omitted a valid session identifier"), nil
		}
		closed, err := d.closeUnresolvedTools(cursorToolCloseOutcome{reason: cursorToolGapTerminal})
		if err != nil {
			return closed, err
		}
		snapshot, snapshotErr := d.cursorResultSnapshot(record, providerRef)
		return appendCursorDecodeResult(closed, snapshot), snapshotErr
	}

	if cursorResultCanceled(record.Subtype) {
		closed, closeErr := d.closeUnresolvedTools(cursorToolCloseOutcome{
			canceled: true, nativeType: ResultTypeResult, nativeSubtype: record.Subtype,
		})
		if closeErr != nil {
			return closed, closeErr
		}
		payload, err := json.Marshal(responseevents.RunPayload{Status: "canceled"})
		if err != nil {
			return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor canceled result payload: %w", err)
		}
		canceled := adapter.DecodeResult{Drafts: []responseevents.Draft{{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID,
			Kind: responseevents.KindRun, Phase: responseevents.PhaseCanceled,
			ProviderSessionRef: providerRef,
			Provenance:         cursorResponseProvenance(ResultTypeResult, record.Subtype, responseevents.RepresentationNotification, responseevents.FidelityNormalized),
			Payload:            payload,
		}}}
		return appendCursorDecodeResult(closed, canceled), nil
	}
	closed, closeErr := d.closeUnresolvedTools(cursorToolCloseOutcome{reason: cursorToolGapFailure})
	if closeErr != nil {
		return closed, closeErr
	}

	failure := failureResultFromPayload(resultPayload{
		Type: ResultTypeResult, Subtype: record.Subtype, IsError: record.IsError,
		Result: record.Result, SessionID: record.SessionID,
	})
	payload, err := json.Marshal(responseevents.ErrorPayload{
		Code: string(failure.Reason), Message: failure.Message,
		Retryable: cursorFailureRetryable(failure.Reason),
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor failed result payload: %w", err)
	}
	failed := adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindError, Phase: responseevents.PhaseFailed,
		ProviderSessionRef: providerRef,
		Provenance:         cursorResponseProvenance(ResultTypeResult, record.Subtype, responseevents.RepresentationNotification, responseevents.FidelityNormalized),
		Payload:            payload,
	}}}
	return appendCursorDecodeResult(closed, failed), nil
}

func (d *ResponseEventDecoder) cursorResultSnapshot(record cursorStreamRecord, providerRef string) (adapter.DecodeResult, error) {
	result := boundedText(record.Result, PublishedTextLimit)
	fidelity := responseevents.FidelityLossless
	if result != record.Result {
		fidelity = responseevents.FidelityLossy
	}
	payload, err := json.Marshal(responseevents.MessagePayload{
		Role:          "assistant",
		ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: result}},
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor terminal result payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
		ItemID: d.messageItemID(), ProviderSessionRef: providerRef,
		Provenance: cursorResponseProvenance(ResultTypeResult, record.Subtype, responseevents.RepresentationSnapshot, fidelity),
		Payload:    payload,
	}}}, nil
}

func cursorResultCanceled(subtype string) bool {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "cancel", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func cursorFailureRetryable(reason workerexecution.WorkFailureType) bool {
	switch reason {
	case workerexecution.WorkFailureTypeThrottled, workerexecution.WorkFailureTypeTimeout, workerexecution.WorkFailureTypeInternalServerError:
		return true
	default:
		return false
	}
}
