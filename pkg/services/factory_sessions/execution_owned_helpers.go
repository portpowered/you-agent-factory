package factorysessions

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// IsTerminalLifecycleStatus reports whether status is terminal and therefore
// immutable except for explicitly allowed inspection or retry behaviors.
func IsTerminalLifecycleStatus(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated:
		return true
	default:
		return false
	}
}

// AllowsRetryDispatchOnTerminal reports whether retry-dispatch remains permitted
// after the session reaches a terminal status. Failed sessions may still accept
// retry-dispatch for failed child dispatches.
func AllowsRetryDispatchOnTerminal(status LifecycleStatus) bool {
	return status == LifecycleStatusFailed
}

// AllowsInterruptDispatchOnSession reports whether interrupt-dispatch remains
// permitted while the session is actively running goal work.
func AllowsInterruptDispatchOnSession(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
		return true
	default:
		return false
	}
}

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func InspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session:    base,
		Status:     base,
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}

// EmptySessionUsage returns the stable zero usage projection for sessions without
// runtime consumption data.
func EmptySessionUsage() SessionUsage {
	return SessionUsage{Resources: []ResourceUsage{}}
}

// EvaluateLifecycleControl classifies one lifecycle control request against the
// current durable session status without runtime-specific dispatch context.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this transition classifier keeps durable lifecycle control outcomes explicit across terminal and active states.
func EvaluateLifecycleControl(operation LifecycleControlKind, status LifecycleStatus) LifecycleControlOutcome {
	if status == "" {
		return LifecycleControlOutcomeInvalidState
	}
	if status == LifecycleStatusInterrupted && operation == LifecycleControlResume {
		return LifecycleControlOutcomeAccepted
	}
	if IsTerminalLifecycleStatus(status) {
		switch operation {
		case LifecycleControlRetryDispatch:
			if status == LifecycleStatusFailed {
				return LifecycleControlOutcomeAccepted
			}
			return LifecycleControlOutcomeTerminalSession
		case LifecycleControlCancel, LifecycleControlTerminate:
			if status == LifecycleStatusCanceled && operation == LifecycleControlCancel {
				return LifecycleControlOutcomeNoOp
			}
			if status == LifecycleStatusTerminated && operation == LifecycleControlTerminate {
				return LifecycleControlOutcomeNoOp
			}
			return LifecycleControlOutcomeTerminalSession
		default:
			return LifecycleControlOutcomeTerminalSession
		}
	}

	switch operation {
	case LifecycleControlPause:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlResume:
		switch status {
		case LifecycleStatusPaused, LifecycleStatusInterrupted:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusResuming, LifecycleStatusRunning:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlCancel:
		switch status {
		case LifecycleStatusCanceling:
			return LifecycleControlOutcomeNoOp
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlTerminate:
		switch status {
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming,
			LifecycleStatusCanceling:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlApprove:
		if status == LifecycleStatusAwaitingApproval {
			return LifecycleControlOutcomeAccepted
		}
		return LifecycleControlOutcomeInvalidState
	case LifecycleControlRetryDispatch:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	default:
		return LifecycleControlOutcomeInvalidState
	}
}

// LifecycleControlLinksForSession builds post-control inspection links for one durable session.
func LifecycleControlLinksForSession(sessionID string, includeEvents bool) LifecycleControlLinks {
	inspection := InspectionLinksForSession(sessionID, includeEvents)
	return LifecycleControlLinks{
		Session:    inspection.Session,
		Status:     inspection.Status,
		Results:    inspection.Results,
		Dispatches: inspection.Dispatches,
		Artifacts:  inspection.Artifacts,
		Events:     inspection.Events,
	}
}

// LifecycleControlOutcomeClass normalizes lifecycle outcomes and errors into
// the stable low-cardinality class used by logs and metrics.
func LifecycleControlOutcomeClass(outcome LifecycleControlOutcome, err error) string {
	if err != nil {
		if errors.Is(err, ErrDurableSessionNotFound) {
			return LifecycleControlOutcomeClassNotFound
		}
		var controlErr *ControlError
		if errors.As(err, &controlErr) {
			return string(controlErr.Outcome)
		}
		return "ERROR"
	}
	if outcome == "" {
		return "ERROR"
	}
	return string(outcome)
}

// LifecycleStatusFromFactoryRuntimeState maps one live Petri factory runtime state
// into the shared Factory Session lifecycle vocabulary used by control surfaces.
func LifecycleStatusFromFactoryRuntimeState(factoryState string) LifecycleStatus {
	switch strings.ToUpper(strings.TrimSpace(factoryState)) {
	case "RUNNING", "IDLE":
		return LifecycleStatusRunning
	case "PAUSED":
		return LifecycleStatusPaused
	case "COMPLETED":
		return LifecycleStatusSucceeded
	case "FAILED":
		return LifecycleStatusFailed
	default:
		return ""
	}
}

// LiveLifecycleControlLinksForSession builds post-control inspection links for
// one live workspace Factory Session.
func LiveLifecycleControlLinksForSession(sessionID string) LifecycleControlLinks {
	base := fmt.Sprintf("/factory-sessions/%s", strings.TrimSpace(sessionID))
	return LifecycleControlLinks{
		Session: base,
		Status:  base,
		Results: base + "/result",
		Events:  base + "/events",
	}
}

// LiveLifecycleControlLogFields returns the canonical structured fields for a
// live-session lifecycle control observation.
func LiveLifecycleControlLogFields(sessionID string, operation LifecycleControlKind, outcomeClass string, status LifecycleStatus, control ControlRequest) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if control.RequestID != "" {
		fields = append(fields, zap.String("request_id", control.RequestID))
	}
	return fields
}

// MaterializeEventReadStream owns the finite stream lifecycle for one durable
// event read. Transports receive an already-closed live channel plus detached
// canonical history and do not manufacture channel-backed streams.
func MaterializeEventReadStream(result EventReadResult) *interfaces.FactoryEventStream {
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	stream := &interfaces.FactoryEventStream{Events: closed}
	for _, raw := range result.Events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		stream.History = append(stream.History, event)
	}
	return stream
}

// NewValidationError constructs one field-scoped validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
