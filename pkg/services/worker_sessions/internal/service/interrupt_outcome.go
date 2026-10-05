package service

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// This persistence representation extends the existing detached result with
// allowlisted error identities. Error messages and provider diagnostics never
// enter the journal. Older results without codes retain their phase-only replay.
type durableInterruptOutcome struct {
	workersessions.InterruptResult
	FailureCauses []string `json:"failureCauses,omitempty"`
}

type interruptFailureIdentity struct {
	code  string
	cause error
}

func interruptFailureIdentities() []interruptFailureIdentity {
	return []interruptFailureIdentity{
		{"SOURCE_CONFLICT", workersessions.ErrInterruptSourceConflict},
		{"EXECUTION_UNAVAILABLE", workersessions.ErrInterruptExecutionUnavailable},
		{"SOURCE_CANCELLATION_FAILED", workersessions.ErrInterruptSourceCancellationFailed},
		{"SUCCESSOR_ADMISSION_FAILED", workersessions.ErrInterruptSuccessorAdmissionFailed},
		{"PERSISTENCE_UNAVAILABLE", recordings.ErrWorkerRecordingPersistence},
		{"UNSAFE_INPUT", recordings.ErrInvalidRecordingRedactionRequest},
		{"CONTINUATION_SOURCE_NOT_FOUND", workersessions.ErrContinuationSourceNotFound},
		{"CONTINUATION_SOURCE_ACTIVE", workersessions.ErrContinuationSourceActive},
		{"CONTINUATION_REFERENCE_MISSING", workersessions.ErrContinuationProviderSessionMissing},
		{"CONTINUATION_REFERENCE_INVALID", workersessions.ErrContinuationProviderSessionInvalid},
		{"CONTINUATION_SOURCE_CONFLICT", workersessions.ErrContinuationSourceConflict},
		{"CONTINUATION_REQUEST_CONFLICT", workersessions.ErrContinuationRequestIDConflict},
		{"CONTINUATION_SUCCESSOR_CONFLICT", workersessions.ErrContinuationSuccessorConflict},
		{"CONTINUATION_EXECUTION_UNAVAILABLE", workersessions.ErrContinuationExecutionUnavailable},
		{"CONTINUATION_NOT_ACCEPTED", workersessions.ErrContinuationNotAccepted},
		{"CONTINUATION_SERVER_STOPPING", workersessions.ErrContinuationServerStopping},
		{"DISPATCH_UNKNOWN", workers.ErrUnknownWorkstationDispatch},
		{"DISPATCH_ALREADY_TERMINAL", workers.ErrWorkstationDispatchAlreadyTerminal},
	}
}

func interruptFailureCodes(err error) []string {
	var codes []string
	for _, identity := range interruptFailureIdentities() {
		if errors.Is(err, identity.cause) {
			codes = append(codes, identity.code)
		}
	}
	return codes
}

func interruptFailureCause(codes []string) (error, error) {
	var causes []error
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		cause := interruptFailureForCode(code)
		if cause == nil || seen[code] {
			return nil, recordings.ErrWorkerRecordingPersistence
		}
		seen[code] = true
		causes = append(causes, cause)
	}
	return errors.Join(causes...), nil
}

func interruptFailureForCode(code string) error {
	for _, identity := range interruptFailureIdentities() {
		if identity.code == code {
			return identity.cause
		}
	}
	return nil
}

func validInterruptOutcome(req workersessions.InterruptRequest, result workersessions.InterruptResult) bool {
	if !validInterruptResultIdentity(req, result) || !validInterruptSessionFacts(result.Source) || !validInterruptSessionFacts(result.Successor) {
		return false
	}
	if result.Successor.ID == "" {
		if result.Successor.State != "" || result.Accepted {
			return false
		}
	} else if result.Successor.ID != req.SuccessorWorkerSessionID || !result.Successor.State.Valid() {
		return false
	}
	switch result.Phase {
	case workersessions.InterruptPhaseValidation, workersessions.InterruptPhaseSourceCancellation:
		return !result.Accepted
	case workersessions.InterruptPhaseSuccessorAdmission:
		return result.Source.State == workersessions.StateCanceled && (!result.Accepted || interruptSuccessorAdmittedState(result.Successor.State))
	default:
		return false
	}
}

// The reader enforces the same privacy boundary as the writer. A malformed
// journal must not turn provider content or private diagnostics into a reply.
func validInterruptSessionFacts(session workersessions.Session) bool {
	return session.Model == nil && session.ReasoningEffort == nil &&
		session.Result == nil && session.ProviderSessionAssociation == nil
}

func validInterruptResultIdentity(req workersessions.InterruptRequest, result workersessions.InterruptResult) bool {
	return result.RequestID == req.RequestID && result.SourceWorkerSessionID == req.SourceWorkerSessionID &&
		result.SuccessorWorkerSessionID == req.SuccessorWorkerSessionID && result.Source.ID == req.SourceWorkerSessionID &&
		result.Source.State.Valid() && validInterruptLineage(req, result)
}

func validInterruptLineage(req workersessions.InterruptRequest, result workersessions.InterruptResult) bool {
	source, successor := result.Source, result.Successor
	if source.PredecessorWorkerSessionID == source.ID || source.PredecessorWorkerSessionID == req.SuccessorWorkerSessionID {
		return false
	}
	if source.SuccessorWorkerSessionID != "" && source.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID {
		return false
	}
	if successor.PredecessorWorkerSessionID != "" && successor.PredecessorWorkerSessionID != req.SourceWorkerSessionID {
		return false
	}
	if successor.SuccessorWorkerSessionID != "" && (successor.SuccessorWorkerSessionID == successor.ID || successor.SuccessorWorkerSessionID == req.SourceWorkerSessionID) {
		return false
	}
	if successor.ID == "" && (successor.PredecessorWorkerSessionID != "" || successor.SuccessorWorkerSessionID != "") {
		return false
	}
	return true
}
