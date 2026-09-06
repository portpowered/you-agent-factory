package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/stateaccessquery"
)

func (s *Service) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.ListResult{}, err
	}
	query, err := work.NormalizeList(options)
	if err != nil {
		return work.ListResult{}, err
	}
	snapshot, err := s.readSnapshot(ctx, sessionID)
	if err != nil {
		return work.ListResult{}, err
	}
	normalized := query.Options()
	selection, err := newSelection(normalized)
	if err != nil {
		return work.ListResult{}, mapQueryValidationError(err)
	}
	ordered := orderedReadModels(s.readModels(snapshot), selection)
	maxResults := normalized.MaxResults
	if maxResults <= 0 {
		maxResults = work.DefaultListMaxResults
	}
	start := 0
	if normalized.NextToken != "" {
		decoded, _ := base64.StdEncoding.DecodeString(normalized.NextToken)
		start = nextIndex(ordered, string(decoded))
	}
	end := min(start+maxResults, len(ordered))
	result := work.ListResult{
		Results:    append([]work.ReadModel(nil), ordered[start:end]...),
		MaxResults: maxResults,
	}
	if normalized.Counts {
		result.Counts = &work.ListCountSummary{Total: len(ordered)}
	}
	if end < len(ordered) {
		result.NextToken = base64.StdEncoding.EncodeToString([]byte(ordered[end-1].CursorID))
	}
	return result, nil
}

func (s *Service) GetWork(
	ctx context.Context,
	sessionID string,
	id string,
) (work.ReadModel, error) {
	if err := requireContext(ctx); err != nil {
		return work.ReadModel{}, err
	}
	snapshot, err := s.readSnapshot(ctx, sessionID)
	if err != nil {
		return work.ReadModel{}, err
	}
	models := s.readModels(snapshot)
	for _, item := range models {
		if item.CursorID == id {
			return item, nil
		}
	}
	for _, item := range models {
		if item.WorkID == id {
			return item, nil
		}
	}
	// An admitted Work can be temporarily absent from the current token
	// projection while its dispatch owns the consumed token. The admission
	// ledger still carries the stable public identity and name, so an exact
	// Work-ID read must not report a false not-found during that interval.
	for _, admission := range snapshot.Admissions {
		if admission.WorkID != id {
			continue
		}
		return work.ReadModel{
			CursorID: admission.WorkID,
			WorkID:   admission.WorkID,
			Name:     admission.Name,
		}, nil
	}
	return work.ReadModel{}, work.ErrWorkNotFound
}

func (s *Service) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	id string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	if _, err := s.MoveWorkForSession(ctx, sessionID, id, stateName, requestID); err != nil {
		return work.ReadModel{}, err
	}
	return s.GetWork(ctx, sessionID, id)
}

func (s *Service) readSnapshot(ctx context.Context, sessionID string) (work.ReadSnapshot, error) {
	if adapter, err := s.tryResolveSession(sessionID); err == nil && adapter != nil {
		snapshot, err := adapter.ReadWorkSnapshot(ctx)
		if err != nil {
			return work.ReadSnapshot{}, fmt.Errorf("read Work snapshot: %w", err)
		}
		return snapshot, nil
	}
	if s == nil || s.snapshots == nil {
		return work.ReadSnapshot{}, errors.New("Work state access snapshot reader is required")
	}
	snapshot, err := s.snapshots.ReadWorkSnapshot(ctx, sessionID)
	if err != nil {
		return work.ReadSnapshot{}, fmt.Errorf("read projected Work snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *Service) tryResolveSession(sessionID string) (stateaccess.SessionAdapter, error) {
	if s == nil || s.sessions == nil {
		return nil, errors.New("Work state access session resolver is required")
	}
	adapter, err := s.sessions.ResolveSessionAdapter(sessionID)
	if err != nil {
		return nil, err
	}
	return adapter, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalBool(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

func newSelection(options work.ListOptions) (stateaccessquery.Selection, error) {
	return stateaccessquery.NewSelectionWithOptions(stateaccessquery.SelectionOptions{
		StateName:         optional(options.StateName),
		StateType:         optional(options.StateType),
		Name:              optional(options.Name),
		WorkTypeName:      optional(options.WorkTypeName),
		TraceID:           optional(options.TraceID),
		Terminal:          optionalBool(options.Terminal),
		NonTerminal:       optionalBool(options.NonTerminal),
		IncludeSuperseded: options.IncludeSuperseded,
		SortBy:            options.SortBy,
	})
}

func orderedReadModels(annotated []work.ReadModel, selection stateaccessquery.Selection) []work.ReadModel {
	byID := make(map[string]work.ReadModel, len(annotated))
	items := make([]stateaccessquery.Item, 0, len(annotated))
	for _, item := range annotated {
		byID[item.CursorID] = item
		items = append(items, stateaccessquery.Item{
			ID:                     item.CursorID,
			WorkID:                 item.WorkID,
			Name:                   item.Name,
			WorkTypeName:           item.WorkTypeName,
			State:                  stateToQueryState(item.State),
			TraceID:                item.TraceID,
			CurrentChainingTraceID: item.CurrentChainingTraceID,
			SupersededBy:           item.SupersededBy,
		})
	}
	selected := selection.Apply(items)
	ordered := make([]work.ReadModel, 0, len(selected))
	for _, item := range selected {
		ordered = append(ordered, byID[item.ID])
	}
	return ordered
}

// readModels applies all response-wide read annotations after one snapshot
// has been acquired. The durability watermark is sampled once here so every
// row in a list response observes the same completed-flush boundary as a
// corresponding show response observes for its snapshot.
func (s *Service) readModels(snapshot work.ReadSnapshot) []work.ReadModel {
	models := annotatedReadModels(snapshot)
	sequence, available := s.sampleCompletedFlushSequence(snapshot.StreamGenerationID)
	for index := range models {
		models[index].ConfirmationState = work.ConfirmationStateUnconfirmed
		if available && models[index].CurrentStateSequenceKnown &&
			models[index].CurrentStateSequence <= sequence {
			models[index].ConfirmationState = work.ConfirmationStateConfirmed
		}
	}
	return models
}

func (s *Service) sampleCompletedFlushSequence(streamGenerationID string) (int64, bool) {
	if s == nil || s.durability == nil || streamGenerationID == "" {
		return 0, false
	}
	return s.durability.CompletedFlushSequence(streamGenerationID)
}

func annotatedReadModels(snapshot work.ReadSnapshot) []work.ReadModel {
	models := make([]work.ReadModel, 0, len(snapshot.Items))
	items := make([]stateaccessquery.Item, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		item = detachReadModel(item)
		// Supersession is a read-time derivation. Do not carry a provider-supplied
		// value across the boundary when canonical admission facts are absent.
		item.SupersededBy = ""
		models = append(models, item)
		items = append(items, stateaccessquery.Item{
			ID:                     item.CursorID,
			WorkID:                 item.WorkID,
			Name:                   item.Name,
			WorkTypeName:           item.WorkTypeName,
			State:                  stateToQueryState(item.State),
			TraceID:                item.TraceID,
			CurrentChainingTraceID: item.CurrentChainingTraceID,
		})
	}

	admissions := make([]stateaccessquery.Admission, 0, len(snapshot.Admissions))
	for _, admission := range snapshot.Admissions {
		admissions = append(admissions, stateaccessquery.Admission{
			WorkID: admission.WorkID,
			Name:   admission.Name,
			Order:  admission.Order,
		})
	}
	annotatedItems := stateaccessquery.AnnotateSupersession(items, admissions)
	for index, item := range annotatedItems {
		if index < len(models) {
			models[index].SupersededBy = item.SupersededBy
			if item.SupersededBy != "" {
				// A superseded failed attempt is historical context, not the
				// current failure for this Work name. Do not expose its detail
				// through the public read projection.
				models[index].FailureDetail = nil
			}
		}
	}
	return models
}

func nextIndex(items []work.ReadModel, cursor string) int {
	for i, item := range items {
		if item.CursorID == cursor {
			return i + 1
		}
	}
	return len(items)
}

func detachReadModel(item work.ReadModel) work.ReadModel {
	item.PreviousChainingTraceIDs = append([]string(nil), item.PreviousChainingTraceIDs...)
	item.Content = work.CloneWorkContentParts(item.Content)
	item.StructuredResult = jsonvalue.Clone(item.StructuredResult)
	item.StructuredResultPresent = jsonvalue.Present(item.StructuredResult, item.StructuredResultPresent)
	item.Tags = work.CloneTags(item.Tags)
	item.Relations = append([]work.ReadRelation(nil), item.Relations...)
	item.ExpectedArtifacts = append([]work.ExpectedArtifactReadModel(nil), item.ExpectedArtifacts...)
	if item.FailureDetail != nil {
		detail := *item.FailureDetail
		item.FailureDetail = &detail
	}
	if item.State != nil {
		state := *item.State
		item.State = &state
	}
	item.StopSummary = cloneStopSummary(item.StopSummary)
	return item
}

func cloneStopSummary(summary *work.StopSummary) *work.StopSummary {
	if summary == nil {
		return nil
	}
	clone := *summary
	if summary.LatestDispatch != nil {
		dispatch := *summary.LatestDispatch
		clone.LatestDispatch = &dispatch
		if summary.LatestDispatch.FailureDetail != nil {
			detail := *summary.LatestDispatch.FailureDetail
			clone.LatestDispatch.FailureDetail = &detail
		}
	}
	return &clone
}

func stateToQueryState(state *work.State) *stateaccessquery.State {
	if state == nil {
		return nil
	}
	return &stateaccessquery.State{Name: state.Name, Type: state.Type}
}

func mapQueryValidationError(err error) error {
	if err == nil {
		return nil
	}
	var validation *stateaccessquery.ValidationError
	if errors.As(err, &validation) {
		return &work.ValidationError{Field: validation.Field, Message: validation.Message}
	}
	return err
}
