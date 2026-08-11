package packagedfactorycatalog

import (
	"fmt"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const invocationScheduleOverlapSkipBridge = "automations.invocation_schedule.overlap_skip"

// ValidateFirstPartyWorkStateRoles verifies the semantic role inventory for a
// parsed first-party Petri Factory. It is intentionally owned by the packaged
// catalog instead of the general Factory validator: customer-authored and
// JavaScript-controlled Factories are not subject to the first-party catalog
// publication rule.
func ValidateFirstPartyWorkStateRoles(
	slug string,
	cfg *factorydefinitions.FactoryConfig,
) error {
	if cfg == nil || !factorydefinitions.IsPetriOrchestratorFactory(cfg) {
		return nil
	}

	used := referencedWorkStates(cfg)
	var disconnected []string
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			identity := workStateIdentity(workType.Name, state.Name)
			_, referenced := used[identity]
			if referenced || lifecycleBridgeName(cfg, workType.Name, state.Name) != "" {
				continue
			}
			disconnected = append(disconnected, identity)
		}
	}
	if len(disconnected) == 0 {
		return nil
	}
	sort.Strings(disconnected)
	return fmt.Errorf(
		"packaged Factory %q declares disconnected Work state(s): %s",
		slug,
		strings.Join(disconnected, ", "),
	)
}

func referencedWorkStates(cfg *factorydefinitions.FactoryConfig) map[string]struct{} {
	used := make(map[string]struct{})
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			if state.Type == factorydefinitions.StateTypeInitial {
				used[workStateIdentity(workType.Name, state.Name)] = struct{}{}
			}
		}
	}
	for _, workstation := range cfg.Workstations {
		addIOStates(used, workstation.Inputs)
		addIOStates(used, workstation.Outputs)
		addIOStates(used, workstation.OnContinue)
		addIOStates(used, workstation.OnRejection)
		addIOStates(used, workstation.OnFailure)
		for _, route := range workstation.ClassificationRoutes {
			addIOStates(used, route.Outputs)
		}
	}
	if cfg.InvocationReturn != nil &&
		cfg.InvocationReturn.Policy == factorydefinitions.InvocationReturnPolicyExplicit {
		used[workStateIdentity(cfg.InvocationReturn.WorkTypeName, cfg.InvocationReturn.TerminalState)] = struct{}{}
	}
	return used
}

func addIOStates(used map[string]struct{}, routes []factorydefinitions.IOConfig) {
	for _, route := range routes {
		used[workStateIdentity(route.WorkTypeName, route.StateName)] = struct{}{}
	}
}

func lifecycleBridgeName(
	cfg *factorydefinitions.FactoryConfig,
	workTypeName string,
	stateName string,
) string {
	if stateName != "skipped" {
		return ""
	}
	if !invocationScheduleOverlapSkipImplemented(cfg, workTypeName) {
		return ""
	}
	return invocationScheduleOverlapSkipBridge
}

// invocationScheduleOverlapSkipImplemented mirrors the production automation
// contract: a CRON workstation with a non-controller output creates a separate
// scheduled Work item, and the scheduler uses that item's terminal "skipped"
// state when an earlier execution is still active. The named bridge is backed
// by the direct fake-clock overlap test in pkg/services/automations/internal.
func invocationScheduleOverlapSkipImplemented(
	cfg *factorydefinitions.FactoryConfig,
	workTypeName string,
) bool {
	for _, workstation := range cfg.Workstations {
		if workstation.Kind != factorydefinitions.WorkstationKindCron || workstation.Cron == nil {
			continue
		}
		controllerTypes := make(map[string]struct{}, len(workstation.Inputs))
		for _, input := range workstation.Inputs {
			controllerTypes[input.WorkTypeName] = struct{}{}
		}
		for _, output := range workstation.Outputs {
			if output.WorkTypeName != workTypeName {
				continue
			}
			if _, isControllerOutput := controllerTypes[output.WorkTypeName]; isControllerOutput {
				continue
			}
			return true
		}
	}
	return false
}

func workStateIdentity(workTypeName, stateName string) string {
	return strings.TrimSpace(workTypeName) + ":" + strings.TrimSpace(stateName)
}
