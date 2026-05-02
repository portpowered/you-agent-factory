package projections

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/workstationconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// ProjectInitialStructure projects the static net topology into the canonical
// INITIAL_STRUCTURE_REQUEST payload used by live event history and replay.
func ProjectInitialStructure(net *state.Net, runtimeConfigs ...interfaces.RuntimeDefinitionLookup) interfaces.InitialStructurePayload {
	if net == nil {
		return interfaces.InitialStructurePayload{}
	}
	runtimeConfig := interfaces.FirstRuntimeDefinitionLookup(runtimeConfigs...)
	return interfaces.InitialStructurePayload{
		Name:         runtimeFactoryName(runtimeConfig),
		Resources:    factoryResources(net.Resources),
		Constraints:  factoryConstraints(net, runtimeConfig),
		Workers:      factoryWorkers(net.Transitions, runtimeConfig),
		WorkTypes:    factoryWorkTypes(net.WorkTypes),
		Workstations: factoryWorkstations(net.Transitions, runtimeConfig),
		Places:       factoryPlaces(net.Places, net),
		Relations:    topologyRelations(net.Transitions),
	}
}

func runtimeFactoryName(runtimeConfig interfaces.RuntimeDefinitionLookup) string {
	type factoryConfigReader interface {
		FactoryConfig() *interfaces.FactoryConfig
	}
	reader, ok := runtimeConfig.(factoryConfigReader)
	if !ok || reader.FactoryConfig() == nil {
		return ""
	}
	return reader.FactoryConfig().Name
}

func factoryResources(resources map[string]*state.ResourceDef) []interfaces.FactoryResource {
	ids := sortedKeys(resources)
	out := make([]interfaces.FactoryResource, 0, len(ids))
	for _, id := range ids {
		resource := resources[id]
		if resource == nil {
			continue
		}
		out = append(out, interfaces.FactoryResource{
			ID:       resource.ID,
			Name:     resource.Name,
			Capacity: resource.Capacity,
		})
	}
	return out
}

func factoryWorkTypes(workTypes map[string]*state.WorkType) []interfaces.FactoryWorkType {
	ids := sortedKeys(workTypes)
	out := make([]interfaces.FactoryWorkType, 0, len(ids))
	for _, id := range ids {
		workType := workTypes[id]
		if workType == nil {
			continue
		}
		states := make([]interfaces.FactoryStateDefinition, 0, len(workType.States))
		for _, stateDef := range workType.States {
			states = append(states, interfaces.FactoryStateDefinition{
				Value:    stateDef.Value,
				Category: string(stateDef.Category),
			})
		}
		out = append(out, interfaces.FactoryWorkType{
			ID:     workType.ID,
			Name:   workType.Name,
			States: states,
		})
	}
	return out
}

func factoryWorkers(transitions map[string]*petri.Transition, runtimeConfig interfaces.RuntimeDefinitionLookup) []interfaces.FactoryWorker {
	if runtimeConfig == nil {
		return nil
	}
	workerIDs := transitionWorkerIDs(transitions)
	out := make([]interfaces.FactoryWorker, 0, len(workerIDs))
	for _, workerID := range workerIDs {
		def, ok := runtimeConfig.Worker(workerID)
		if !ok || def == nil {
			continue
		}
		out = append(out, factoryWorker(workerID, def))
	}
	return out
}

func transitionWorkerIDs(transitions map[string]*petri.Transition) []string {
	ids := make([]string, 0, len(transitions))
	seen := make(map[string]bool, len(transitions))
	for _, transition := range transitions {
		if transition == nil || transition.WorkerType == "" || seen[transition.WorkerType] {
			continue
		}
		seen[transition.WorkerType] = true
		ids = append(ids, transition.WorkerType)
	}
	sort.Strings(ids)
	return ids
}

func factoryWorker(workerID string, def *interfaces.WorkerConfig) interfaces.FactoryWorker {
	return interfaces.FactoryWorker{
		ID:            workerID,
		Name:          workerID,
		Provider:      interfaces.PermissivePublicFactoryWorkerProvider(def.ExecutorProvider),
		ModelProvider: interfaces.PermissivePublicFactoryWorkerModelProvider(def.ModelProvider),
		Model:         def.Model,
		Config:        workerConfig(def),
	}
}

func workerConfig(def *interfaces.WorkerConfig) map[string]string {
	if def == nil {
		return nil
	}
	config := make(map[string]string)
	if def.Type != "" {
		config["type"] = def.Type
	}
	if len(config) == 0 {
		return nil
	}
	return config
}

func factoryWorkstations(transitions map[string]*petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) []interfaces.FactoryWorkstation {
	ids := sortedKeys(transitions)
	out := make([]interfaces.FactoryWorkstation, 0, len(ids))
	for _, id := range ids {
		transition := transitions[id]
		if transition == nil {
			continue
		}
		kind := interfaces.CanonicalPublicWorkstationKind(workstationconfig.Kind(transition, runtimeConfig))
		out = append(out, interfaces.FactoryWorkstation{
			ID:                transition.ID,
			Name:              transition.Name,
			WorkerID:          transition.WorkerType,
			Kind:              kind,
			Config:            workstationConfig(transition, runtimeConfig),
			InputPlaceIDs:     arcPlaceIDs(transition.InputArcs),
			OutputPlaceIDs:    arcPlaceIDs(transition.OutputArcs),
			ContinuePlaceIDs:  arcPlaceIDs(transition.ContinueArcs),
			RejectionPlaceIDs: arcPlaceIDs(transition.RejectionArcs),
			FailurePlaceIDs:   arcPlaceIDs(transition.FailureArcs),
		})
	}
	return out
}

func workstationConfig(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) map[string]string {
	if transition == nil || runtimeConfig == nil {
		return nil
	}
	configValues := make(map[string]string)
	if workstation, ok := workstationconfig.Workstation(transition, runtimeConfig); ok && workstation != nil {
		addStringValue(configValues, "configured_worker", workstation.WorkerTypeName)
		addStringValue(configValues, "behavior", string(workstation.Kind))
		addStringValue(configValues, "worktree", workstation.Worktree)
		addStringValue(configValues, "working_directory", workstation.WorkingDirectory)
		addStringValue(configValues, "type", workstation.Type)
		addStringValue(configValues, "worker", workstation.WorkerTypeName)
		addStringValue(configValues, "prompt_file", workstation.PromptFile)
		addStringValue(configValues, "output_schema", workstation.OutputSchema)
	}
	if len(configValues) == 0 {
		return nil
	}
	return configValues
}

func factoryConstraints(net *state.Net, runtimeConfig interfaces.RuntimeDefinitionLookup) []interfaces.FactoryConstraint {
	if net == nil {
		return nil
	}
	constraints := make([]interfaces.FactoryConstraint, 0)
	constraints = append(constraints, globalLimitConstraints(net)...)
	constraints = append(constraints, transitionGuardConstraints(net.Transitions)...)
	constraints = append(constraints, runtimeConstraints(net.Transitions, runtimeConfig)...)
	sortFactoryConstraints(constraints)
	return uniqueFactoryConstraints(constraints)
}

func globalLimitConstraints(net *state.Net) []interfaces.FactoryConstraint {
	if net == nil {
		return nil
	}
	values := make(map[string]string)
	if net.Limits.MaxTokenAge > 0 {
		values["max_token_age"] = net.Limits.MaxTokenAge.String()
	}
	if net.Limits.MaxTotalVisits > 0 {
		values["max_total_visits"] = strconv.Itoa(net.Limits.MaxTotalVisits)
	}
	if len(values) == 0 {
		return nil
	}
	return []interfaces.FactoryConstraint{{
		ID:     "global/limits",
		Type:   "global_limit",
		Scope:  "global",
		Values: values,
	}}
}

func transitionGuardConstraints(transitions map[string]*petri.Transition) []interfaces.FactoryConstraint {
	ids := sortedKeys(transitions)
	constraints := make([]interfaces.FactoryConstraint, 0)
	for _, id := range ids {
		transition := transitions[id]
		if transition == nil {
			continue
		}
		constraints = append(constraints, arcGuardConstraints(transition, "input", transition.InputArcs)...)
		constraints = append(constraints, arcGuardConstraints(transition, "output", transition.OutputArcs)...)
		constraints = append(constraints, arcGuardConstraints(transition, "continue", transition.ContinueArcs)...)
		constraints = append(constraints, arcGuardConstraints(transition, "rejection", transition.RejectionArcs)...)
		constraints = append(constraints, arcGuardConstraints(transition, "failure", transition.FailureArcs)...)
	}
	return constraints
}

func arcGuardConstraints(transition *petri.Transition, arcSet string, arcs []petri.Arc) []interfaces.FactoryConstraint {
	constraints := make([]interfaces.FactoryConstraint, 0)
	for index, arc := range arcs {
		if arc.Guard == nil {
			continue
		}
		constraints = append(constraints, interfaces.FactoryConstraint{
			ID:     fmt.Sprintf("workstation/%s/%s/%d/guard", transition.ID, arcSet, index),
			Type:   guardConstraintType(arc.Guard),
			Scope:  "workstation:" + transition.ID,
			Values: guardConstraintValues(arc, arcSet),
		})
	}
	return constraints
}

func guardConstraintType(guard petri.Guard) string {
	switch guard.(type) {
	case *petri.VisitCountGuard:
		return "visit_count_guard"
	case *petri.AllWithParentGuard:
		return "all_children_complete_guard"
	case *petri.AnyWithParentGuard:
		return "any_child_failed_guard"
	case *petri.FanoutCountGuard:
		return "fanout_count_guard"
	case *petri.MatchColorGuard:
		return "match_color_guard"
	case *petri.DependencyGuard:
		return "dependency_guard"
	case *petri.CronTimeWindowGuard:
		return "cron_time_window_guard"
	case *petri.ExpiredTimeWorkGuard:
		return "expired_time_work_guard"
	default:
		return "guard"
	}
}

func guardConstraintValues(arc petri.Arc, arcSet string) map[string]string {
	values := map[string]string{
		"arc_set":  arcSet,
		"place_id": arc.PlaceID,
	}
	addStringValue(values, "binding", arc.Name)
	addStringValue(values, "mode", arcModeValue(arc.Mode))
	addStringValue(values, "cardinality", arcCardinalityValue(arc.Cardinality))
	switch guard := arc.Guard.(type) {
	case *petri.VisitCountGuard:
		addStringValue(values, "watched_transition_id", guard.TransitionID)
		if guard.MaxVisits > 0 {
			values["max_visits"] = strconv.Itoa(guard.MaxVisits)
		}
	case *petri.AllWithParentGuard:
		addStringValue(values, "match_binding", guard.MatchBinding)
	case *petri.AnyWithParentGuard:
		addStringValue(values, "match_binding", guard.MatchBinding)
	case *petri.FanoutCountGuard:
		addStringValue(values, "match_binding", guard.MatchBinding)
		addStringValue(values, "count_binding", guard.CountBinding)
	case *petri.MatchColorGuard:
		addStringValue(values, "field", guard.Field)
		addStringValue(values, "match_binding", guard.MatchBinding)
		addStringValue(values, "match_field", guard.MatchField)
	case *petri.CronTimeWindowGuard:
		addStringValue(values, "workstation", guard.Workstation)
	}
	return values
}

func runtimeConstraints(transitions map[string]*petri.Transition, runtimeConfig interfaces.RuntimeDefinitionLookup) []interfaces.FactoryConstraint {
	if runtimeConfig == nil {
		return nil
	}
	ids := sortedKeys(transitions)
	constraints := make([]interfaces.FactoryConstraint, 0)
	for _, id := range ids {
		transition := transitions[id]
		if transition == nil {
			continue
		}
		constraints = append(constraints, runtimeWorkerConstraints(transition, runtimeConfig)...)
		constraints = append(constraints, runtimeWorkstationConstraints(transition, runtimeConfig)...)
	}
	return constraints
}

func runtimeWorkerConstraints(transition *petri.Transition, runtimeConfig interfaces.RuntimeDefinitionLookup) []interfaces.FactoryConstraint {
	if transition == nil || transition.WorkerType == "" {
		return nil
	}
	def, ok := runtimeConfig.Worker(transition.WorkerType)
	if !ok || def == nil {
		return nil
	}
	constraints := make([]interfaces.FactoryConstraint, 0, 2)
	if def.Concurrency > 0 {
		constraints = append(constraints, interfaces.FactoryConstraint{
			ID:    "worker/" + transition.WorkerType + "/concurrency",
			Type:  "worker_concurrency",
			Scope: "worker:" + transition.WorkerType,
			Values: map[string]string{
				"max_concurrency": strconv.Itoa(def.Concurrency),
			},
		})
	}
	if def.Timeout != "" {
		constraints = append(constraints, interfaces.FactoryConstraint{
			ID:    "worker/" + transition.WorkerType + "/timeout",
			Type:  "worker_timeout",
			Scope: "worker:" + transition.WorkerType,
			Values: map[string]string{
				"timeout": def.Timeout,
			},
		})
	}
	return constraints
}

func runtimeWorkstationConstraints(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) []interfaces.FactoryConstraint {
	if transition == nil || runtimeConfig == nil {
		return nil
	}
	workstation, ok := workstationconfig.Workstation(transition, runtimeConfig)
	if !ok || workstation == nil {
		return nil
	}
	return workstationConfigConstraints(transition, workstation)
}

func workstationConfigConstraints(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	normalized := config.CloneWorkstationConfig(*cfg)
	config.NormalizeWorkstationExecutionLimit(&normalized)

	constraints := make([]interfaces.FactoryConstraint, 0)
	constraints = append(constraints, workstationResourceConstraints(transition, &normalized)...)
	constraints = append(constraints, workstationConfiguredGuardConstraints(transition, &normalized)...)
	constraints = append(constraints, workstationCronConstraint(transition, &normalized)...)
	constraints = append(constraints, workstationStopWordsConstraint(transition, &normalized)...)
	constraints = append(constraints, workstationLimitConstraint(transition, &normalized)...)
	return constraints
}

func workstationResourceConstraints(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	constraints := make([]interfaces.FactoryConstraint, 0, len(cfg.Resources))
	for index, resource := range cfg.Resources {
		if resource.Name == "" || resource.Capacity <= 0 {
			continue
		}
		constraints = append(constraints, interfaces.FactoryConstraint{
			ID:    fmt.Sprintf("workstation/%s/resource/%s/%d", transition.ID, resource.Name, index),
			Type:  "resource_usage",
			Scope: "workstation:" + transition.ID,
			Values: map[string]string{
				"resource_id": resource.Name,
				"capacity":    strconv.Itoa(resource.Capacity),
			},
		})
	}
	return constraints
}

func workstationConfiguredGuardConstraints(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	constraints := make([]interfaces.FactoryConstraint, 0, len(cfg.Guards))
	for index, guard := range cfg.Guards {
		values := guardConfigValues(guard)
		if len(values) == 0 {
			continue
		}
		constraints = append(constraints, interfaces.FactoryConstraint{
			ID:     fmt.Sprintf("workstation/%s/config-guard/%d", transition.ID, index),
			Type:   "configured_guard",
			Scope:  "workstation:" + transition.ID,
			Values: values,
		})
	}
	return constraints
}

func workstationCronConstraint(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	if cfg.Cron == nil {
		return nil
	}
	values := make(map[string]string)
	addStringValue(values, "schedule", cfg.Cron.Schedule)
	if cfg.Cron.TriggerAtStart {
		values["trigger_at_start"] = "true"
	}
	addStringValue(values, "jitter", cfg.Cron.Jitter)
	addStringValue(values, "expiry_window", cfg.Cron.ExpiryWindow)
	if len(values) == 0 {
		return nil
	}
	return []interfaces.FactoryConstraint{{
		ID:     "workstation/" + transition.ID + "/cron",
		Type:   "cron_trigger",
		Scope:  "workstation:" + transition.ID,
		Values: values,
	}}
}

func workstationStopWordsConstraint(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	if len(cfg.StopWords) > 0 {
		return []interfaces.FactoryConstraint{{
			ID:     "workstation/" + transition.ID + "/stop-words",
			Type:   "stop_words",
			Scope:  "workstation:" + transition.ID,
			Values: map[string]string{"words": strings.Join(cfg.StopWords, ",")},
		}}
	}
	return nil
}

func workstationLimitConstraint(transition *petri.Transition, cfg *interfaces.FactoryWorkstationConfig) []interfaces.FactoryConstraint {
	if cfg.Limits.MaxRetries <= 0 && cfg.Limits.MaxExecutionTime == "" {
		return nil
	}
	values := make(map[string]string)
	if cfg.Limits.MaxRetries > 0 {
		values["max_retries"] = strconv.Itoa(cfg.Limits.MaxRetries)
	}
	addStringValue(values, "max_execution_time", cfg.Limits.MaxExecutionTime)
	return []interfaces.FactoryConstraint{{
		ID:     "workstation/" + transition.ID + "/limits",
		Type:   "workstation_limit",
		Scope:  "workstation:" + transition.ID,
		Values: values,
	}}
}

func guardConfigValues(guard interfaces.GuardConfig) map[string]string {
	values := make(map[string]string)
	addStringValue(values, "type", string(guard.Type))
	addStringValue(values, "workstation", guard.Workstation)
	if guard.MaxVisits > 0 {
		values["max_visits"] = strconv.Itoa(guard.MaxVisits)
	}
	return values
}

func arcModeValue(mode interfaces.ArcMode) string {
	switch mode {
	case interfaces.ArcModeObserve:
		return "OBSERVE"
	default:
		return "CONSUME"
	}
}

func arcCardinalityValue(cardinality petri.ArcCardinality) string {
	switch cardinality.Mode {
	case petri.CardinalityAll:
		return "ALL"
	case petri.CardinalityAllTerminal:
		return "ALL_TERMINAL"
	case petri.CardinalityN:
		return "N:" + strconv.Itoa(cardinality.Count)
	case petri.CardinalityZeroOrMore:
		return "ZERO_OR_MORE"
	default:
		return "ONE"
	}
}

func addStringValue(values map[string]string, key, value string) {
	if value != "" {
		values[key] = value
	}
}

func sortFactoryConstraints(constraints []interfaces.FactoryConstraint) {
	sort.Slice(constraints, func(i, j int) bool {
		if constraints[i].Scope != constraints[j].Scope {
			return constraints[i].Scope < constraints[j].Scope
		}
		if constraints[i].ID != constraints[j].ID {
			return constraints[i].ID < constraints[j].ID
		}
		return constraints[i].Type < constraints[j].Type
	})
}

func uniqueFactoryConstraints(constraints []interfaces.FactoryConstraint) []interfaces.FactoryConstraint {
	if len(constraints) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(constraints))
	out := make([]interfaces.FactoryConstraint, 0, len(constraints))
	for _, constraint := range constraints {
		if constraint.ID == "" || seen[constraint.ID] {
			continue
		}
		seen[constraint.ID] = true
		out = append(out, constraint)
	}
	return out
}

func factoryPlaces(places map[string]*petri.Place, net *state.Net) []interfaces.FactoryPlace {
	ids := sortedKeys(places)
	out := make([]interfaces.FactoryPlace, 0, len(ids))
	for _, id := range ids {
		place := places[id]
		if place == nil {
			continue
		}
		out = append(out, interfaces.FactoryPlace{
			ID:       place.ID,
			TypeID:   place.TypeID,
			State:    place.State,
			Category: string(net.StateCategoryForPlace(place.ID)),
		})
	}
	return out
}

func topologyRelations(transitions map[string]*petri.Transition) []interfaces.FactoryRelation {
	ids := sortedKeys(transitions)
	out := make([]interfaces.FactoryRelation, 0, len(ids))
	for _, id := range ids {
		transition := transitions[id]
		if transition == nil {
			continue
		}
		for _, arc := range transition.InputArcs {
			out = append(out, interfaces.FactoryRelation{
				Type:          "INPUT",
				TargetWorkID:  arc.PlaceID,
				RequiredState: arc.Name,
			})
		}
		for _, arc := range transition.OutputArcs {
			out = append(out, interfaces.FactoryRelation{
				Type:         "OUTPUT",
				SourceWorkID: transition.ID,
				TargetWorkID: arc.PlaceID,
			})
		}
	}
	return out
}

func arcPlaceIDs(arcs []petri.Arc) []string {
	if len(arcs) == 0 {
		return nil
	}
	placeIDs := make([]string, 0, len(arcs))
	for _, arc := range arcs {
		placeIDs = append(placeIDs, arc.PlaceID)
	}
	sort.Strings(placeIDs)
	return placeIDs
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
