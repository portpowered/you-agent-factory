package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

type durableSessionStateReader interface {
	HasDurableState(context.Context, string) (bool, error)
}

type currentBoardHistoryOpening struct {
	allowMissingHistory bool
	hasDurableState     bool
}

const (
	currentBoardRecordingFailureCode    = "CURRENT_BOARD_RECORDING_FAILURE"
	currentBoardRecordingMissingCode    = "CURRENT_BOARD_RECORDING_MISSING"
	currentBoardRecordingCorruptCode    = "CURRENT_BOARD_RECORDING_CORRUPT"
	currentBoardRecordingUnreadableCode = "CURRENT_BOARD_RECORDING_UNREADABLE"
)

// currentBoardHistoryRestoreError is also a safe CLI error. The method names
// intentionally match the transport's narrow coded-error interface without
// making runtime opening depend on the CLI package. Its Error method never
// includes the underlying cause, which keeps startup diagnostics safe while
// Unwrap retains typed failure matching for service callers.
type currentBoardHistoryRestoreError struct {
	code    string
	message string
	cause   error
}

func (err *currentBoardHistoryRestoreError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

func (err *currentBoardHistoryRestoreError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *currentBoardHistoryRestoreError) CLIErrorCode() string {
	if err == nil {
		return ""
	}
	return err.code
}

func (err *currentBoardHistoryRestoreError) CLIErrorMessage() string {
	if err == nil {
		return ""
	}
	return err.message
}

func logCurrentBoardHistoryFailure(
	logger *zap.Logger,
	sessionID string,
	recordPath string,
	err error,
) {
	if logger == nil {
		return
	}
	kind := "UNREADABLE_OR_CORRUPT_RECORDING"
	var queryErr *recordings.HistoricalRecordingQueryError
	if errors.As(err, &queryErr) && queryErr.Kind != "" {
		kind = string(queryErr.Kind)
	}
	logger.Error(
		"current Factory Session board recording could not be restored; startup aborted",
		zap.String("session_id", sessionID),
		zap.String("recording_path", recordPath),
		zap.String("failure", kind),
	)
}

// inspectCurrentBoardHistory makes the missing-history escape hatch explicit.
// A successful persistence probe is required before a missing board recording
// can be treated as either a fresh opening or an interrupted write. The latter
// is observable through hasDurableState so the caller can warn that the board
// was lost while preserving the durable snapshot.
func inspectCurrentBoardHistory(
	ctx context.Context,
	service any,
	sessionID string,
) (currentBoardHistoryOpening, error) {
	if service == nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	probe, ok := service.(durableSessionStateReader)
	if !ok {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	hasDurableState, err := probe.HasDurableState(ctx, sessionID)
	if err != nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history initialization: %w", err)
	}
	return currentBoardHistoryOpening{
		allowMissingHistory: true,
		hasDurableState:     hasDurableState,
	}, nil
}

// restoreCurrentBoardState loads a detached Factory world state through the
// public Recordings history contract for callers that explicitly request a
// current-board read.
func restoreCurrentBoardState(
	service historicalRecordingReader,
	recordPath string,
	sessionID string,
	allowMissingHistory bool,
) (*factorydefinitions.FactoryWorldState, error) {
	recordPath = strings.TrimSpace(recordPath)
	if recordPath == "" {
		return nil, nil
	}
	recordPath = factoryruntime.RecordingPath(recordPath).ForSession(sessionID)
	if service == nil {
		return nil, fmt.Errorf("restore current Factory Session board: Recordings history is unavailable")
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(sessionID)}
	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID("current-board/" + scope.FactorySessionID),
			Artifact:    recordings.RecordingArtifactReference(recordPath),
			Scope:       scope,
		},
	})
	if err != nil {
		var queryErr *recordings.HistoricalRecordingQueryError
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorMissingHistory {
			if allowMissingHistory {
				return nil, nil
			}
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"MISSING_HISTORY: durable state exists but the current-board recording is missing; preserve the durable snapshot and restore the recording from a trusted backup without deleting the snapshot",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorCorruptHistory {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"CORRUPT_HISTORY: the current-board recording is corrupt or incompatible; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorUnavailable {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"UNREADABLE_RECORDING: the current-board recording is present but unreadable; preserve the artifact for investigation and repair access or replace it from a trusted backup before retrying",
				err,
			)
		}
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"UNREADABLE_OR_CORRUPT_RECORDING: the current-board recording is unreadable or corrupt; preserve the artifact for investigation and repair it or replace it from a trusted backup before retrying",
			err,
		)
	}
	view := result.WorldState
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 || strings.TrimSpace(view.Payload) == "" {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: Recordings returned an incompatible or empty world-state view; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			nil,
		)
	}
	if view.Scope != scope {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			fmt.Sprintf(
				"CORRUPT_HISTORY: world-state scope %#v does not match %#v; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				view.Scope,
				scope,
			),
			nil,
		)
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: decode world state failed; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			err,
		)
	}
	return &state, nil
}

func currentBoardHistoryFailure(
	recordPath string,
	sessionID string,
	diagnostic string,
	cause error,
) error {
	message := fmt.Sprintf(
		"restore current Factory Session board from %q (session %q): %s",
		recordPath,
		sessionID,
		diagnostic,
	)
	code := currentBoardHistoryFailureCode(diagnostic)
	return &currentBoardHistoryRestoreError{
		code:    code,
		message: message,
		cause:   cause,
	}
}

func currentBoardHistoryFailureCode(diagnostic string) string {
	colon := strings.IndexByte(diagnostic, ':')
	if colon < 0 {
		return currentBoardRecordingFailureCode
	}
	switch strings.TrimSpace(diagnostic[:colon]) {
	case "MISSING_HISTORY":
		return currentBoardRecordingMissingCode
	case "CORRUPT_HISTORY":
		return currentBoardRecordingCorruptCode
	case "UNREADABLE_RECORDING":
		return currentBoardRecordingUnreadableCode
	default:
		return currentBoardRecordingFailureCode
	}
}
