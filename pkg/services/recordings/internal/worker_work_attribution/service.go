// Package workerworkattribution projects explicit recorded names for captured
// Worker associations. It does not consult live Work or provider state.
package workerworkattribution

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
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

type historyIdentity struct{ factory, recording, artifact string }

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
	page, err := q.capture(ctx, request.WorkerSessionID)
	if err != nil {
		return result, err
	}
	if page.Catalog.WorkerSessionID == "" {
		return result, nil // No compatible capture: live presentation may still supply its name.
	}
	opening, err := validateOpening(page, request)
	if err != nil {
		return result, err
	}
	projection, err := q.projection(ctx, page, request.FactorySessionID)
	if err != nil {
		if isUnavailableHistory(err) {
			result.HistoryUnavailable = true
			return result, nil // Optional attribution is unavailable; capture identity and health remain authoritative.
		}
		return result, err
	}
	association, exists := projection.associations[request.WorkerSessionID]
	if !exists {
		if page.Catalog.OriginatingArtifact == "" || projection.reportedDefault {
			result.HistoryUnavailable = true
			return result, nil // The legacy candidate has no validated association; never borrow its name.
		}
		return result, nil // Histories may lack a canonical association.
	}
	if association.dispatch != opening.DispatchID || !containsWork(association.workIDs, request.WorkID) {
		return result, recordings.ErrInvalidProjectionInput
	}
	result.WorkName = projection.names[request.WorkID]
	return result, nil
}

func isUnavailableHistory(err error) bool {
	var historyError *recordings.HistoricalRecordingQueryError
	return errors.As(err, &historyError) && (historyError.Kind == recordings.HistoricalRecordingQueryErrorMissingHistory || historyError.Kind == recordings.HistoricalRecordingQueryErrorUnavailable)
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

func (q *attributionQuery) capture(ctx context.Context, workerID string) (recordings.WorkerCapturedActivityPage, error) {
	if page, loaded := q.captures[workerID]; loaded {
		return page, nil
	}
	page, err := q.service.captures.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: workerID, Limit: 1})
	if canceled := ctx.Err(); canceled != nil {
		return page, canceled
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return page, err
	}
	if err == nil && page.Catalog.WorkerSessionID == "" {
		return page, recordings.ErrWorkerRecordingReplay
	}
	q.captures[workerID] = page
	return page, nil
}

func (q *attributionQuery) projection(ctx context.Context, page recordings.WorkerCapturedActivityPage, factory string) (nameProjection, error) {
	key := historyIdentity{page.Catalog.FactorySessionID, page.Catalog.RecordingID, page.Catalog.OriginatingArtifact}
	projection, loaded := q.projections[key]
	if !loaded {
		history, err := q.service.history.ReadWorkerFactoryHistory(ctx, page)
		if canceled := ctx.Err(); canceled != nil {
			return nameProjection{}, canceled
		}
		if err != nil {
			return nameProjection{}, err
		}
		// Default recordings retain their reported ~default token while the
		// capture retains the canonical UUID. Accept that source-native alias
		// only from the selected artifact; exact captured association checks below
		// remain mandatory. A foreign explicit scope is never accepted.
		sourceScope := history.Recording.Scope.FactorySessionID
		if sourceScope != factory {
			if _, err := uuid.Parse(factory); err != nil || sourceScope != "~default" {
				return nameProjection{}, recordings.ErrInvalidProjectionScope
			}
		}
		projection, err = projectNames(history, sourceScope)
		if err != nil {
			return nameProjection{}, err
		}
		projection.reportedDefault = sourceScope != factory
		q.projections[key] = projection
	}
	return projection, nil
}
