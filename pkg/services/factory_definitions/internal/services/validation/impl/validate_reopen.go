package impl

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func workStateReopenTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	var targets []Target
	for typeIndex, workType := range cfg.WorkTypes {
		for stateIndex, state := range workType.States {
			if reason := stateReopenReason(state, workType.States); reason != "" {
				targets = append(targets, Target{
					Code: CodeWorkStateInvalidReopen, Severity: SeverityError,
					Message: "invalid onReopen: " + reason,
					Path:    fmt.Sprintf("%s.workTypes[%d].states[%d].onReopen", validationRoot, typeIndex, stateIndex),
					Subject: Subject{Type: SubjectTypeWorkState,
						ID:       factorydefinitions.CanonicalFactoryGraphWorkTypeID(workType) + ":" + state.Name,
						Location: SubjectLocationStates},
				})
			}
		}
	}
	return targets
}

func stateReopenReason(state factorydefinitions.StateConfig, states []factorydefinitions.StateConfig) string {
	rule := state.OnReopen
	if rule == nil {
		return ""
	}
	if state.Type != factorydefinitions.StateTypeProcessing {
		return "source state must be PROCESSING"
	}
	if rule.MaxWaits < 1 {
		return "maxWaits must be positive"
	}
	if strings.TrimSpace(rule.State) == "" || strings.TrimSpace(rule.ExhaustedState) == "" {
		return "state and exhaustedState must name destinations"
	}
	if rule.State == state.Name || rule.ExhaustedState == state.Name || rule.State == rule.ExhaustedState {
		return "source and destinations must be distinct"
	}
	destinations := make(map[string]factorydefinitions.StateType, len(states))
	for _, destination := range states {
		destinations[destination.Name] = destination.Type
	}
	if destinations[rule.State] != factorydefinitions.StateTypeProcessing {
		return "state must name a PROCESSING state in the same work type"
	}
	if destinations[rule.ExhaustedState] != factorydefinitions.StateTypeFailed {
		return "exhaustedState must name a FAILED state in the same work type"
	}
	return ""
}
