package config

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/factory/state/validation"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
)

// ConfigMapper converts a FactoryConfig into a petri state.
type ConfigMapper struct{}

func (cm *ConfigMapper) Map(ctx context.Context, cfg *interfaces.FactoryConfig, runtimeConfigs ...interfaces.RuntimeWorkstationLookup) (*state.Net, error) {
	cv := NewConfigValidator()
	if result := cv.Validate(cfg); result.HasErrors() {
		return nil, fmt.Errorf("%s", result.Error())
	}

	places := cm.convertToPlaces(cfg)
	fanoutGroups := make(map[string]string)
	transitions := cm.convertToTransitions(cfg, places, fanoutGroups)
	cm.addDefaultTimeExpiryTransition(cfg, transitions)
	cm.addDependencyGuards(cfg, transitions)

	n := &state.Net{
		Places:      places,
		Transitions: transitions,
		WorkTypes:   cm.convertToWorkTypes(cfg),
		Resources:   cm.convertToResources(cfg),
		InputTypes:  cm.convertToInputTypes(cfg),
	}
	if len(fanoutGroups) > 0 {
		n.FanoutGroups = fanoutGroups
	}

	state.NormalizeTransitionTopology(n, firstTransitionTopologyRuntimeConfig(cfg, runtimeConfigs...))
	if err := validateNetTopology(n); err != nil {
		return nil, err
	}

	return n, nil
}

func validateNetTopology(n *state.Net) error {
	validator := validation.NewCompositeValidator(
		&validation.TypeAlignmentValidator{},
	)

	for _, violation := range validator.Validate(n) {
		if violation.Level == validation.ViolationError {
			return fmt.Errorf("net validation failed: %s - %s (at %s)", violation.Code, violation.Message, violation.Location)
		}
	}

	return nil
}

func firstTransitionTopologyRuntimeConfig(cfg *interfaces.FactoryConfig, runtimeConfigs ...interfaces.RuntimeWorkstationLookup) interfaces.RuntimeWorkstationLookup {
	if runtimeConfig := interfaces.FirstRuntimeWorkstationLookup(runtimeConfigs...); runtimeConfig != nil {
		return runtimeConfig
	}
	return factoryConfigWorkstationLookup(cfg)
}

type factoryConfigWorkstationLookupAdapter struct {
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeWorkstationLookup = factoryConfigWorkstationLookupAdapter{}

func factoryConfigWorkstationLookup(cfg *interfaces.FactoryConfig) interfaces.RuntimeWorkstationLookup {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}
	workstations := make(map[string]*interfaces.FactoryWorkstationConfig, len(cfg.Workstations))
	for i := range cfg.Workstations {
		workstation := &cfg.Workstations[i]
		workstations[workstation.Name] = workstation
	}
	return factoryConfigWorkstationLookupAdapter{workstations: workstations}
}

func (c factoryConfigWorkstationLookupAdapter) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c.workstations == nil {
		return nil, false
	}
	workstation, ok := c.workstations[name]
	return workstation, ok
}

// portos:func-length-exception owner=agent-factory reason=legacy-topology-construction review=2026-07-18 removal=split-transition-assembly-before-next-topology-expansion
func (cm *ConfigMapper) convertToTransitions(cfg *interfaces.FactoryConfig, places map[string]*petri.Place, fanoutGroups map[string]string) map[string]*petri.Transition {
	transitions := make(map[string]*petri.Transition)

	for _, ws := range cfg.Workstations {
		t := &petri.Transition{
			ID:         ws.Name,
			Name:       ws.Name,
			Type:       petri.TransitionNormal,
			WorkerType: ws.WorkerTypeName,
		}
		transitions[t.Name] = t

		for _, input := range ws.Inputs {
			placeID := mapToID(input)
			name := fmt.Sprintf("%s:%s:to:%s", input.WorkTypeName, input.StateName, t.Name)
			t.InputArcs = append(t.InputArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         name,
				PlaceID:      placeID,
				TransitionID: t.ID,
			})
		}
		for _, output := range ws.Outputs {
			placeID := mapToID(output)
			name := fmt.Sprintf("%s:%s:from:%s", output.WorkTypeName, output.StateName, t.Name)
			t.OutputArcs = append(t.OutputArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         name,
				PlaceID:      placeID,
				TransitionID: t.ID,
			})
		}
		if ws.OnRejection != nil {
			placeID := mapToID(*ws.OnRejection)
			name := fmt.Sprintf("%s:%s:rejection:%s", ws.OnRejection.WorkTypeName, ws.OnRejection.StateName, t.Name)
			t.RejectionArcs = append(t.RejectionArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         name,
				PlaceID:      placeID,
				TransitionID: t.ID,
			})
		}
		if ws.OnFailure != nil {
			placeID := mapToID(*ws.OnFailure)
			name := fmt.Sprintf("%s:%s:failure:%s", ws.OnFailure.WorkTypeName, ws.OnFailure.StateName, t.Name)
			t.FailureArcs = append(t.FailureArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         name,
				PlaceID:      placeID,
				TransitionID: t.ID,
			})
		}

		cm.applyWorkstationGuards(ws, t)

		// Handle per-input guards: generate observation arcs with parent-match guards scoped to the specific input.
		cm.applyInputGuards(ws, t, places, fanoutGroups)

		cm.addCronTimeInputArc(ws, t)

		// Handle resource usage: generate consume arcs (input) and release arcs (output).
		for _, ru := range ws.Resources {
			resourcePlaceID := fmt.Sprintf("%s:%s", ru.Name, interfaces.ResourceStateAvailable)

			// Consume arc: take resource token(s) when transition fires.
			t.InputArcs = append(t.InputArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         fmt.Sprintf("%s:consume:%s", ru.Name, t.Name),
				PlaceID:      resourcePlaceID,
				TransitionID: t.ID,
				Direction:    petri.ArcInput,
				Mode:         interfaces.ArcModeConsume,
				Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityN, Count: ru.Capacity},
			})

			// Release arc: return resource token(s) when transition completes.
			t.OutputArcs = append(t.OutputArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         fmt.Sprintf("%s:release:%s", ru.Name, t.Name),
				PlaceID:      resourcePlaceID,
				TransitionID: t.ID,
				Direction:    petri.ArcOutput,
				Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityN, Count: ru.Capacity},
			})
		}
	}
	return transitions
}

func (cm *ConfigMapper) applyWorkstationGuards(ws interfaces.FactoryWorkstationConfig, t *petri.Transition) {
	if len(t.InputArcs) == 0 || len(ws.Guards) == 0 {
		return
	}

	sourceBinding := inputArcBindingName(t.InputArcs[0])
	for _, g := range ws.Guards {
		switch g.Type {
		case interfaces.GuardTypeMatchesFields:
			for i := range t.InputArcs {
				matcher := &petri.MatchesFieldsGuard{
					InputKey: g.MatchConfig.InputKey,
				}
				if i > 0 {
					matcher.MatchBinding = sourceBinding
				}
				t.InputArcs[i].Guard = combineArcGuards(t.InputArcs[i].Guard, matcher)
			}
		default:
			guard := cm.resolveGuard(g)
			if guard != nil {
				t.InputArcs[0].Guard = combineArcGuards(t.InputArcs[0].Guard, guard)
			}
		}
	}
}

// applyInputGuards processes per-input guard declarations on workstation inputs.
// For each input with a Guard field, it:
// - Finds the parent input arc (by matching parent_input work type) and names it "parent"
// - Converts the guarded input arc to OBSERVE mode with the appropriate guard
// - If spawned_by is set, creates a fanout count place + consume arc for dynamic count tracking
// portos:func-length-exception owner=agent-factory reason=legacy-guard-topology-builder review=2026-07-18 removal=split-dynamic-and-static-input-guard-builders-when-guards-are-next-touched
func (cm *ConfigMapper) applyInputGuards(ws interfaces.FactoryWorkstationConfig, t *petri.Transition, netPlaces map[string]*petri.Place, fanoutGroups map[string]string) {
	// Check if any inputs have guards.
	hasGuards := false
	for _, input := range ws.Inputs {
		if input.Guard != nil {
			hasGuards = true
			break
		}
	}
	if !hasGuards {
		return
	}

	parentBinding := "parent"
	countBinding := "fanout-count"

	// Find and name the parent input arc. The parent is identified by matching
	// the parent_input work type from any parent-aware guarded input.
	for _, input := range ws.Inputs {
		if input.Guard == nil || input.Guard.Type == interfaces.GuardTypeSameName {
			continue
		}
		// Find the arc index for the parent input.
		for i := range t.InputArcs {
			parentPlaceID := fmt.Sprintf("%s:", input.Guard.ParentInput)
			if len(t.InputArcs[i].PlaceID) >= len(parentPlaceID) && t.InputArcs[i].PlaceID[:len(parentPlaceID)] == parentPlaceID {
				t.InputArcs[i].Name = parentBinding
				break
			}
		}
		break // All guarded inputs in a workstation share the same parent binding.
	}

	// Process each parent-aware guarded input: replace the original consume arc
	// with an observe arc and (for dynamic fanout) a count consume arc. The
	// count arc MUST appear before the observation arc so that the
	// "fanout-count" binding is available when the FanoutCountGuard evaluates.
	for idx, input := range ws.Inputs {
		if input.Guard == nil || input.Guard.Type == interfaces.GuardTypeSameName {
			continue
		}
		g := input.Guard

		childPlaceID := mapToID(input)
		childArcName := fmt.Sprintf("%s:%s:observe:%s", input.WorkTypeName, input.StateName, t.Name)

		if g.SpawnedBy != "" {
			// Dynamic fanout: create a count place and wire up FanoutCountGuard.
			countPlaceID := fmt.Sprintf("%s:fanout-count", g.SpawnedBy)
			netPlaces[countPlaceID] = &petri.Place{
				ID:     countPlaceID,
				TypeID: "fanout-count",
				State:  "count",
			}
			fanoutGroups[g.SpawnedBy] = countPlaceID

			// Replace the original input arc at [idx] with the count consume arc,
			// then append the observation arc.
			t.InputArcs[idx] = petri.Arc{
				ID:           uuid.NewString(),
				Name:         countBinding,
				PlaceID:      countPlaceID,
				TransitionID: t.ID,
				Direction:    petri.ArcInput,
				Mode:         interfaces.ArcModeConsume,
				Guard: &petri.MatchColorGuard{
					Field:        "parent_id",
					MatchBinding: parentBinding,
					MatchField:   "work_id",
				},
				Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			}

			var cardinality petri.ArcCardinality
			var childGuard petri.Guard
			if g.Type == interfaces.GuardTypeAllChildrenComplete {
				cardinality = petri.ArcCardinality{Mode: petri.CardinalityZeroOrMore}
				childGuard = &petri.FanoutCountGuard{
					MatchBinding: parentBinding,
					CountBinding: countBinding,
				}
			} else {
				cardinality = petri.ArcCardinality{Mode: petri.CardinalityOne}
				childGuard = &petri.AnyWithParentGuard{MatchBinding: parentBinding}
			}

			t.InputArcs = append(t.InputArcs, petri.Arc{
				ID:           uuid.NewString(),
				Name:         childArcName,
				PlaceID:      childPlaceID,
				TransitionID: t.ID,
				Direction:    petri.ArcInput,
				Mode:         interfaces.ArcModeObserve,
				Guard:        childGuard,
				Cardinality:  cardinality,
			})
		} else {
			// Static fanout: use AllWithParentGuard/AnyWithParentGuard.
			var childGuard petri.Guard
			var cardinality petri.ArcCardinality
			if g.Type == interfaces.GuardTypeAllChildrenComplete {
				childGuard = &petri.AllWithParentGuard{MatchBinding: parentBinding}
				cardinality = petri.ArcCardinality{Mode: petri.CardinalityAll}
			} else {
				childGuard = &petri.AnyWithParentGuard{MatchBinding: parentBinding}
				cardinality = petri.ArcCardinality{Mode: petri.CardinalityOne}
			}

			// Replace the original input arc with the observation arc.
			t.InputArcs[idx] = petri.Arc{
				ID:           uuid.NewString(),
				Name:         childArcName,
				PlaceID:      childPlaceID,
				TransitionID: t.ID,
				Direction:    petri.ArcInput,
				Mode:         interfaces.ArcModeObserve,
				Guard:        childGuard,
				Cardinality:  cardinality,
			}
		}
	}

	// Same-name guards stay on the original consume arc and bind against the
	// referenced peer input's final arc name.
	for idx, input := range ws.Inputs {
		if input.Guard == nil || input.Guard.Type != interfaces.GuardTypeSameName {
			continue
		}

		peerBinding, ok := inputGuardBindingName(ws.Inputs, t.InputArcs, input.Guard.MatchInput)
		if !ok {
			continue
		}
		t.InputArcs[idx].Guard = &petri.SameNameGuard{MatchBinding: peerBinding}
	}
}

func inputGuardBindingName(inputs []interfaces.IOConfig, arcs []petri.Arc, workTypeName string) (string, bool) {
	for _, input := range inputs {
		if input.WorkTypeName != workTypeName {
			continue
		}
		placeID := mapToID(input)
		for _, arc := range arcs {
			if arc.Direction != petri.ArcInput {
				continue
			}
			if arc.PlaceID == placeID && arc.Name != "" {
				return arc.Name, true
			}
		}
	}
	return "", false
}

func (cm *ConfigMapper) addDefaultTimeExpiryTransition(cfg *interfaces.FactoryConfig, transitions map[string]*petri.Transition) {
	if !hasCronWorkstation(cfg) {
		return
	}
	t := &petri.Transition{
		ID:   interfaces.SystemTimeExpiryTransitionID,
		Name: interfaces.SystemTimeExpiryTransitionID,
		Type: petri.TransitionExhaustion,
		InputArcs: []petri.Arc{
			{
				ID:           uuid.NewString(),
				Name:         fmt.Sprintf("%s:to:%s", interfaces.SystemTimePendingPlaceID, interfaces.SystemTimeExpiryTransitionID),
				PlaceID:      interfaces.SystemTimePendingPlaceID,
				TransitionID: interfaces.SystemTimeExpiryTransitionID,
				Direction:    petri.ArcInput,
				Mode:         interfaces.ArcModeConsume,
				Guard:        &petri.ExpiredTimeWorkGuard{},
				Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityAll},
			},
		},
	}
	transitions[t.Name] = t
}

func (cm *ConfigMapper) convertToPlaces(cfg *interfaces.FactoryConfig) map[string]*petri.Place {
	places := make(map[string]*petri.Place)

	for _, resource := range cfg.Resources {
		place := mapToPlace(resource)
		places[place.ID] = place
	}

	for _, workType := range cfg.WorkTypes {
		for _, state := range workType.States {
			id := fmt.Sprintf("%s:%s", workType.Name, state.Name)
			places[id] = &petri.Place{
				ID:     id,
				TypeID: workType.Name,
				State:  state.Name,
			}
		}
	}
	if hasCronWorkstation(cfg) {
		places[interfaces.SystemTimePendingPlaceID] = &petri.Place{
			ID:     interfaces.SystemTimePendingPlaceID,
			TypeID: interfaces.SystemTimeWorkTypeID,
			State:  interfaces.SystemTimePendingState,
		}
	}
	return places
}

func (cm *ConfigMapper) addCronTimeInputArc(ws interfaces.FactoryWorkstationConfig, t *petri.Transition) {
	if ws.Kind != interfaces.WorkstationKindCron {
		return
	}
	t.InputArcs = append(t.InputArcs, petri.Arc{
		ID:           uuid.NewString(),
		Name:         fmt.Sprintf("%s:to:%s", interfaces.SystemTimePendingPlaceID, t.Name),
		PlaceID:      interfaces.SystemTimePendingPlaceID,
		TransitionID: t.ID,
		Direction:    petri.ArcInput,
		Mode:         interfaces.ArcModeConsume,
		Guard:        &petri.CronTimeWindowGuard{Workstation: ws.Name},
		Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityOne},
	})
}

// resolveGuard converts a workstation-level GuardConfig into a petri Guard.
func (cm *ConfigMapper) resolveGuard(g interfaces.GuardConfig) petri.Guard {
	switch g.Type {
	case interfaces.GuardTypeVisitCount:
		return &petri.VisitCountGuard{
			TransitionID: g.Workstation, // workstation name == transition ID
			MaxVisits:    g.MaxVisits,
		}
	default:
		return nil
	}
}

func combineArcGuards(existing, next petri.Guard) petri.Guard {
	if next == nil {
		return existing
	}
	if existing == nil {
		return next
	}

	var guards []petri.Guard
	if chained, ok := existing.(*petri.AllGuard); ok {
		guards = append(guards, chained.Guards...)
	} else {
		guards = append(guards, existing)
	}
	if chained, ok := next.(*petri.AllGuard); ok {
		guards = append(guards, chained.Guards...)
	} else {
		guards = append(guards, next)
	}
	return &petri.AllGuard{Guards: guards}
}

func inputArcBindingName(arc petri.Arc) string {
	if arc.Name != "" {
		return arc.Name
	}
	return arc.ID
}

func mapToID(io interfaces.IOConfig) string {
	return fmt.Sprintf("%s:%s", io.WorkTypeName, io.StateName)
}

func mapToPlace(resource interfaces.ResourceConfig) *petri.Place {
	return &petri.Place{
		ID:     fmt.Sprintf("%s:%s", resource.Name, interfaces.ResourceStateAvailable),
		TypeID: resource.Name,
		State:  interfaces.ResourceStateAvailable,
	}
}

// convertToWorkTypes builds WorkType definitions from config for the Net.
func (cm *ConfigMapper) convertToWorkTypes(cfg *interfaces.FactoryConfig) map[string]*state.WorkType {
	workTypes := make(map[string]*state.WorkType, len(cfg.WorkTypes)+1)
	for _, wt := range cfg.WorkTypes {
		states := make([]state.StateDefinition, len(wt.States))
		for i, s := range wt.States {
			states[i] = state.StateDefinition{
				Value:    s.Name,
				Category: mapStateCategory(s.Type),
			}
		}
		workTypes[wt.Name] = &state.WorkType{
			ID:     wt.Name,
			Name:   wt.Name,
			States: states,
		}
	}
	if hasCronWorkstation(cfg) {
		workTypes[interfaces.SystemTimeWorkTypeID] = &state.WorkType{
			ID:   interfaces.SystemTimeWorkTypeID,
			Name: interfaces.SystemTimeWorkTypeID,
			States: []state.StateDefinition{
				{Value: interfaces.SystemTimePendingState, Category: state.StateCategoryProcessing},
			},
		}
	}
	return workTypes
}

// convertToResources builds ResourceDef definitions from config for the Net.
func (cm *ConfigMapper) convertToResources(cfg *interfaces.FactoryConfig) map[string]*state.ResourceDef {
	if len(cfg.Resources) == 0 {
		return nil
	}
	resources := make(map[string]*state.ResourceDef, len(cfg.Resources))
	for _, r := range cfg.Resources {
		resources[r.Name] = &state.ResourceDef{
			ID:       r.Name,
			Name:     r.Name,
			Capacity: r.Capacity,
		}
	}
	return resources
}

// addDependencyGuards adds a DependencyGuard to input arcs consuming from INITIAL places
// on NORMAL transitions that don't already have a guard. This ensures DEPENDS_ON
// relations from canonical work request batches are enforced by the scheduler.
func (cm *ConfigMapper) addDependencyGuards(cfg *interfaces.FactoryConfig, transitions map[string]*petri.Transition) {
	// Build a set of INITIAL place IDs.
	initialPlaces := make(map[string]bool)
	for _, wt := range cfg.WorkTypes {
		for _, s := range wt.States {
			if s.Type == interfaces.StateTypeInitial {
				initialPlaces[fmt.Sprintf("%s:%s", wt.Name, s.Name)] = true
			}
		}
	}

	for _, t := range transitions {
		if t.Type != petri.TransitionNormal {
			continue
		}
		for i := range t.InputArcs {
			arc := &t.InputArcs[i]
			if arc.Guard != nil {
				continue // don't override existing guards
			}
			if initialPlaces[arc.PlaceID] {
				arc.Guard = &petri.DependencyGuard{}
			}
		}
	}
}

// convertToInputTypes builds InputType definitions from config for the Net.
// The implicit "default" input type is always included.
func (cm *ConfigMapper) convertToInputTypes(cfg *interfaces.FactoryConfig) map[string]*state.InputType {
	inputTypes := make(map[string]*state.InputType, len(cfg.InputTypes)+1)
	// Always include the implicit default input type.
	inputTypes["default"] = &state.InputType{
		Name: "default",
		Kind: string(interfaces.InputKindDefault),
	}
	for _, it := range cfg.InputTypes {
		inputTypes[it.Name] = &state.InputType{
			Name: it.Name,
			Kind: string(it.Type),
		}
	}
	return inputTypes
}

func mapStateCategory(st interfaces.StateType) state.StateCategory {
	switch st {
	case interfaces.StateTypeInitial:
		return state.StateCategoryInitial
	case interfaces.StateTypeProcessing:
		return state.StateCategoryProcessing
	case interfaces.StateTypeTerminal:
		return state.StateCategoryTerminal
	case interfaces.StateTypeFailed:
		return state.StateCategoryFailed
	default:
		return state.StateCategoryProcessing
	}
}

func hasCronWorkstation(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	for _, ws := range cfg.Workstations {
		if ws.Kind == interfaces.WorkstationKindCron {
			return true
		}
	}
	return false
}
