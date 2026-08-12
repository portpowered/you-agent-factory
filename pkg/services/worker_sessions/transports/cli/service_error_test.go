package cli

import (
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestMapInvokeServiceErrorPreservesRemoteClassificationInSyncAndAsyncModes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "invalid", err: workersessions.ErrInvalidExecutionRequest, code: "WORKER_SESSION_INVOKE_INVALID"},
		{name: "request id conflict", err: workersessions.ErrStartRequestIDConflict, code: "WORKER_SESSION_START_REQUEST_ID_CONFLICT"},
		{name: "identity conflict", err: workersessions.ErrSessionNotStartable, code: "WORKER_SESSION_NOT_STARTABLE"},
		{name: "admission", err: workersessions.ErrStartAdmissionFailed, code: "WORKER_SESSION_ADMISSION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, async := range []bool{false, true} {
				t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
					got := mapInvokeServiceError(test.err, async)
					assertCLIErrorCode(t, got, test.code)
				})
			}
		})
	}
}

func TestMapContinueServiceErrorPreservesRemoteClassificationInSyncAndAsyncModes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "unknown source", err: workersessions.ErrContinuationSourceNotFound, code: "NOT_FOUND"},
		{name: "active source", err: workersessions.ErrContinuationSourceActive, code: "WORKER_SESSION_CONTINUATION_CONFLICT"},
		{name: "request id conflict", err: workersessions.ErrContinuationRequestIDConflict, code: "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT"},
		{name: "successor conflict", err: workersessions.ErrContinuationSuccessorConflict, code: "WORKER_SESSION_CONTINUATION_CONFLICT"},
		{name: "admission", err: workersessions.ErrContinuationNotAccepted, code: "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, async := range []bool{false, true} {
				t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
					got := mapContinueServiceError(test.err, async)
					assertCLIErrorCode(t, got, test.code)
				})
			}
		})
	}
}

func assertCLIErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error = %v, want CLIError(%s)", err, want)
	}
	if cliErr.Code != want {
		t.Fatalf("CLI error code = %q, want %q", cliErr.Code, want)
	}
}
