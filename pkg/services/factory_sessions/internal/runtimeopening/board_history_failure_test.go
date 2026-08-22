package runtimeopening

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRestoreCurrentBoardStateFailsClosedForCorruptHistory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		stub historicalBoardReaderStub
		want string
	}{
		{
			name: "query failure",
			stub: historicalBoardReaderStub{err: errors.New("corrupt canonical history")},
			want: "UNREADABLE_OR_CORRUPT_RECORDING",
		},
		{
			name: "incompatible view",
			stub: historicalBoardReaderStub{result: recordings.HistoricalRecordingQueryResult{
				WorldState: recordings.WorldStateView{SchemaVersion: "unknown", Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}, Payload: "{}"},
			}},
			want: "CORRUPT_HISTORY",
		},
		{
			name: "invalid payload",
			stub: historicalBoardReaderStub{result: recordings.HistoricalRecordingQueryResult{
				WorldState: recordings.WorldStateView{SchemaVersion: recordings.WorldStateViewSchemaV1, Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}, Payload: "not-json"},
			}},
			want: "preserve the artifact",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := restoreCurrentBoardState(&tc.stub, "board.json", "~default", false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("restore error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestRestoreCurrentBoardStateKeepsPresentRecordingFailuresFatalAndActionable(t *testing.T) {
	t.Parallel()

	const (
		recordPath = "board.json"
		sessionID  = "session-corrupt"
	)
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "typed corruption",
			err: &recordings.HistoricalRecordingQueryError{
				Kind:        recordings.HistoricalRecordingQueryErrorCorruptHistory,
				RecordingID: "current-board/session-corrupt",
			},
			want: []string{"CORRUPT_HISTORY", "board.session-corrupt.json", sessionID, "preserve the artifact"},
		},
		{
			name: "typed unreadable artifact",
			err: &recordings.HistoricalRecordingQueryError{
				Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
				RecordingID: "current-board/session-corrupt",
			},
			want: []string{"UNREADABLE_RECORDING", "board.session-corrupt.json", sessionID, "replace it from a trusted backup"},
		},
		{
			name: "missing artifact is fatal when recovery is not authorized",
			err: &recordings.HistoricalRecordingQueryError{
				Kind:        recordings.HistoricalRecordingQueryErrorMissingHistory,
				RecordingID: "current-board/session-corrupt",
			},
			want: []string{"MISSING_HISTORY", "board.session-corrupt.json", sessionID, "preserve the durable snapshot"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := restoreCurrentBoardState(&historicalBoardReaderStub{err: tc.err}, recordPath, sessionID, false)
			if err == nil {
				t.Fatal("restore current board error = nil, want fatal error")
			}
			for _, fragment := range tc.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("restore current board error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}
}

func TestCurrentBoardHistoryFailureExposesSafeCLIClassificationWithoutCauseText(t *testing.T) {
	t.Parallel()

	err := currentBoardHistoryFailure(
		"/recordings/current-board.json",
		"session-safe",
		"CORRUPT_HISTORY: preserve the artifact for investigation and repair before retrying",
		errors.New("work payload must not appear in startup output"),
	)
	var coded interface {
		CLIErrorCode() string
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("current board failure = %T, want safe CLI error contract", err)
	}
	if coded.CLIErrorCode() != currentBoardRecordingCorruptCode {
		t.Fatalf("CLI error code = %q, want %q", coded.CLIErrorCode(), currentBoardRecordingCorruptCode)
	}
	if !strings.Contains(coded.CLIErrorMessage(), "/recordings/current-board.json") ||
		!strings.Contains(coded.CLIErrorMessage(), "preserve the artifact") {
		t.Fatalf("CLI error message = %q, want path and remedy", coded.CLIErrorMessage())
	}
	if strings.Contains(coded.CLIErrorMessage(), "work payload") || strings.Contains(err.Error(), "work payload") {
		t.Fatalf("startup error exposed underlying cause: %q", err)
	}
}

func TestLogCurrentBoardHistoryFailureUsesSafeStructuredContext(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	path := filepath.Join(t.TempDir(), "current-board.json")
	err := currentBoardHistoryFailure(
		path,
		"session-safe",
		"CORRUPT_HISTORY: preserve the artifact for investigation and repair before retrying",
		errors.New("malformed recording"),
	)
	logCurrentBoardHistoryFailure(logger, "session-safe", path, err)

	if logs.Len() != 1 {
		t.Fatalf("logged failure count = %d, want 1", logs.Len())
	}
	entry := logs.All()[0]
	fields := entry.ContextMap()
	if entry.Message != "current Factory Session board recording could not be restored; startup aborted" {
		t.Fatalf("log message = %q", entry.Message)
	}
	if fields["session_id"] != "session-safe" || fields["recording_path"] != path || fields["failure"] != "UNREADABLE_OR_CORRUPT_RECORDING" {
		t.Fatalf("log context = %#v", fields)
	}
	if strings.Contains(fmt.Sprint(fields), "malformed recording") {
		t.Fatal("log unexpectedly included the underlying recording failure text")
	}
}
