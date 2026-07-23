package session

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func lifecycleControlHumanLine(response factoryapi.FactorySessionLifecycleControlResponse) string {
	detail := lifecycleControlDetail(response)
	operationLabel := strings.ToLower(string(response.Operation))

	switch response.Outcome {
	case factoryapi.FactorySessionLifecycleControlOutcomeAccepted:
		switch response.Operation {
		case factoryapi.FactorySessionLifecycleControlKindPause:
			return fmt.Sprintf("Paused Factory session %s (lifecycle status: %s).", response.SessionId, response.Status)
		case factoryapi.FactorySessionLifecycleControlKindResume:
			return fmt.Sprintf("Resumed Factory session %s (lifecycle status: %s).", response.SessionId, response.Status)
		default:
			return fmt.Sprintf(
				"Factory session %s %s accepted (lifecycle status: %s).",
				response.SessionId,
				operationLabel,
				response.Status,
			)
		}
	case factoryapi.FactorySessionLifecycleControlOutcomeNoOp:
		switch response.Operation {
		case factoryapi.FactorySessionLifecycleControlKindPause:
			return fmt.Sprintf("Factory session %s is already paused.", response.SessionId)
		case factoryapi.FactorySessionLifecycleControlKindResume:
			return fmt.Sprintf("Factory session %s is already running.", response.SessionId)
		default:
			return fmt.Sprintf(
				"Factory session %s already satisfies the requested %s (lifecycle status: %s).",
				response.SessionId,
				operationLabel,
				response.Status,
			)
		}
	case factoryapi.FactorySessionLifecycleControlOutcomeInvalidState:
		line := fmt.Sprintf(
			"Factory session %s cannot be %s while lifecycle status is %s.",
			response.SessionId,
			lifecycleControlPastParticiple(response.Operation),
			response.Status,
		)
		return appendLifecycleControlDetail(line, detail)
	case factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession:
		line := fmt.Sprintf(
			"Factory session %s is terminal and cannot be %s (lifecycle status: %s).",
			response.SessionId,
			operationLabel,
			response.Status,
		)
		return appendLifecycleControlDetail(line, detail)
	case factoryapi.FactorySessionLifecycleControlOutcomeConflict:
		line := fmt.Sprintf(
			"Factory session %s %s request conflicted with the current lifecycle state (%s).",
			response.SessionId,
			operationLabel,
			response.Status,
		)
		return appendLifecycleControlDetail(line, detail)
	default:
		return fmt.Sprintf(
			"Factory session %s %s returned %s (lifecycle status: %s).",
			response.SessionId,
			operationLabel,
			response.Outcome,
			response.Status,
		)
	}
}

func lifecycleControlDetail(response factoryapi.FactorySessionLifecycleControlResponse) string {
	if response.Detail == nil {
		return ""
	}
	return strings.TrimSpace(*response.Detail)
}

func appendLifecycleControlDetail(line, detail string) string {
	if detail == "" {
		return line
	}
	return line + " " + detail
}

func lifecycleControlPastParticiple(operation factoryapi.FactorySessionLifecycleControlKind) string {
	switch operation {
	case factoryapi.FactorySessionLifecycleControlKindPause:
		return "paused"
	case factoryapi.FactorySessionLifecycleControlKindResume:
		return "resumed"
	default:
		return strings.ToLower(string(operation)) + "d"
	}
}
