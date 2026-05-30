package validation

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func duplicateIdentifierTargets(cfg *interfaces.FactoryConfig) []Target {
	var targets []Target
	targets = append(targets, duplicateNameTargets(cfg, "workTypes", workTypeNames(cfg), SubjectTypeWorkType)...)
	targets = append(targets, duplicateNameTargets(cfg, "workers", workerNames(cfg), SubjectTypeWorker)...)
	targets = append(targets, duplicateNameTargets(cfg, "resources", resourceNames(cfg), SubjectTypeResource)...)
	targets = append(targets, duplicateNameTargets(cfg, "workstations", workstationNames(cfg), SubjectTypeWorkstation)...)
	return targets
}

func duplicateNameTargets(cfg *interfaces.FactoryConfig, collection string, names []string, subjectType SubjectType) []Target {
	seen := make(map[string]int, len(names))
	var targets []Target
	for index, name := range names {
		path := fmt.Sprintf("%s.%s[%d].name", validationRoot, collection, index)
		if strings.TrimSpace(name) == "" {
			targets = append(targets, Target{
				Code:     CodeDuplicateIdentifier,
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s name must be non-empty.", collection),
				Subject: Subject{
					Type:     subjectType,
					ID:       "",
					Location: SubjectLocationDefinition,
				},
				Path: path,
			})
			continue
		}
		if firstIndex, ok := seen[name]; ok {
			firstPath := fmt.Sprintf("%s.%s[%d].name", validationRoot, collection, firstIndex)
			message := fmt.Sprintf("duplicate %s name %q.", singularCollectionLabel(collection), name)
			targets = append(targets,
				duplicateIdentifierTarget(subjectType, name, firstPath, message),
				duplicateIdentifierTarget(subjectType, name, path, message),
			)
			continue
		}
		seen[name] = index
	}
	return targets
}

func singularCollectionLabel(collection string) string {
	switch collection {
	case "workTypes":
		return "work type"
	case "workstations":
		return "workstation"
	case "workers":
		return "worker"
	case "resources":
		return "resource"
	default:
		return collection
	}
}

func duplicateIdentifierTarget(subjectType SubjectType, id, path, message string) Target {
	return Target{
		Code:     CodeDuplicateIdentifier,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     subjectType,
			ID:       id,
			Location: SubjectLocationDefinition,
		},
		Path: path,
	}
}

func duplicateWorkStateTargets(cfg *interfaces.FactoryConfig) []Target {
	var targets []Target
	for workTypeIndex, workType := range cfg.WorkTypes {
		seen := make(map[string]int, len(workType.States))
		for stateIndex, state := range workType.States {
			path := fmt.Sprintf("%s.workTypes[%d].states[%d].name", validationRoot, workTypeIndex, stateIndex)
			if strings.TrimSpace(state.Name) == "" {
				targets = append(targets, Target{
					Code:     CodeDuplicateIdentifier,
					Severity: SeverityError,
					Message:  fmt.Sprintf("work type %q state name must be non-empty.", workType.Name),
					Subject: Subject{
						Type:     SubjectTypeWorkType,
						ID:       workType.Name,
						Location: SubjectLocationStates,
					},
					Path: path,
				})
				continue
			}
			if firstIndex, ok := seen[state.Name]; ok {
				stateID := workType.Name + ":" + state.Name
				message := fmt.Sprintf("duplicate work state name %q on work type %q.", state.Name, workType.Name)
				targets = append(targets,
					duplicateWorkStateTarget(workType.Name, stateID, fmt.Sprintf("%s.workTypes[%d].states[%d].name", validationRoot, workTypeIndex, firstIndex), message),
					duplicateWorkStateTarget(workType.Name, stateID, path, message),
				)
				continue
			}
			seen[state.Name] = stateIndex
		}
	}
	return targets
}

func duplicateWorkStateTarget(workTypeName, stateID, path, message string) Target {
	return Target{
		Code:     CodeDuplicateIdentifier,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeWorkState,
			ID:       stateID,
			Location: SubjectLocationStates,
		},
		Path: path,
	}
}

func danglingReferenceTargets(cfg *interfaces.FactoryConfig) []Target {
	workers := stringSet(workerNames(cfg))
	resources := stringSet(resourceNames(cfg))
	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if strings.TrimSpace(workstation.WorkerTypeName) != "" && !workers[workstation.WorkerTypeName] {
			targets = append(targets, Target{
				Code:     CodeDanglingWorkerReference,
				Severity: SeverityError,
				Message:  fmt.Sprintf("workstation %q references non-existent worker %q.", workstation.Name, workstation.WorkerTypeName),
				Subject: Subject{
					Type:     SubjectTypeWorkstation,
					ID:       workstation.Name,
					Location: SubjectLocationReference,
				},
				Path: fmt.Sprintf("%s.workstations[%d].worker", validationRoot, workstationIndex),
			})
		}
		for resourceIndex, resource := range workstation.Resources {
			if strings.TrimSpace(resource.Name) == "" || !resources[resource.Name] {
				targets = append(targets, Target{
					Code:     CodeDanglingResourceReference,
					Severity: SeverityError,
					Message:  fmt.Sprintf("workstation %q references non-existent resource %q.", workstation.Name, resource.Name),
					Subject: Subject{
						Type:     SubjectTypeResource,
						ID:       resource.Name,
						Location: SubjectLocationReference,
					},
					Path: fmt.Sprintf("%s.workstations[%d].resources[%d].name", validationRoot, workstationIndex, resourceIndex),
				})
			}
		}
	}
	return targets
}

func invalidPlaceReferenceTargets(cfg *interfaces.FactoryConfig) []Target {
	validPlaces := buildValidPlaces(cfg)
	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		targets = append(targets, invalidPlaceIOTargets(workstation, workstation.Inputs, validPlaces, workstationIndex, "inputs", SubjectLocationInputs)...)
		targets = append(targets, invalidPlaceIOTargets(workstation, workstation.Outputs, validPlaces, workstationIndex, "outputs", SubjectLocationOutputs)...)
		targets = append(targets, invalidPlaceIOTargets(workstation, workstation.OnContinue, validPlaces, workstationIndex, "on_continue", SubjectLocationOutputs)...)
		targets = append(targets, invalidPlaceIOTargets(workstation, workstation.OnRejection, validPlaces, workstationIndex, "on_rejection", SubjectLocationOnRejection)...)
		targets = append(targets, invalidPlaceIOTargets(workstation, workstation.OnFailure, validPlaces, workstationIndex, "on_failure", SubjectLocationOnFailure)...)
		for routeIndex, route := range workstation.ClassificationRoutes {
			targets = append(targets, invalidPlaceIOTargets(
				workstation,
				route.Outputs,
				validPlaces,
				workstationIndex,
				fmt.Sprintf("classification_routes[%d].outputs", routeIndex),
				SubjectLocationOutputs,
			)...)
		}
	}
	return targets
}

func invalidPlaceIOTargets(
	workstation interfaces.FactoryWorkstationConfig,
	ios []interfaces.IOConfig,
	validPlaces map[string]bool,
	workstationIndex int,
	routeField string,
	location SubjectLocation,
) []Target {
	if len(ios) == 0 {
		return nil
	}
	var targets []Target
	for ioIndex, io := range ios {
		placeID := placeKey(io.WorkTypeName, io.StateName)
		if validPlaces[placeID] {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeDanglingPlaceReference,
			Severity: SeverityError,
			Message:  fmt.Sprintf("references non-existent state %q of work type %q", io.StateName, io.WorkTypeName),
			Subject: Subject{
				Type:     SubjectTypeRoute,
				ID:       workstation.Name + "->" + placeID,
				Location: location,
			},
			Path: fmt.Sprintf("%s.workstations[%d].%s[%d]", validationRoot, workstationIndex, apiRouteField(routeField), ioIndex),
		})
	}
	return targets
}

func conflictingWorkstationOutputTargets(cfg *interfaces.FactoryConfig) []Target {
	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		inputCounts := ioWorkTypeCounts(workstation.Inputs)
		if len(inputCounts) == 0 {
			continue
		}
		targets = append(targets, conflictingRouteTargets(workstation.Name, inputCounts, workstation.Outputs, fmt.Sprintf("%s.workstations[%d].outputs", validationRoot, workstationIndex))...)
		targets = append(targets, conflictingRouteTargets(workstation.Name, inputCounts, workstation.OnContinue, fmt.Sprintf("%s.workstations[%d].onContinue", validationRoot, workstationIndex))...)
		targets = append(targets, conflictingRouteTargets(workstation.Name, inputCounts, workstation.OnRejection, fmt.Sprintf("%s.workstations[%d].onRejection", validationRoot, workstationIndex))...)
		targets = append(targets, conflictingRouteTargets(workstation.Name, inputCounts, workstation.OnFailure, fmt.Sprintf("%s.workstations[%d].onFailure", validationRoot, workstationIndex))...)
	}
	return targets
}

func conflictingRouteTargets(workstation string, inputCounts map[string]int, routes []interfaces.IOConfig, fieldPrefix string) []Target {
	routeCounts := ioWorkTypeCounts(routes)
	var targets []Target
	for workType, inputCount := range inputCounts {
		routeCount := routeCounts[workType]
		if routeCount == 0 || routeCount == inputCount {
			continue
		}
		for routeIndex, route := range routes {
			if route.WorkTypeName != workType {
				continue
			}
			targets = append(targets, Target{
				Code:     CodeWorkstationConflictingOutputs,
				Severity: SeverityError,
				Message:  fmt.Sprintf("workstation %q routes work type %q to conflicting output states.", workstation, workType),
				Subject: Subject{
					Type:     SubjectTypeWorkstation,
					ID:       workstation,
					Location: SubjectLocationOutputs,
				},
				Path: fmt.Sprintf("%s[%d]", fieldPrefix, routeIndex),
			})
		}
	}
	return targets
}

func missingOutcomeRouteTargets(cfg *interfaces.FactoryConfig) []Target {
	failedWorkTypes := failedWorkTypeSet(cfg)
	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		hasFailureRoute := len(workstation.OnFailure) > 0 ||
			workstationCanDefaultFailureRoute(workstation, failedWorkTypes)
		if !hasFailureRoute {
			targets = append(targets, Target{
				Code:     CodeWorkstationMissingFailureRoute,
				Severity: SeverityError,
				Message:  fmt.Sprintf("workstation %q must define a failure route.", workstation.Name),
				Subject: Subject{
					Type:     SubjectTypeWorkstation,
					ID:       workstation.Name,
					Location: SubjectLocationOnFailure,
				},
				Path: fmt.Sprintf("%s.workstations[%d].onFailure", validationRoot, workstationIndex),
			})
		}
		if !workstationNeedsExplicitRejectionRoute(workstation) {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkstationMissingRejectionRoute,
			Severity: SeverityError,
			Message:  fmt.Sprintf("workstation %q must define a reject route.", workstation.Name),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       workstation.Name,
				Location: SubjectLocationOnRejection,
			},
			Path: fmt.Sprintf("%s.workstations[%d].onRejection", validationRoot, workstationIndex),
		})
	}
	return targets
}

func missingWorkTypeOutcomeStateTargets(cfg *interfaces.FactoryConfig) []Target {
	referencedWorkTypes := referencedWorkTypeSet(cfg)
	var targets []Target
	for _, workType := range cfg.WorkTypes {
		if !referencedWorkTypes[workType.Name] {
			continue
		}
		hasCompletion := false
		hasFailure := false
		for _, state := range workType.States {
			switch state.Type {
			case interfaces.StateTypeTerminal:
				hasCompletion = true
			case interfaces.StateTypeFailed:
				hasFailure = true
			}
		}
		if !hasCompletion {
			targets = append(targets, Target{
				Code:     CodeWorkTypeMissingCompletionState,
				Severity: SeverityError,
				Message:  fmt.Sprintf("work type %q must declare a completion state.", workType.Name),
				Subject: Subject{
					Type:     SubjectTypeWorkType,
					ID:       workType.Name,
					Location: SubjectLocationStates,
				},
			})
		}
		if !hasFailure {
			targets = append(targets, Target{
				Code:     CodeWorkTypeMissingFailureState,
				Severity: SeverityError,
				Message:  fmt.Sprintf("work type %q must declare a failure state.", workType.Name),
				Subject: Subject{
					Type:     SubjectTypeWorkType,
					ID:       workType.Name,
					Location: SubjectLocationStates,
				},
			})
		}
	}
	return targets
}

func missingTerminalCompletionPathTargets(cfg *interfaces.FactoryConfig) []Target {
	adj := buildWorkStateAdjacency(cfg)
	terminalStates := terminalStateSet(cfg)
	referencedStates := referencedWorkStateSet(cfg)
	stateTypes := workStateTypesForConfig(cfg)
	var targets []Target
	for stateID := range referencedStates {
		if stateTypes[stateID] == interfaces.StateTypeFailed || stateTypes[stateID] == interfaces.StateTypeTerminal {
			continue
		}
		if terminalStates[stateID] {
			continue
		}
		if canReachTerminalState(stateID, terminalStates, adj) {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkStateMissingTerminalPath,
			Severity: SeverityError,
			Message:  fmt.Sprintf("work state %q has no terminal completion path.", stateID),
			Subject: Subject{
				Type:     SubjectTypeWorkState,
				ID:       stateID,
				Location: SubjectLocationTerminal,
			},
		})
	}
	return targets
}

func referencedWorkTypeSet(cfg *interfaces.FactoryConfig) map[string]bool {
	referenced := map[string]bool{}
	addRoutes := func(routes []interfaces.IOConfig) {
		for _, route := range routes {
			if strings.TrimSpace(route.WorkTypeName) != "" {
				referenced[route.WorkTypeName] = true
			}
		}
	}
	for _, workstation := range cfg.Workstations {
		addRoutes(workstation.Inputs)
		addRoutes(workstation.Outputs)
		addRoutes(workstation.OnContinue)
		addRoutes(workstation.OnRejection)
		addRoutes(workstation.OnFailure)
		for _, route := range workstation.ClassificationRoutes {
			addRoutes(route.Outputs)
		}
	}
	return referenced
}

func referencedWorkStateSet(cfg *interfaces.FactoryConfig) map[string]bool {
	referenced := map[string]bool{}
	for _, workstation := range cfg.Workstations {
		for _, input := range workstation.Inputs {
			referenced[placeKey(input.WorkTypeName, input.StateName)] = true
		}
	}
	return referenced
}

func buildWorkStateAdjacency(cfg *interfaces.FactoryConfig) map[string]map[string]bool {
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
	connectRoutes := func(inputs, routes []interfaces.IOConfig) {
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
		connectRoutes(workstation.Inputs, workstation.Outputs)
		connectRoutes(workstation.Inputs, workstation.OnContinue)
		connectRoutes(workstation.Inputs, workstation.OnRejection)
		connectRoutes(workstation.Inputs, workstation.OnFailure)
		for _, route := range workstation.ClassificationRoutes {
			connectRoutes(workstation.Inputs, route.Outputs)
		}
	}
	return adj
}

func canReachTerminalState(start string, terminalStates map[string]bool, adj map[string]map[string]bool) bool {
	if terminalStates[start] {
		return true
	}
	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range adj[current] {
			if terminalStates[next] {
				return true
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func workStateTypesForConfig(cfg *interfaces.FactoryConfig) map[string]interfaces.StateType {
	types := map[string]interfaces.StateType{}
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			types[placeKey(workType.Name, state.Name)] = state.Type
		}
	}
	return types
}

func terminalStateSet(cfg *interfaces.FactoryConfig) map[string]bool {
	terminal := map[string]bool{}
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			if state.Type == interfaces.StateTypeTerminal {
				terminal[placeKey(workType.Name, state.Name)] = true
			}
		}
	}
	return terminal
}

func workstationNeedsExplicitRejectionRoute(workstation interfaces.FactoryWorkstationConfig) bool {
	if len(workstation.OnRejection) > 0 {
		return false
	}
	return workstation.Kind == interfaces.WorkstationKindRepeater && len(workstation.Inputs) == 0
}

func workstationCanDefaultFailureRoute(
	workstation interfaces.FactoryWorkstationConfig,
	failedWorkTypes map[string]bool,
) bool {
	for _, input := range workstation.Inputs {
		if failedWorkTypes[input.WorkTypeName] {
			return true
		}
	}
	if workstationIOsContainFailedWorkType(workstation.Outputs, failedWorkTypes) {
		return true
	}
	for _, route := range workstation.ClassificationRoutes {
		if workstationIOsContainFailedWorkType(route.Outputs, failedWorkTypes) {
			return true
		}
	}
	return false
}

func workstationIOsContainFailedWorkType(ios []interfaces.IOConfig, failedWorkTypes map[string]bool) bool {
	for _, io := range ios {
		if failedWorkTypes[io.WorkTypeName] {
			return true
		}
	}
	return false
}

func failedWorkTypeSet(cfg *interfaces.FactoryConfig) map[string]bool {
	failed := make(map[string]bool)
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

func buildValidPlaces(cfg *interfaces.FactoryConfig) map[string]bool {
	places := make(map[string]bool)
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			places[placeKey(workType.Name, state.Name)] = true
		}
	}
	for _, resource := range cfg.Resources {
		places[fmt.Sprintf("%s:available", resource.Name)] = true
	}
	return places
}

func workStateSet(cfg *interfaces.FactoryConfig) map[string]bool {
	states := make(map[string]bool)
	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			states[placeKey(workType.Name, state.Name)] = true
		}
	}
	return states
}

func placeKey(workType, state string) string {
	return workType + ":" + state
}

func apiRouteField(routeField string) string {
	replacements := []struct{ from, to string }{
		{"classification_routes", "classificationRoutes"},
		{"on_continue", "onContinue"},
		{"on_rejection", "onRejection"},
		{"on_failure", "onFailure"},
	}
	field := routeField
	for _, replacement := range replacements {
		field = strings.ReplaceAll(field, replacement.from, replacement.to)
	}
	return field
}

func ioWorkTypeCounts(ios []interfaces.IOConfig) map[string]int {
	counts := make(map[string]int)
	for _, io := range ios {
		if strings.TrimSpace(io.WorkTypeName) == "" {
			continue
		}
		counts[io.WorkTypeName]++
	}
	return counts
}

func workTypeNames(cfg *interfaces.FactoryConfig) []string {
	names := make([]string, 0, len(cfg.WorkTypes))
	for _, workType := range cfg.WorkTypes {
		names = append(names, workType.Name)
	}
	return names
}

func workerNames(cfg *interfaces.FactoryConfig) []string {
	names := make([]string, 0, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		names = append(names, worker.Name)
	}
	return names
}

func resourceNames(cfg *interfaces.FactoryConfig) []string {
	names := make([]string, 0, len(cfg.Resources))
	for _, resource := range cfg.Resources {
		names = append(names, resource.Name)
	}
	return names
}

func workstationNames(cfg *interfaces.FactoryConfig) []string {
	names := make([]string, 0, len(cfg.Workstations))
	for _, workstation := range cfg.Workstations {
		names = append(names, workstation.Name)
	}
	return names
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
