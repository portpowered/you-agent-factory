// Package workerworkattribution projects explicit recorded names for captured
// Worker associations. It does not consult live Work or provider state.
package workerworkattribution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// CaptureReader supplies committed identity and opening facts from the selected
// profile. The reader, rather than the caller, owns artifact selection.
type CaptureReader interface {
	ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error)
}

// HistoryReader selects canonical Factory history for the accepted capture.
// Its implementation must validate originating recording identity and use the
// existing Recordings codecs; no Work-ID search across recordings is permitted.
type HistoryReader interface {
	ReadWorkerFactoryHistory(context.Context, recordings.WorkerCapturedActivityPage) (recordings.HistoricalRecordingQueryResult, error)
}

// Service is the read-only attribution component. High-volume presentation
// reads deliberately reuse the capture/history owners' safe error diagnostics
// rather than logging customer names or emitting a log for every listed row.
type Service struct {
	captures CaptureReader
	history  HistoryReader
}

var _ recordings.WorkerWorkAttributionReader = (*Service)(nil)

// New constructs the component with its exact read dependencies. Production
// construction belongs to Recordings wire; construction performs no IO.
func New(captures CaptureReader, history HistoryReader) *Service {
	return &Service{captures: captures, history: history}
}

// ResolveWorkerWorkAttribution shares capture lookup and canonical reduction
// within one query, preserving request order and scoped identities.
func (s *Service) ResolveWorkerWorkAttribution(ctx context.Context, requests []recordings.WorkerWorkAttributionRequest) ([]recordings.WorkerWorkAttribution, error) {
	if ctx == nil {
		return nil, recordings.ErrInvalidProjectionInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.captures == nil || s.history == nil {
		return nil, recordings.ErrMissingWorkerRecordingReader
	}
	for _, request := range requests {
		if strings.TrimSpace(request.WorkerSessionID) == "" || request.WorkerSessionID != strings.TrimSpace(request.WorkerSessionID) {
			return nil, recordings.ErrInvalidWorkerRecordingRequest
		}
	}
	query := attributionQuery{
		service: s, captures: make(map[string]recordings.WorkerCapturedActivityPage),
		projections: make(map[historyIdentity]nameProjection),
	}
	results := make([]recordings.WorkerWorkAttribution, 0, len(requests))
	for _, request := range requests {
		result, err := query.resolve(ctx, request)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, ctx.Err()
}

type historyIdentity struct{ factory, recording string }

type attributionQuery struct {
	service     *Service
	captures    map[string]recordings.WorkerCapturedActivityPage
	projections map[historyIdentity]nameProjection
}

func (q *attributionQuery) resolve(ctx context.Context, request recordings.WorkerWorkAttributionRequest) (recordings.WorkerWorkAttribution, error) {
	result := recordings.WorkerWorkAttribution{
		WorkerSessionID: request.WorkerSessionID, FactorySessionID: request.FactorySessionID, WorkID: request.WorkID,
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if request.FactorySessionID == "" || request.WorkID == "" {
		return result, nil
	}
	page, loaded := q.captures[request.WorkerSessionID]
	if !loaded {
		var err error
		page, err = q.service.captures.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{
			WorkerSessionID: request.WorkerSessionID, Limit: 1,
		})
		if canceled := ctx.Err(); canceled != nil {
			return result, canceled
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if err == nil && page.Catalog.WorkerSessionID == "" {
			return result, recordings.ErrWorkerRecordingReplay
		}
		q.captures[request.WorkerSessionID] = page
	}
	if page.Catalog.WorkerSessionID == "" {
		return result, nil // No compatible capture: live presentation may still supply its name.
	}
	opening, err := validateOpening(page, request)
	if err != nil {
		return result, err
	}
	key := historyIdentity{page.Catalog.FactorySessionID, page.Catalog.RecordingID}
	projection, loaded := q.projections[key]
	if !loaded {
		history, err := q.service.history.ReadWorkerFactoryHistory(ctx, page)
		if canceled := ctx.Err(); canceled != nil {
			return result, canceled
		}
		if err != nil {
			return result, err
		}
		projection, err = projectNames(history, request.FactorySessionID)
		if err != nil {
			return result, err
		}
		q.projections[key] = projection
	}
	association, exists := projection.associations[request.WorkerSessionID]
	if !exists {
		return result, nil // Legacy histories may lack a canonical association.
	}
	if association.dispatch != opening.DispatchID || !containsWork(association.workIDs, request.WorkID) {
		return result, recordings.ErrInvalidProjectionInput
	}
	result.WorkName = projection.names[request.WorkID]
	return result, nil
}

func validateOpening(page recordings.WorkerCapturedActivityPage, request recordings.WorkerWorkAttributionRequest) (workers.SessionPayload, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || page.Opening.ID.Position != 1 ||
		page.Catalog.WorkerSessionID != request.WorkerSessionID || page.Catalog.FactorySessionID != request.FactorySessionID ||
		opening.WorkerSessionID != request.WorkerSessionID || opening.FactorySessionID != request.FactorySessionID ||
		page.Catalog.RecordingID == "" || opening.RecordingID != page.Catalog.RecordingID || opening.DispatchID == "" ||
		len(opening.WorkIDs) == 0 || opening.WorkIDs[0] != request.WorkID {
		return workers.SessionPayload{}, recordings.ErrWorkerRecordingReplay
	}
	return opening, nil
}

func containsWork(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
