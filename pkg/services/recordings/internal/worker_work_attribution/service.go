// Package workerworkattribution projects explicit recorded names for captured
// Worker associations. It does not consult live Work or provider state.
package workerworkattribution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// CaptureReader supplies committed identity and opening facts from the selected
// profile. The reader, rather than the caller, owns artifact selection.
type CaptureReader interface {
	ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error)
}

// HistoryReader loads canonical Factory history only during startup preparation.
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
	// Prepared projections are startup facts, indexed by the complete capture
	// identity. Requests never select, stat, read or refresh their source.
	mu       sync.RWMutex
	prepared map[preparedIdentity]nameProjection
}

type preparedIdentity struct {
	historyIdentity
	worker string
}

func captureIdentity(page recordings.WorkerCapturedActivityPage) preparedIdentity {
	c := page.Catalog
	return preparedIdentity{historyIdentity{c.FactorySessionID, c.RecordingID, c.RecordingGenerationID, c.OriginatingArtifact}, c.WorkerSessionID}
}

var _ recordings.WorkerWorkAttributionReader = (*Service)(nil)

// New constructs the component with its exact read dependencies. Production
// construction belongs to Recordings wire; construction performs no IO.
func New(captures CaptureReader, history HistoryReader) *Service {
	return &Service{captures: captures, history: history, prepared: make(map[preparedIdentity]nameProjection)}
}

// ResolveWorkerWorkAttribution shares capture lookup and materialized attribution
// within one query, preserving request order and scoped identities.
func (s *Service) ResolveWorkerWorkAttribution(ctx context.Context, requests []recordings.WorkerWorkAttributionRequest) ([]recordings.WorkerWorkAttribution, error) {
	if ctx == nil {
		return nil, recordings.ErrInvalidProjectionInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.captures == nil {
		return nil, recordings.ErrMissingWorkerRecordingReader
	}
	for _, request := range requests {
		if strings.TrimSpace(request.WorkerSessionID) == "" || request.WorkerSessionID != strings.TrimSpace(request.WorkerSessionID) {
			return nil, recordings.ErrInvalidWorkerRecordingRequest
		}
	}
	query := attributionQuery{
		service: s, captures: make(map[string]recordings.WorkerCapturedActivityPage),
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

type historyIdentity struct{ factory, recording, generation, artifact string }

type projectionResult struct {
	projection nameProjection
	err        error
}

type attributionQuery struct {
	service     *Service
	captures    map[string]recordings.WorkerCapturedActivityPage
	projections map[historyIdentity]projectionResult
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
	name, err := capturedWorkName(page.Catalog.WorkName, opening)
	if err != nil {
		return result, err
	}
	result.WorkName = name
	q.service.mu.RLock()
	projection := q.service.prepared[captureIdentity(page)]
	q.service.mu.RUnlock()
	association, exists := projection.associations[request.WorkerSessionID]
	if !exists {
		return result, nil // Optional absence never changes capture health/provider.
	}
	if association.dispatch != opening.DispatchID || !containsWork(association.workIDs, request.WorkID) {
		return result, recordings.ErrInvalidProjectionInput
	}
	names := make(map[string]string)
	if result.WorkName != "" {
		names[request.WorkID] = result.WorkName
	}
	if err := recordExplicitName(names, request.WorkID, projection.names[request.WorkID]); err != nil {
		return result, err
	}
	result.WorkName = names[request.WorkID]
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

// Legacy opening labels are explicit Work facts only after validateOpening has
// proved their primary Work/Factory/Worker/recording association. Arbitrary
// provider metadata and labels on other records cannot supply a Work name.
func capturedWorkName(catalogName string, opening workers.SessionPayload) (string, error) {
	if len(opening.SessionMetadata) == 0 {
		return catalogName, nil
	}
	var metadata struct {
		Labels []string `json:"labels"`
	}
	if json.Unmarshal(opening.SessionMetadata, &metadata) != nil {
		return "", recordings.ErrInvalidProjectionInput
	}
	names := make(map[string]string)
	if catalogName != "" {
		names["work"] = catalogName
	}
	for _, label := range metadata.Labels {
		const prefix = "tag:_work_name="
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		// A legacy label needs an explicit physical attempt, never an inferred
		// association from a reused Work name or provider reference.
		if opening.AttemptID == "" {
			return "", recordings.ErrWorkerRecordingReplay
		}
		if err := recordExplicitName(names, "work", strings.TrimPrefix(label, prefix)); err != nil {
			return "", err
		}
	}
	return names["work"], nil
}
