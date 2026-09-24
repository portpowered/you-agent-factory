package service

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

// QueryHistoricalWorkerAssociations reads one artifact and returns only the
// Work-scoped Worker Session associations that can be established from its
// recorded dispatch facts. It never opens a Factory Session or activates a
// runtime.
func (service *Service) QueryHistoricalWorkerAssociations(
	request recordings.HistoricalWorkerAssociationsRequest,
) (recordings.HistoricalWorkerAssociationsResult, error) {
	started := time.Now()
	logger := serviceLogger(service)
	baseFields := []any{
		"operation", "query_historical_worker_associations",
		"recordingID", strings.TrimSpace(string(request.Recording.RecordingID)),
		"workID", strings.TrimSpace(request.WorkID),
	}
	logger.Info("recordings historical worker associations query started", baseFields...)
	outcome := "failed"
	failureClass := ""
	defer func() {
		fields := append(append([]any(nil), baseFields...), "outcome", outcome, "duration", time.Since(started))
		if failureClass != "" {
			fields = append(fields, "failureClass", failureClass)
		}
		logger.Info("recordings historical worker associations query finished", fields...)
	}()

	identity, base, err := historicalWorkerAssociationRequest(request)
	if err != nil {
		failureClass = "invalid_request"
		return recordings.HistoricalWorkerAssociationsResult{}, err
	}
	result, failureClass, err := service.queryHistoricalWorkerAssociations(identity, base)
	outcome = historicalWorkerAssociationOutcome(result, err)
	return result, err
}

func serviceLogger(service *Service) logging.Logger {
	if service == nil {
		return logging.NoopLogger{}
	}
	return logging.EnsureLogger(service.logger)
}

func historicalWorkerAssociationOutcome(
	result recordings.HistoricalWorkerAssociationsResult,
	err error,
) string {
	if err != nil {
		return "failed"
	}
	switch result.State {
	case recordings.HistoricalWorkerAssociationsAvailable:
		return "available"
	case recordings.HistoricalWorkerAssociationsGap:
		return "gap"
	case recordings.HistoricalWorkerAssociationsWorkNotFound:
		return "work_not_found"
	default:
		return "unavailable"
	}
}

func historicalWorkerAssociationRequest(
	request recordings.HistoricalWorkerAssociationsRequest,
) (recordings.HistoricalRecordingIdentity, recordings.HistoricalWorkerAssociationsResult, error) {
	identity, err := validHistoricalRecordingIdentity(request.Recording)
	if err != nil {
		return recordings.HistoricalRecordingIdentity{}, recordings.HistoricalWorkerAssociationsResult{}, err
	}
	workID := strings.TrimSpace(request.WorkID)
	if workID == "" {
		return recordings.HistoricalRecordingIdentity{}, recordings.HistoricalWorkerAssociationsResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorInvalidRequest, identity, "", nil,
		)
	}
	return identity, recordings.HistoricalWorkerAssociationsResult{
		FactorySessionID: string(identity.RecordingID),
		WorkID:           workID,
	}, nil
}

func (service *Service) queryHistoricalWorkerAssociations(
	identity recordings.HistoricalRecordingIdentity,
	base recordings.HistoricalWorkerAssociationsResult,
) (recordings.HistoricalWorkerAssociationsResult, string, error) {
	if service == nil || service.readArtifact == nil || service.projection == nil {
		return unavailableWorkerAssociations(base), "query_dependencies_unavailable", nil
	}
	payload, err := service.readArtifact(string(identity.Artifact))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return unavailableWorkerAssociations(base), "recording_not_found", nil
		}
		return unavailableWorkerAssociations(base), "artifact_read_failed", nil
	}
	if identity.Scope.FactorySessionID == "" {
		var found bool
		identity.Scope, found, err = historicalArtifactScope(payload, identity.RecordingID)
		if err != nil {
			return unavailableWorkerAssociations(base), "recording_scope_decode_failed", nil
		}
		if !found {
			return unavailableWorkerAssociations(base), "recording_scope_unavailable", nil
		}
	}
	history, err := service.queryHistoricalRecording(identity, payload)
	if err != nil {
		return historicalWorkerAssociationQueryError(base, err)
	}
	return historicalWorkerAssociationResult(base, history)
}

func historicalWorkerAssociationQueryError(
	base recordings.HistoricalWorkerAssociationsResult,
	err error,
) (recordings.HistoricalWorkerAssociationsResult, string, error) {
	var queryErr *recordings.HistoricalRecordingQueryError
	if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorInvalidRequest {
		return recordings.HistoricalWorkerAssociationsResult{}, "invalid_request", err
	}
	if errors.As(err, &queryErr) {
		switch queryErr.Kind {
		case recordings.HistoricalRecordingQueryErrorMissingHistory:
			return unavailableWorkerAssociations(base), "recording_not_found", nil
		case recordings.HistoricalRecordingQueryErrorCorruptHistory:
			return unavailableWorkerAssociations(base), "corrupt_history", nil
		case recordings.HistoricalRecordingQueryErrorUnavailable:
			return unavailableWorkerAssociations(base), "history_query_unavailable", nil
		}
	}
	return unavailableWorkerAssociations(base), "history_query_failed", nil
}

func historicalWorkerAssociationResult(
	base recordings.HistoricalWorkerAssociationsResult,
	history recordings.HistoricalRecordingQueryResult,
) (recordings.HistoricalWorkerAssociationsResult, string, error) {
	if historicalRecordingHasGap(history.Status) {
		base.State = recordings.HistoricalWorkerAssociationsGap
		base.ErrorCode = "RECORDED_WORKER_HISTORY_GAP"
		return base, "recorded_history_gap", nil
	}
	if len(history.Events) == 0 {
		return unavailableWorkerAssociations(base), "recording_has_no_events", nil
	}

	workIDsByDispatch, knownWorkIDs := historicalDispatchWorkIDs(history.Events)
	if _, found := knownWorkIDs[base.WorkID]; !found {
		base.State = recordings.HistoricalWorkerAssociationsWorkNotFound
		base.ErrorCode = "WORK_NOT_FOUND"
		return base, "work_not_found", nil
	}
	for _, dispatch := range history.Dispatches {
		if dispatch.Association == nil || !slices.Contains(workIDsByDispatch[dispatch.ID], base.WorkID) {
			continue
		}
		base.WorkerSessionIDs = append(base.WorkerSessionIDs, dispatch.Association.WorkerSessionID)
		if dispatch.Status == recordings.FactoryDispatchStatusRunning {
			base.IncompleteDispatchIDs = append(base.IncompleteDispatchIDs, dispatch.ID)
		}
	}
	slices.Sort(base.WorkerSessionIDs)
	slices.Sort(base.IncompleteDispatchIDs)
	base.Count = len(base.WorkerSessionIDs)
	base.State = recordings.HistoricalWorkerAssociationsAvailable
	return base, "", nil
}

func unavailableWorkerAssociations(
	result recordings.HistoricalWorkerAssociationsResult,
) recordings.HistoricalWorkerAssociationsResult {
	result.State = recordings.HistoricalWorkerAssociationsUnavailable
	result.ErrorCode = "RECORDED_WORKER_HISTORY_UNAVAILABLE"
	return result
}

func historicalRecordingHasGap(status recordings.RecordingStatusFacts) bool {
	for _, failure := range status.Failures {
		code := strings.ToUpper(strings.TrimSpace(failure.Code))
		if code == "RETENTION_GAP" || code == "RECORDING_GAP" || code == "CANONICAL_EVENT_GAP" {
			return true
		}
	}
	return false
}

func historicalDispatchWorkIDs(
	events []recordings.CanonicalEvent,
) (map[string][]string, map[string]struct{}) {
	byDispatch := make(map[string][]string)
	known := make(map[string]struct{})
	for _, event := range events {
		legacy := canonical.FactoryEventFromCanonical(event)
		workIDs := make([]string, 0)
		if legacy.Context.WorkIDs != nil {
			workIDs = append(workIDs, (*legacy.Context.WorkIDs)...)
		}
		if legacy.Context.DispatchID != nil {
			if legacy.Type == factorydefinitions.FactoryEventTypeDispatchRequest {
				var request factorydefinitions.DispatchRequestEventPayload
				if legacy.DecodePayload(&request) == nil {
					for _, input := range request.Inputs {
						workIDs = append(workIDs, input.WorkID)
					}
				}
			}
			if legacy.Type == factorydefinitions.FactoryEventTypeDispatchQueued {
				var queued factorydefinitions.DispatchQueuedEventPayload
				if legacy.DecodePayload(&queued) == nil && queued.InputWorkIDs != nil {
					workIDs = append(workIDs, (*queued.InputWorkIDs)...)
				}
			}
			dispatchID := strings.TrimSpace(*legacy.Context.DispatchID)
			if dispatchID != "" {
				byDispatch[dispatchID] = appendUniqueStrings(byDispatch[dispatchID], workIDs...)
			}
		}
		if legacy.Type == factorydefinitions.FactoryEventTypeWorkStateChange {
			var stateChange factorydefinitions.WorkStateChangeEventPayload
			if legacy.DecodePayload(&stateChange) == nil {
				workIDs = append(workIDs, stateChange.WorkID)
			}
		}
		for _, id := range workIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				known[id] = struct{}{}
			}
		}
	}
	return byDispatch, known
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition != "" && !slices.Contains(values, addition) {
			values = append(values, addition)
		}
	}
	return values
}

func historicalArtifactScope(
	payload []byte,
	recordingID recordings.RecordingID,
) (recordings.CanonicalEventScope, bool, error) {
	if replayimpl.IsReplayV2Artifact(payload) {
		stream, err := replayimpl.ParseReplayV2(payload)
		if err != nil {
			return recordings.CanonicalEventScope{}, false, err
		}
		if strings.TrimSpace(stream.Header.SessionID) != string(recordingID) {
			return recordings.CanonicalEventScope{}, false, nil
		}
		if len(stream.Events) == 0 {
			return recordings.CanonicalEventScope{}, false, nil
		}
		return factoryEventScope(stream.Events[0]), true, nil
	}
	var header struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return recordings.CanonicalEventScope{}, false, err
	}
	if header.SchemaVersion == string(recordings.PortableArtifactSchemaV1) {
		var artifact recordings.PortableArtifact
		if err := json.Unmarshal(payload, &artifact); err != nil {
			return recordings.CanonicalEventScope{}, false, err
		}
		return artifact.Summary.Scope, true, nil
	}
	if header.SchemaVersion == factorydefinitions.ReplayV1SourceFormat {
		var artifact legacyArtifactDocument
		if err := json.Unmarshal(payload, &artifact); err != nil {
			return recordings.CanonicalEventScope{}, false, err
		}
		if len(artifact.Events) == 0 {
			return recordings.CanonicalEventScope{}, false, nil
		}
		return factoryEventScope(artifact.Events[0]), true, nil
	}
	return recordings.CanonicalEventScope{}, false, nil
}

func factoryEventScope(event factorydefinitions.FactoryEvent) recordings.CanonicalEventScope {
	if event.Context.SessionID == nil {
		return recordings.CanonicalEventScope{}
	}
	return recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(*event.Context.SessionID)}
}
