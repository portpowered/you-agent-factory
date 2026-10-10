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

// capturedProvider projects one selected-profile association. It never owns a
// provider filesystem or an execution handle.
type capturedProvider struct {
	reader recordings.WorkerCapturedActivityReader
}

const capturedPageLimit = 1000

var capturedProviderIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Inspect uses only activation-prepared metadata. Reusing an exact reference
// is unambiguous only when every association forms one complete scoped chain.
func (s *capturedProvider) inspectAssociation(ctx context.Context, req providersessions.InspectRequest) error {
	ref := req.Session
	if !capturedProviderIdentifier.MatchString(ref.ID) {
		return providersessions.ErrInvalidIdentifier
	}
	items, err := s.inspectCaptures(ctx, req)
	if err != nil {
		return err
	}
	if req.WorkerSessionID != "" {
		if _, exists := items[req.WorkerSessionID]; !exists {
			return providersessions.ErrSessionNotFound
		}
	}
	if err := validateCapturedChain(items); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return captureReadError(err)
	}
	return nil
}

func (s *capturedProvider) inspectCaptures(ctx context.Context, req providersessions.InspectRequest) (map[string]recordings.WorkerCapturedCatalogItem, error) {
	request := recordings.WorkerCapturedCatalogRequest{
		Limit: capturedPageLimit, RequireCompleteMembership: true, PreparedSummariesOnly: true,
	}
	items := make(map[string]recordings.WorkerCapturedCatalogItem)
	seen := make(map[string]bool)
	generation := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, captureReadError(err)
		}
		page, err := s.reader.ListPreparedWorkerSessionCaptures(ctx, request)
		if err != nil {
			return nil, captureReadError(err)
		}
		if request.NextToken == "" {
			generation = page.GenerationID
		} else if page.GenerationID != generation {
			return nil, providersessions.ErrSessionStorageUnavailable
		}
		for _, item := range page.Items {
			if req.WorkerSessionID != "" && item.Catalog.FactorySessionID != req.FactorySessionID {
				continue
			}
			if err := collectInspectionCapture(items, item, req.Session, generation); err != nil {
				return nil, err
			}
		}
		if page.NextToken == "" {
			break
		}
		if len(page.Items) == 0 || seen[page.NextToken] {
			return nil, providersessions.ErrSessionStorageUnavailable
		}
		seen[page.NextToken] = true
		request.NextToken = page.NextToken
	}
	if len(items) == 0 {
		return nil, providersessions.ErrSessionNotFound
	}
	return items, nil
}

func collectInspectionCapture(items map[string]recordings.WorkerCapturedCatalogItem, item recordings.WorkerCapturedCatalogItem, ref providers.SessionRef, generation string) error {
	if !captureHasRef(item, ref) {
		return nil
	}
	id := item.Catalog.WorkerSessionID
	if id == "" || generation == "" {
		return providersessions.ErrSessionStorageUnavailable
	}
	if previous, exists := items[id]; exists && !reflect.DeepEqual(previous, item) {
		return providersessions.ErrAmbiguousSessionFile
	}
	items[id] = item
	return nil
}

func validateCapturedChain(items map[string]recordings.WorkerCapturedCatalogItem) error {
	facts := make(map[string]workers.SessionPayload, len(items))
	root := ""
	for id, item := range items {
		payload, err := capturedAssociationFacts(item)
		if err != nil {
			return err
		}
		facts[id] = payload
		if payload.Lineage.PredecessorWorkerSessionID == "" {
			if root != "" {
				return providersessions.ErrAmbiguousSessionFile
			}
			root = id
		}
	}
	if root == "" {
		return providersessions.ErrSessionStorageUnavailable
	}
	seen := make(map[string]bool)
	for id := root; id != ""; {
		if seen[id] {
			return providersessions.ErrSessionStorageUnavailable
		}
		seen[id] = true
		source := facts[id]
		next := source.Lineage.SuccessorWorkerSessionID
		if next != "" {
			successor, exists := facts[next]
			if !exists || successor.FactorySessionID != source.FactorySessionID ||
				successor.Lineage.PredecessorWorkerSessionID != id ||
				successor.Lineage.PreviousDispatchID != source.DispatchID ||
				successor.Lineage.PreviousAttemptID != source.AttemptID {
				return providersessions.ErrSessionStorageUnavailable
			}
		}
		id = next
	}
	if len(seen) != len(items) {
		return providersessions.ErrAmbiguousSessionFile
	}
	return nil
}

func capturedAssociationFacts(item recordings.WorkerCapturedCatalogItem) (workers.SessionPayload, error) {
	var facts workers.SessionPayload
	if !validAssociationSummary(item) {
		return facts, providersessions.ErrSessionStorageUnavailable
	}
	var err error
	facts, err = associationOpening(item.Opening, item.Catalog)
	if err != nil {
		return facts, err
	}
	facts.Lineage = &workers.SessionLineage{}
	terminalFound := false
	var previous events.AggregateSequence
	for _, record := range append([]events.Record{item.Opening}, item.MetadataRecords...) {
		if !associationRecordInPrefix(record, item, previous) {
			return facts, providersessions.ErrSessionStorageUnavailable
		}
		previous = record.ID.Position
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil {
			return facts, providersessions.ErrSessionStorageUnavailable
		}
		if draft.Kind != workers.KindSession {
			continue
		}
		var payload workers.SessionPayload
		if json.Unmarshal(draft.Payload, &payload) != nil || !associationIdentityMatches(payload, draft, facts, item.Catalog) {
			return facts, providersessions.ErrSessionStorageUnavailable
		}
		if err := mergeAssociationLineage(&facts, payload.Lineage); err != nil {
			return facts, err
		}
		if record.ID.Position == item.Terminal.Position {
			terminalFound = draft.Phase == item.Terminal.Phase && payload.Status == item.Terminal.Status
		}
	}
	if !associationTerminalMatches(facts, item, terminalFound) {
		return facts, providersessions.ErrSessionStorageUnavailable
	}
	if item.SuccessorWorkerSessionID != "" {
		facts.Lineage.SuccessorWorkerSessionID = item.SuccessorWorkerSessionID
	}
	return facts, nil
}

func associationOpening(record events.Record, catalog recordings.WorkerSessionCatalogEntry) (workers.SessionPayload, error) {
	var draft workers.Draft
	var payload workers.SessionPayload
	if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &payload) != nil || draft.DispatchID == "" || payload.AttemptID != draft.DispatchID ||
		payload.WorkerSessionID != catalog.WorkerSessionID || payload.FactorySessionID != catalog.FactorySessionID {
		return payload, providersessions.ErrSessionStorageUnavailable
	}
	payload.DispatchID = draft.DispatchID
	return payload, nil
}

func associationTerminalMatches(facts workers.SessionPayload, item recordings.WorkerCapturedCatalogItem, found bool) bool {
	return found && (facts.Lineage.SuccessorWorkerSessionID == "" || item.SuccessorWorkerSessionID == "" ||
		facts.Lineage.SuccessorWorkerSessionID == item.SuccessorWorkerSessionID)
}

func validAssociationSummary(item recordings.WorkerCapturedCatalogItem) bool {
	return item.Health == recordings.WorkerRecordingStatusComplete && item.Terminal != nil &&
		item.Terminal.Position >= 1 && uint64(item.Terminal.Position) <= item.Catalog.CommittedPosition &&
		item.Catalog.RecordingID != "" && item.Catalog.RecordingGenerationID != "" && item.Opening.ID.Position == 1 && item.Opening.ID.Topic != ""
}

func associationRecordInPrefix(record events.Record, item recordings.WorkerCapturedCatalogItem, previous events.AggregateSequence) bool {
	return record.ID.Topic == item.Opening.ID.Topic && record.ID.Position > previous && uint64(record.ID.Position) <= item.Catalog.CommittedPosition
}

func associationIdentityMatches(payload workers.SessionPayload, draft workers.Draft, facts workers.SessionPayload, catalog recordings.WorkerSessionCatalogEntry) bool {
	// Metadata/terminal envelopes may omit correlation already fixed by the
	// opening and selected physical capture. Explicit disagreement is unsafe.
	return draft.DispatchID == facts.DispatchID && (payload.WorkerSessionID == "" || payload.WorkerSessionID == catalog.WorkerSessionID) &&
		(payload.FactorySessionID == "" || payload.FactorySessionID == catalog.FactorySessionID) && (payload.AttemptID == "" || payload.AttemptID == facts.AttemptID)
}

func mergeAssociationLineage(facts *workers.SessionPayload, lineage *workers.SessionLineage) error {
	if lineage == nil {
		return nil
	}
	if lineage.Validate(facts.WorkerSessionID, facts.DispatchID, facts.AttemptID) != nil {
		return providersessions.ErrSessionStorageUnavailable
	}
	current := facts.Lineage
	if lineage.PredecessorWorkerSessionID != "" {
		if current.PredecessorWorkerSessionID != "" && (current.PredecessorWorkerSessionID != lineage.PredecessorWorkerSessionID ||
			current.PreviousDispatchID != lineage.PreviousDispatchID || current.PreviousAttemptID != lineage.PreviousAttemptID) {
			return providersessions.ErrSessionStorageUnavailable
		}
		current.PredecessorWorkerSessionID = lineage.PredecessorWorkerSessionID
		current.PreviousDispatchID, current.PreviousAttemptID = lineage.PreviousDispatchID, lineage.PreviousAttemptID
	}
	if lineage.SuccessorWorkerSessionID != "" {
		if current.SuccessorWorkerSessionID != "" && current.SuccessorWorkerSessionID != lineage.SuccessorWorkerSessionID {
			return providersessions.ErrSessionStorageUnavailable
		}
		current.SuccessorWorkerSessionID = lineage.SuccessorWorkerSessionID
	}
	return nil
}

func (s *capturedProvider) Details(ctx context.Context, ref providers.SessionRef) (providersessions.Detail, error) {
	if err := validateSessionRef(ref); err != nil {
		return providersessions.Detail{}, err
	}
	if !capturedProviderIdentifier.MatchString(ref.ID) {
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
	return capturedProviderDetail(page, ref)
}

func (s *capturedProvider) selectCapture(ctx context.Context, ref providers.SessionRef) (recordings.WorkerSessionCatalogEntry, error) {
	request := recordings.WorkerCapturedCatalogRequest{Limit: capturedPageLimit, RequireCompleteMembership: true}
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
			selected, err = selectCapturedAssociation(selected, item.Catalog)
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

func selectCapturedAssociation(current, candidate recordings.WorkerSessionCatalogEntry) (recordings.WorkerSessionCatalogEntry, error) {
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

func (s *capturedProvider) readCapture(ctx context.Context, selected recordings.WorkerSessionCatalogEntry) (recordings.WorkerCapturedActivityPage, error) {
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
		if !validCapturedProviderPage(page, selected) {
			return first, providersessions.ErrSessionStorageUnavailable
		}
		if request.NextToken == "" {
			first = page
		} else if !sameCapturedProviderHead(page, first) {
			return first, providersessions.ErrSessionStorageUnavailable
		}
		previous, err = validateCapturedProviderRecords(page, previous)
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

func validCapturedProviderPage(page recordings.WorkerCapturedActivityPage, selected recordings.WorkerSessionCatalogEntry) bool {
	return page.Catalog == selected && page.Health == recordings.WorkerRecordingStatusComplete && page.Terminal != nil &&
		page.Terminal.Position >= 1 && uint64(page.Terminal.Position) <= selected.CommittedPosition
}

func sameCapturedProviderHead(page, first recordings.WorkerCapturedActivityPage) bool {
	return reflect.DeepEqual(page.Opening, first.Opening) && *page.Terminal == *first.Terminal && reflect.DeepEqual(page.TokenUsage, first.TokenUsage)
}

func validateCapturedProviderRecords(page recordings.WorkerCapturedActivityPage, previous uint64) (uint64, error) {
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

func capturedProviderDetail(page recordings.WorkerCapturedActivityPage, ref providers.SessionRef) (providersessions.Detail, error) {
	entries, err := page.ProjectTranscript()
	if err != nil {
		return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
	}
	detail := providersessions.Detail{
		ProviderSession: providersessions.Ref{Provider: providersessions.Provider(ref.Provider), Kind: ref.Kind, ID: ref.ID},
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
