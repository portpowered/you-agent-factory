package service

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (s *applicationService) ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	query, err := work.NormalizeList(options)
	if err != nil {
		return work.ListResult{}, err
	}
	snapshot, err := s.snapshot(ctx, sessionID)
	if err != nil {
		return work.ListResult{}, err
	}
	normalized := query.Options()
	selection, err := work.NewSelection(optional(normalized.StateName), optional(normalized.StateType), optional(normalized.Name), optional(normalized.WorkTypeName), optional(normalized.TraceID), normalized.SortBy)
	if err != nil {
		return work.ListResult{}, err
	}
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	items := make([]work.Item, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		item = cloneReadModel(item)
		byID[item.CursorID] = item
		items = append(items, work.Item{ID: item.CursorID, Name: item.Name, WorkTypeName: item.WorkTypeName, State: item.State, TraceID: item.TraceID, CurrentChainingTraceID: item.CurrentChainingTraceID})
	}
	selected := selection.Apply(items)
	ordered := make([]work.ReadModel, 0, len(selected))
	for _, item := range selected {
		ordered = append(ordered, byID[item.ID])
	}
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
	result := work.ListResult{Results: append([]work.ReadModel(nil), ordered[start:end]...), MaxResults: maxResults}
	if end < len(ordered) {
		result.NextToken = base64.StdEncoding.EncodeToString([]byte(ordered[end-1].CursorID))
	}
	return result, nil
}

func (s *applicationService) GetWork(ctx context.Context, sessionID, id string) (work.ReadModel, error) {
	snapshot, err := s.snapshot(ctx, sessionID)
	if err != nil {
		return work.ReadModel{}, err
	}
	for _, item := range snapshot.Items {
		if item.CursorID == id {
			return cloneReadModel(item), nil
		}
	}
	for _, item := range snapshot.Items {
		if item.WorkID == id {
			return cloneReadModel(item), nil
		}
	}
	return work.ReadModel{}, work.ErrWorkNotFound
}

func (s *applicationService) MoveWorkAndRead(ctx context.Context, sessionID, id, stateName, requestID string) (work.ReadModel, error) {
	if _, err := s.MoveWorkForSession(ctx, sessionID, id, stateName, requestID); err != nil {
		return work.ReadModel{}, err
	}
	return s.GetWork(ctx, sessionID, id)
}

func (s *applicationService) snapshot(ctx context.Context, sessionID string) (work.ReadSnapshot, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	snapshot, err := runtime.ReadWorkSnapshot(ctx)
	if err != nil {
		return work.ReadSnapshot{}, fmt.Errorf("read Work snapshot: %w", err)
	}
	return snapshot, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func nextIndex(items []work.ReadModel, cursor string) int {
	for i, item := range items {
		if item.CursorID == cursor {
			return i + 1
		}
	}
	return len(items)
}

func cloneReadModel(item work.ReadModel) work.ReadModel {
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
