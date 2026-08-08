package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

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
	ordered := orderedReadModels(snapshot, selection)
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
	for _, item := range snapshot.Items {
		if item.CursorID == id {
			return detachReadModel(item), nil
		}
	}
	for _, item := range snapshot.Items {
		if item.WorkID == id {
			return detachReadModel(item), nil
		}
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
	if s == nil || s.recordings == nil {
		return work.ReadSnapshot{}, errors.New("Work state access recordings adapter is required")
	}
	snapshot, err := s.recordings.ReadWorkSnapshot(ctx, sessionID)
	if err != nil {
		return work.ReadSnapshot{}, fmt.Errorf("read Work snapshot from Recordings: %w", err)
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
		StateName:    optional(options.StateName),
		StateType:    optional(options.StateType),
		Name:         optional(options.Name),
		WorkTypeName: optional(options.WorkTypeName),
		TraceID:      optional(options.TraceID),
		Terminal:     optionalBool(options.Terminal),
		NonTerminal:  optionalBool(options.NonTerminal),
		SortBy:       options.SortBy,
	})
}

func orderedReadModels(snapshot work.ReadSnapshot, selection stateaccessquery.Selection) []work.ReadModel {
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	items := make([]stateaccessquery.Item, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		item = detachReadModel(item)
		byID[item.CursorID] = item
		items = append(items, stateaccessquery.Item{
			ID:                     item.CursorID,
			Name:                   item.Name,
			WorkTypeName:           item.WorkTypeName,
			State:                  stateToQueryState(item.State),
			TraceID:                item.TraceID,
			CurrentChainingTraceID: item.CurrentChainingTraceID,
		})
	}
	selected := selection.Apply(items)
	ordered := make([]work.ReadModel, 0, len(selected))
	for _, item := range selected {
		ordered = append(ordered, byID[item.ID])
	}
	return ordered
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
	item.Tags = work.CloneTags(item.Tags)
	item.Relations = append([]work.ReadRelation(nil), item.Relations...)
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
