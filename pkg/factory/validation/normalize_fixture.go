package validation

import "github.com/portpowered/infinite-you/pkg/interfaces"

// NormalizeFixtureConfig augments programmatic test fixtures with the minimum
// structural fields required by canonical validation without changing mapping intent.
func NormalizeFixtureConfig(cfg *interfaces.FactoryConfig) {
	if cfg == nil {
		return
	}
	referencedWorkTypes := map[string]bool{}
	for _, workstation := range cfg.Workstations {
		collectReferencedWorkTypes(workstation.Inputs, referencedWorkTypes)
		collectReferencedWorkTypes(workstation.Outputs, referencedWorkTypes)
		collectReferencedWorkTypes(workstation.OnContinue, referencedWorkTypes)
		collectReferencedWorkTypes(workstation.OnRejection, referencedWorkTypes)
		collectReferencedWorkTypes(workstation.OnFailure, referencedWorkTypes)
		for _, route := range workstation.ClassificationRoutes {
			collectReferencedWorkTypes(route.Outputs, referencedWorkTypes)
		}
	}
	for workTypeIndex, workType := range cfg.WorkTypes {
		if !referencedWorkTypes[workType.Name] {
			continue
		}
		cfg.WorkTypes[workTypeIndex].States = ensureOutcomeStates(workType.States)
	}
	for workstationIndex, workstation := range cfg.Workstations {
		if workstationHasFailureRoute(workstation, cfg) {
			continue
		}
		if routes := defaultFailureRoutesForWorkstation(workstation, cfg); len(routes) > 0 {
			cfg.Workstations[workstationIndex].OnFailure = routes
		}
	}
	ensureTerminalReachability(cfg)
}

func collectReferencedWorkTypes(routes []interfaces.IOConfig, referenced map[string]bool) {
	for _, route := range routes {
		if route.WorkTypeName != "" {
			referenced[route.WorkTypeName] = true
		}
	}
}

func ensureOutcomeStates(states []interfaces.StateConfig) []interfaces.StateConfig {
	hasCompletion := false
	hasFailure := false
	for _, state := range states {
		switch state.Type {
		case interfaces.StateTypeTerminal:
			hasCompletion = true
		case interfaces.StateTypeFailed:
			hasFailure = true
		}
	}
	if hasCompletion && hasFailure {
		return states
	}
	next := append([]interfaces.StateConfig(nil), states...)
	if !hasCompletion {
		next = append(next, interfaces.StateConfig{Name: "complete", Type: interfaces.StateTypeTerminal})
	}
	if !hasFailure {
		next = append(next, interfaces.StateConfig{Name: "failed", Type: interfaces.StateTypeFailed})
	}
	return next
}

func workstationHasFailureRoute(workstation interfaces.FactoryWorkstationConfig, cfg *interfaces.FactoryConfig) bool {
	if len(workstation.OnFailure) > 0 {
		return true
	}
	failedWorkTypes := failedWorkTypesForConfig(cfg)
	for _, input := range workstation.Inputs {
		if failedWorkTypes[input.WorkTypeName] {
			return true
		}
	}
	for _, output := range workstation.Outputs {
		if failedWorkTypes[output.WorkTypeName] {
			return true
		}
	}
	for _, route := range workstation.ClassificationRoutes {
		for _, output := range route.Outputs {
			if failedWorkTypes[output.WorkTypeName] {
				return true
			}
		}
	}
	return false
}

func defaultFailureRoutesForWorkstation(
	workstation interfaces.FactoryWorkstationConfig,
	cfg *interfaces.FactoryConfig,
) []interfaces.IOConfig {
	failedWorkTypes := failedWorkTypesForConfig(cfg)
	candidates := make([]interfaces.IOConfig, 0, len(workstation.Inputs)+len(workstation.Outputs))
	candidates = append(candidates, workstation.Inputs...)
	candidates = append(candidates, workstation.Outputs...)
	for _, route := range workstation.ClassificationRoutes {
		candidates = append(candidates, route.Outputs...)
	}
	for _, candidate := range candidates {
		if !failedWorkTypes[candidate.WorkTypeName] {
			continue
		}
		if failedState, ok := failedStateForWorkType(cfg, candidate.WorkTypeName); ok {
			return []interfaces.IOConfig{{
				WorkTypeName: candidate.WorkTypeName,
				StateName:    failedState,
			}}
		}
	}
	return nil
}

func failedWorkTypesForConfig(cfg *interfaces.FactoryConfig) map[string]bool {
	failed := map[string]bool{}
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			if state.Type == interfaces.StateTypeFailed {
				failed[workType.Name] = true
				break
			}
		}
	}
	return failed
}

func failedStateForWorkType(cfg *interfaces.FactoryConfig, workTypeName string) (string, bool) {
	for _, workType := range cfg.WorkTypes {
		if workType.Name != workTypeName {
			continue
		}
		for _, state := range workType.States {
			if state.Type == interfaces.StateTypeFailed {
				return state.Name, true
			}
		}
	}
	return "", false
}

func ensureTerminalReachability(cfg *interfaces.FactoryConfig) {
	adj := buildFixtureWorkStateAdjacency(cfg)
	terminalStates := terminalStateSet(cfg)
	for workstationIndex, workstation := range cfg.Workstations {
		for _, input := range workstation.Inputs {
			stateID := placeKey(input.WorkTypeName, input.StateName)
			if terminalStates[stateID] || canReachTerminalState(stateID, terminalStates, adj) {
				continue
			}
			terminalState, ok := firstTerminalStateForWorkType(cfg, input.WorkTypeName)
			if !ok {
				continue
			}
			cfg.Workstations[workstationIndex].OnContinue = append(
				cfg.Workstations[workstationIndex].OnContinue,
				interfaces.IOConfig{WorkTypeName: input.WorkTypeName, StateName: terminalState},
			)
			adj = buildFixtureWorkStateAdjacency(cfg)
		}
	}
}

func buildFixtureWorkStateAdjacency(cfg *interfaces.FactoryConfig) map[string]map[string]bool {
	adj := map[string]map[string]bool{}
	addEdge := func(from, to string) {
		if from == "" || to == "" {
			return
		}
		if adj[from] == nil {
			adj[from] = map[string]bool{}
		}
		adj[from][to] = true
	}
	connect := func(inputs, routes []interfaces.IOConfig) {
		inputPlaces := make([]string, 0, len(inputs))
		for _, input := range inputs {
			inputPlaces = append(inputPlaces, placeKey(input.WorkTypeName, input.StateName))
		}
		for _, route := range routes {
			outputPlace := placeKey(route.WorkTypeName, route.StateName)
			for _, inputPlace := range inputPlaces {
				addEdge(inputPlace, outputPlace)
			}
		}
	}
	for _, workstation := range cfg.Workstations {
		connect(workstation.Inputs, workstation.Outputs)
		connect(workstation.Inputs, workstation.OnContinue)
		connect(workstation.Inputs, workstation.OnRejection)
		connect(workstation.Inputs, workstation.OnFailure)
		for _, route := range workstation.ClassificationRoutes {
			connect(workstation.Inputs, route.Outputs)
		}
	}
	return adj
}

func firstTerminalStateForWorkType(cfg *interfaces.FactoryConfig, workTypeName string) (string, bool) {
	for _, workType := range cfg.WorkTypes {
		if workType.Name != workTypeName {
			continue
		}
		for _, state := range workType.States {
			if state.Type == interfaces.StateTypeTerminal {
				return state.Name, true
			}
		}
	}
	return "", false
}
