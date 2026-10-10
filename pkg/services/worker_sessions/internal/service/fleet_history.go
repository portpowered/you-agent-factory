package service

import (
	"context"
	"sort"
	"strings"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

const fleetHistorySourcePageSize = 1000

// FleetHistory samples admitted owners before projecting the shared durable
// catalog. It owns one bounded snapshot cache for the process-wide query.
type capturedObservationReader interface {
	GetCapturedObservation(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error)
}

type FleetHistory struct {
	continuation capturedObservationReader
	catalog      func(context.Context) ([]workersessions.Service, error)
	logs         *LogReader
	clock        platformclock.Source
	logger       logging.Logger
	snapshots    observationSnapshots
}

func NewFleetHistory(catalog func(context.Context) ([]workersessions.Service, error), captured recordings.WorkerCapturedActivityReader, clock platformclock.Source, logger logging.Logger, snapshots *HistorySnapshotBudget, continuation capturedObservationReader) *FleetHistory {
	if catalog == nil || clock == nil || snapshots == nil {
		return nil
	}
	query := &FleetHistory{continuation: continuation, catalog: catalog, clock: clock, logger: logging.EnsureLogger(logger), snapshots: newObservationSnapshots(snapshots)}
	// Older injected writers can lack captured-read support. Active queries
	// remain available; durable queries explicitly report unavailable.
	if captured != nil {
		query.logs = &LogReader{reader: captured, logger: query.logger}
	}
	return query
}

func (s *FleetHistory) ListWorkerSessionObservations(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	if s == nil {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	if err := req.Validate(); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	if req.History == "" {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrInvalidObservationHistory
	}
	if ctx == nil {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	limit := req.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	filter := historyFilter(req)
	var result workersessions.ListWorkerSessionObservationsResult
	var err error
	if token := strings.TrimSpace(req.NextToken); token != "" {
		result, err = s.snapshots.next(token, filter, limit, s.clock.Now())
	} else {
		var rows []workersessions.Observation
		rows, err = s.observations(ctx, req)
		if err == nil {
			result, err = s.snapshots.first(rows, filter, limit, s.clock.Now())
		}
	}
	if canceled := ctx.Err(); canceled != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, canceled
	}
	s.logger.Info("worker session fleet history list", "history", string(req.History), "result_count", len(result.Observations), "has_next", result.NextToken != "", "failed", err != nil)
	return result, err
}

func (s *FleetHistory) observations(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error) {
	sources, err := s.catalog(ctx)
	if err != nil {
		return nil, err
	}
	owners := make(map[historyIdentity]struct{})
	result := make([]workersessions.Observation, 0)
	ownerRequest := req
	ownerRequest.History, ownerRequest.States = workersessions.ObservationHistoryActive, nil
	ownerRequest.MaxResults, ownerRequest.NextToken = fleetHistorySourcePageSize, ""
	for _, source := range sources {
		if source == nil {
			continue
		}
		rows, err := readFleetHistoryOwners(ctx, source, ownerRequest)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := identityOfHistory(row)
			if _, duplicate := owners[key]; duplicate {
				continue
			}
			owners[key] = struct{}{}
			if req.History != workersessions.ObservationHistoryArchived && observationStateMatches(row.State, req.States) {
				result = append(result, row.Clone())
			}
		}
	}
	if req.History != workersessions.ObservationHistoryActive {
		if s.logs == nil {
			return nil, workersessions.ErrObservationProjectionUnavailable
		}
		archived, err := s.logs.archivedHistory(ctx, req, owners)
		if err != nil {
			return nil, err
		}
		s.completeArchivedCapabilities(ctx, archived)
		result = append(result, archived...)
	}
	sort.Slice(result, func(i, j int) bool {
		return historyIdentityLess(identityOfHistory(result[i]), identityOfHistory(result[j]))
	})
	return result, ctx.Err()
}

func historyIdentityLess(left, right historyIdentity) bool {
	if left.worker != right.worker {
		return left.worker < right.worker
	}
	if left.factory != right.factory {
		return left.factory < right.factory
	}
	return left.attempt < right.attempt
}

func readFleetHistoryOwners(ctx context.Context, source workersessions.Service, req workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error) {
	// Registries already return detached, sorted admitted owners. Sampling them
	// directly avoids retaining an extra cursor snapshot in every source cache
	// for a fleet query that freezes the same facts again in its own cache.
	if owner, ok := source.(interface {
		activeHistoryObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error)
	}); ok {
		rows, err := owner.activeHistoryObservations(ctx, req)
		if err != nil {
			return nil, err
		}
		return rows, ctx.Err()
	}
	return readFleetHistoryOwnerPages(ctx, source, req)
}

func readFleetHistoryOwnerPages(ctx context.Context, source workersessions.Service, req workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error) {
	rows := make([]workersessions.Observation, 0)
	seenTokens := make(map[string]struct{})
	var previous historyIdentity
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := source.ListWorkerSessionObservations(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(page.Observations) > req.MaxResults || (page.NextToken != "" && len(page.Observations) != req.MaxResults) {
			return nil, workersessions.ErrInvalidObservationPagination
		}
		for _, row := range page.Observations {
			key := identityOfHistory(row)
			if !validFleetHistoryOwner(row, req) || (len(rows) != 0 && !historyIdentityLess(previous, key)) {
				return nil, workersessions.ErrObservationProjectionUnavailable
			}
			rows, previous = append(rows, row.Clone()), key
		}
		if page.NextToken == "" {
			return rows, ctx.Err()
		}
		if _, repeated := seenTokens[page.NextToken]; repeated || page.NextToken == req.NextToken {
			return nil, workersessions.ErrInvalidObservationPagination
		}
		seenTokens[page.NextToken] = struct{}{}
		req.NextToken = page.NextToken
	}
}

func validFleetHistoryOwner(row workersessions.Observation, req workersessions.ListWorkerSessionObservationsRequest) bool {
	return row.Validate() == nil && !row.State.Terminal() && observationScopeMatches(row.Direct, req.Scope) &&
		(req.FactorySessionID == "" || row.FactorySessionID == strings.TrimSpace(req.FactorySessionID))
}

// Reuse the injected admission owner for current capability. Fleet history
// continues to own its selected historical identity and health projection.
func (s *FleetHistory) completeArchivedCapabilities(ctx context.Context, rows []workersessions.Observation) {
	if s.continuation == nil {
		return
	}
	for i := range rows {
		row, err := s.continuation.GetCapturedObservation(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
			WorkerSessionID: rows[i].WorkerSessionID, FactorySessionID: rows[i].FactorySessionID,
		})
		if err == nil && identityOfHistory(row) == identityOfHistory(rows[i]) {
			rows[i].Revivable = row.Revivable
			rows[i].ContinuationHeadWorkerSessionID = row.ContinuationHeadWorkerSessionID
		}
	}
}
