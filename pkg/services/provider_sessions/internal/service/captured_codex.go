package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"

	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// capturedCodex projects one selected-profile association. It never owns a
// provider filesystem or an execution handle.
type capturedCodex struct {
	reader recordings.WorkerCapturedActivityReader
}

const capturedPageLimit = 1000

var capturedCodexIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (s *capturedCodex) Details(ctx context.Context, ref providers.SessionRef) (providersessions.Detail, error) {
	if err := validateSessionRef(ref); err != nil {
		return providersessions.Detail{}, err
	}
	if ref.Provider != providers.IDCodex {
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
	if !capturedCodexIdentifier.MatchString(ref.ID) {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}
	selected, err := s.selectCapture(ctx, ref)
	if err != nil {
		return providersessions.Detail{}, err
	}
	page, err := s.readCapture(ctx, selected)
	if err != nil {
		return providersessions.Detail{}, err
	}
	// The catalog and the pinned activity must agree on the association. A
	// changed tuple must never disclose a different provider's transcript.
	if !captureHasRef(recordings.WorkerCapturedCatalogItem{Opening: page.Opening, MetadataRecords: capturedRecords(page)}, ref) {
		return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
	}
	return capturedCodexDetail(page, ref)
}

func (s *capturedCodex) selectCapture(ctx context.Context, ref providers.SessionRef) (recordings.WorkerSessionCatalogEntry, error) {
	request := recordings.WorkerCapturedCatalogRequest{Limit: capturedPageLimit}
	var selected recordings.WorkerSessionCatalogEntry
	generation := ""
	seen := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return selected, captureReadError(err)
		}
		page, err := s.reader.ListWorkerSessionCaptures(ctx, request)
		if err != nil {
			return selected, captureReadError(err)
		}
		if request.NextToken == "" {
			generation = page.GenerationID
		} else if page.GenerationID != generation {
			return selected, providersessions.ErrSessionStorageUnavailable
		}
		for _, item := range page.Items {
			if !captureHasRef(item, ref) {
				continue
			}
			selected, err = selectCodexAssociation(selected, item.Catalog)
			if err != nil {
				return selected, err
			}
		}
		if page.NextToken == "" {
			break
		}
		if len(page.Items) == 0 || seen[page.NextToken] {
			return selected, providersessions.ErrSessionStorageUnavailable
		}
		seen[page.NextToken] = true
		request.NextToken = page.NextToken
	}
	if selected.WorkerSessionID == "" {
		return selected, providersessions.ErrSessionNotFound
	}
	return selected, nil
}

func selectCodexAssociation(current, candidate recordings.WorkerSessionCatalogEntry) (recordings.WorkerSessionCatalogEntry, error) {
	if candidate.WorkerSessionID == "" {
		return current, providersessions.ErrSessionStorageUnavailable
	}
	if current.WorkerSessionID == "" || current == candidate {
		return candidate, nil
	}
	if current.WorkerSessionID != candidate.WorkerSessionID || current.RecordingID != candidate.RecordingID || current.RecordingGenerationID != candidate.RecordingGenerationID {
		return current, providersessions.ErrAmbiguousSessionFile
	}
	return current, providersessions.ErrSessionStorageUnavailable
}

func captureHasRef(item recordings.WorkerCapturedCatalogItem, ref providers.SessionRef) bool {
	// The latest committed association wins inside a single journal, including
	// unfinished captures. Health is checked separately rather than hiding a
	// known incomplete association behind NOT_FOUND.
	var latest *workers.SessionContinuation
	var position uint64
	for _, record := range append([]events.Record{item.Opening}, item.MetadataRecords...) {
		var draft workers.Draft
		var payload workers.SessionPayload
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindSession ||
			json.Unmarshal(draft.Payload, &payload) != nil || payload.Continuation == nil {
			continue
		}
		if uint64(record.ID.Position) >= position {
			latest, position = payload.Continuation, uint64(record.ID.Position)
		}
	}
	return latest != nil && latest.Provider == string(ref.Provider) && latest.Kind == ref.Kind && latest.ID == ref.ID
}

func capturedRecords(page recordings.WorkerCapturedActivityPage) []events.Record {
	records := make([]events.Record, len(page.Records))
	for i, record := range page.Records {
		records[i] = record.Record
	}
	return records
}

func (s *capturedCodex) readCapture(ctx context.Context, selected recordings.WorkerSessionCatalogEntry) (recordings.WorkerCapturedActivityPage, error) {
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: selected.WorkerSessionID, Limit: capturedPageLimit}
	var first recordings.WorkerCapturedActivityPage
	var records []recordings.WorkerCapturedRecord
	var previous uint64
	seen := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return first, captureReadError(err)
		}
		page, err := s.reader.ReadWorkerCapturedActivity(ctx, request)
		if err != nil {
			return first, captureReadError(err)
		}
		if !validCapturedCodexPage(page, selected) {
			return first, providersessions.ErrSessionStorageUnavailable
		}
		if request.NextToken == "" {
			first = page
		} else if !sameCapturedCodexHead(page, first) {
			return first, providersessions.ErrSessionStorageUnavailable
		}
		previous, err = validateCapturedCodexRecords(page, previous)
		if err != nil {
			return first, err
		}
		records = append(records, page.Records...)
		if page.NextToken == "" {
			break
		}
		if len(page.Records) == 0 || seen[page.NextToken] {
			return first, providersessions.ErrSessionStorageUnavailable
		}
		seen[page.NextToken] = true
		request.NextToken = page.NextToken
	}
	if previous != selected.CommittedPosition {
		return first, providersessions.ErrSessionStorageUnavailable
	}
	first.Records, first.NextToken = records, ""
	if !captureTerminalMatches(first) {
		return first, providersessions.ErrSessionStorageUnavailable
	}
	return first, nil
}

func validCapturedCodexPage(page recordings.WorkerCapturedActivityPage, selected recordings.WorkerSessionCatalogEntry) bool {
	return page.Catalog == selected && page.Health == recordings.WorkerRecordingStatusComplete && page.Terminal != nil &&
		page.Terminal.Position >= 1 && uint64(page.Terminal.Position) <= selected.CommittedPosition
}

func sameCapturedCodexHead(page, first recordings.WorkerCapturedActivityPage) bool {
	return reflect.DeepEqual(page.Opening, first.Opening) && *page.Terminal == *first.Terminal && reflect.DeepEqual(page.TokenUsage, first.TokenUsage)
}

func validateCapturedCodexRecords(page recordings.WorkerCapturedActivityPage, previous uint64) (uint64, error) {
	for _, record := range page.Records {
		position := uint64(record.Record.ID.Position)
		if position != previous+1 || position > page.Catalog.CommittedPosition || record.Truncated || record.Record.ID.Topic != page.Opening.ID.Topic {
			return previous, providersessions.ErrSessionStorageUnavailable
		}
		previous = position
	}
	return previous, nil
}

func captureTerminalMatches(page recordings.WorkerCapturedActivityPage) bool {
	if len(page.Records) == 0 || page.Opening.ID.Position != 1 || !reflect.DeepEqual(page.Records[0].Record, page.Opening) {
		return false
	}
	for _, record := range page.Records {
		if record.Record.ID.Position != page.Terminal.Position {
			continue
		}
		var draft workers.Draft
		var payload workers.SessionPayload
		return json.Unmarshal(record.Record.Payload, &draft) == nil && draft.Kind == workers.KindSession && draft.Phase == page.Terminal.Phase &&
			json.Unmarshal(draft.Payload, &payload) == nil && payload.Status == page.Terminal.Status &&
			(payload.WorkerSessionID == "" || payload.WorkerSessionID == page.Catalog.WorkerSessionID) &&
			(payload.FactorySessionID == "" || payload.FactorySessionID == page.Catalog.FactorySessionID)
	}
	return false
}

func captureReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return providersessions.ErrOperationCanceled
	}
	// Storage failures may contain private profile paths. A selected capture
	// disappearing is unavailable history, not a successful empty transcript.
	return providersessions.ErrSessionStorageUnavailable
}

func capturedCodexDetail(page recordings.WorkerCapturedActivityPage, ref providers.SessionRef) (providersessions.Detail, error) {
	entries, err := page.ProjectTranscript()
	if err != nil {
		return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
	}
	detail := providersessions.Detail{
		ProviderSession: providersessions.Ref{Provider: providersessions.ProviderCodex, Kind: ref.Kind, ID: ref.ID},
		Transcript:      make([]providersessions.TranscriptEntry, 0, len(entries)),
		Parse: providersessions.ParseSummary{
			EventCount:    len(page.Records),
			FunctionCalls: []providersessions.FunctionCallSummary{}, ParseErrors: []providersessions.LineError{},
			Reasoning: []providersessions.ReasoningSummary{}, Turns: []providersessions.TurnSummary{}, UnknownEvents: []providersessions.UnknownEvent{},
		},
	}
	for _, entry := range entries {
		mapped := providersessions.TranscriptEntry{Type: providersessions.TranscriptEntryType(entry.Type), Order: entry.Order,
			Text: entry.Text, Summary: entry.Summary, Name: entry.Name, CallID: entry.CallID, Arguments: entry.Arguments,
			Output: entry.Output, Status: entry.Status, Timestamp: entry.Timestamp, TurnIndex: entry.TurnIndex}
		detail.Transcript = append(detail.Transcript, mapped)
		if mapped.Type == providersessions.TranscriptToolCall || mapped.Type == providersessions.TranscriptToolOutput {
			detail.Parse.FunctionCalls = append(detail.Parse.FunctionCalls, providersessions.FunctionCallSummary{
				Type: entry.Type, Order: entry.Order, Name: entry.Name, CallID: entry.CallID, Arguments: entry.Arguments,
				Output: entry.Output, Status: entry.Status, TurnIndex: entry.TurnIndex})
		}
		if mapped.Type == providersessions.TranscriptReasoning {
			detail.Parse.Reasoning = append(detail.Parse.Reasoning, providersessions.ReasoningSummary{Order: entry.Order, Summary: entry.Summary, TurnIndex: entry.TurnIndex})
		}
	}
	if page.TokenUsage != nil {
		usage := page.TokenUsage
		values := []int64{usage.InputTokens, usage.CachedInputTokens, usage.OutputTokens, usage.ReasoningOutputTokens, usage.TotalTokens}
		converted := make([]int, len(values))
		for i, value := range values {
			if value < 0 || int64(int(value)) != value {
				return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
			}
			converted[i] = int(value)
		}
		detail.Parse.TokenUsage = &providersessions.TokenUsage{InputTokens: &converted[0], CachedInputTokens: &converted[1],
			OutputTokens: &converted[2], ReasoningOutputTokens: &converted[3], TotalTokens: &converted[4]}
	}
	return detail, nil
}
