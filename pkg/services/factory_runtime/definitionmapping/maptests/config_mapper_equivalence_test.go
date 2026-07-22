package maptests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state/validation"
)

// TestConfigMapper_BuilderEquivalence verifies that the config mapper produces
// nets structurally equivalent to those built with the fluent builder API.
// Each subtest constructs the same logical net via both approaches and compares.
func TestConfigMapper_BuilderEquivalence(t *testing.T) {
	tests := []struct {
		name     string
		buildNet func() (*factoryruntime.Net, error)
		buildCfg func() *interfaces.FactoryConfig
	}{
		{
			name:     "simple_linear_path",
			buildNet: buildSimpleLinear,
			buildCfg: configSimpleLinear,
		},
		{
			name:     "rejection_and_failure_arcs",
			buildNet: buildRejectionFailure,
			buildCfg: configRejectionFailure,
		},
		{
			name:     "rejection_loop_with_guarded_loop_breaker",
			buildNet: buildRejectionGuardedLoopBreaker,
			buildCfg: configRejectionGuardedLoopBreaker,
		},
		{
			name:     "resource_contention",
			buildNet: buildResourceContention,
			buildCfg: configResourceContention,
		},
		{
			name:     "fanout_all_children_guard",
			buildNet: buildFanoutAllChildrenGuard,
			buildCfg: configFanoutAllChildrenGuard,
		},
		{
			name:     "multi_work_type_pipeline",
			buildNet: buildMultiWorkType,
			buildCfg: configMultiWorkType,
		},
	}

	mapper := testConfigMapper{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builderNet, err := tt.buildNet()
			if err != nil {
				t.Fatalf("builder failed: %v", err)
			}

			configNet, err := mapper.Map(context.Background(), tt.buildCfg())
			if err != nil {
				t.Fatalf("config mapper failed: %v", err)
			}

			assertStructuralEquivalence(t, builderNet, configNet)
		})
	}
}

// --- Pattern 1: Simple linear path ---

func buildSimpleLinear() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("task", "init", "complete", "failed"),
		},
		nil,
		&factoryruntime.PetriTransition{
			ID:         "transformer",
			Name:       "transformer",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "transform-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("task:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("task:complete"),
			},
		},
	)
}

func configSimpleLinear() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: threeStates("init", "complete", "failed")},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "transform-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "transformer",
				WorkerTypeName: "transform-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
		},
	}
}

// --- Pattern 2: Rejection + failure arcs ---

func buildRejectionFailure() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("task", "init", "processing", "complete", "failed"),
		},
		nil,
		&factoryruntime.PetriTransition{
			ID:         "processor",
			Name:       "processor",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "process-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("task:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("task:complete"),
			},
			RejectionArcs: []factoryruntime.PetriArc{
				newOutputArc("task:init"),
			},
			FailureArcs: []factoryruntime.PetriArc{
				newOutputArc("task:failed"),
			},
		},
	)
}

func configRejectionFailure() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: fourStates("init", "processing", "complete", "failed")},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "process-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "process-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				OnRejection:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			},
		},
	}
}

// --- Pattern 3: Rejection loop with guarded loop breaker ---

func buildRejectionGuardedLoopBreaker() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("task", "init", "complete", "failed"),
		},
		nil,
		&factoryruntime.PetriTransition{
			ID:         "reviewer",
			Name:       "reviewer",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "review-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("task:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("task:complete"),
			},
			RejectionArcs: []factoryruntime.PetriArc{
				newOutputArc("task:init"),
			},
		},
		&factoryruntime.PetriTransition{
			ID:   "reviewer-loop-breaker",
			Name: "reviewer-loop-breaker",
			Type: factoryruntime.PetriTransitionNormal,
			InputArcs: []factoryruntime.PetriArc{
				newGuardedInputArc("task:init", "task:init:to:reviewer-loop-breaker", &factoryruntime.PetriVisitCountGuard{
					TransitionID: "reviewer",
					MaxVisits:    3,
				}),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("task:failed"),
			},
		},
	)
}

func configRejectionGuardedLoopBreaker() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: threeStates("init", "complete", "failed")},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "review-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "reviewer",
				WorkerTypeName: "review-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				OnRejection:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			},
			{
				Name:    "reviewer-loop-breaker",
				Type:    interfaces.WorkstationTypeLogical,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
				Guards: []interfaces.GuardConfig{{
					Type:        interfaces.GuardTypeVisitCount,
					Workstation: "reviewer",
					MaxVisits:   3,
				}},
			},
		},
	}
}

// --- Pattern 4: Resource contention ---

func buildResourceContention() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("task", "init", "complete", "failed"),
		},
		[]*factoryruntime.ResourceDef{
			{ID: "gpu", Name: "gpu", Capacity: 2},
		},
		&factoryruntime.PetriTransition{
			ID:         "processor",
			Name:       "processor",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "gpu-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("task:init", ""),
				newInputArc("gpu:available", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("task:complete"),
				newOutputArc("gpu:available"),
			},
		},
	)
}

func configResourceContention() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: threeStates("init", "complete", "failed")},
		},
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "gpu-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "gpu-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				Resources:      []interfaces.ResourceConfig{{Name: "gpu", Capacity: 1}},
			},
		},
	}
}

// --- Pattern 5: Fanout fan-in (all children complete guard) ---

func buildFanoutAllChildrenGuard() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("request", "init", "waiting", "complete", "failed"),
			newAutoCategorizedWorkType("page", "init", "complete", "failed"),
		},
		nil,
		&factoryruntime.PetriTransition{
			ID:         "splitter",
			Name:       "splitter",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "split-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("request:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("request:waiting"),
			},
		},
		&factoryruntime.PetriTransition{
			ID:         "page-processor",
			Name:       "page-processor",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "page-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("page:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("page:complete"),
			},
		},
		&factoryruntime.PetriTransition{
			ID:         "collector",
			Name:       "collector",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "collect-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("request:waiting", "parent"),
				newObserveAllArc("page:complete", "children", &factoryruntime.PetriAllWithParentGuard{MatchBinding: "parent"}),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("request:complete"),
			},
		},
	)
}

func configFanoutAllChildrenGuard() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "request", States: fourStates("init", "waiting", "complete", "failed")},
			{Name: "page", States: threeStates("init", "complete", "failed")},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "split-worker"},
			{Name: "page-worker"},
			{Name: "collect-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "splitter",
				WorkerTypeName: "split-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "request", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "request", StateName: "waiting"}},
			},
			{
				Name:           "page-processor",
				WorkerTypeName: "page-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "page", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "page", StateName: "complete"}},
			},
			{
				Name:           "collector",
				WorkerTypeName: "collect-worker",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "request", StateName: "waiting"},
					{
						WorkTypeName: "page",
						StateName:    "complete",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "request",
						},
					},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "request", StateName: "complete"}},
			},
		},
	}
}

// --- Pattern 6: Multi-work-type pipeline ---

func buildMultiWorkType() (*factoryruntime.Net, error) {
	return buildTestNet("test",
		[]*factoryruntime.WorkType{
			newAutoCategorizedWorkType("request", "init", "validated", "complete", "failed"),
			newAutoCategorizedWorkType("report", "init", "complete", "failed"),
		},
		nil,
		&factoryruntime.PetriTransition{
			ID:         "validator",
			Name:       "validator",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "validation-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("request:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("request:validated"),
			},
		},
		&factoryruntime.PetriTransition{
			ID:         "report-generator",
			Name:       "report-generator",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "report-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("request:validated", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("request:complete"),
				newOutputArc("report:init"),
			},
		},
		&factoryruntime.PetriTransition{
			ID:         "report-finalizer",
			Name:       "report-finalizer",
			Type:       factoryruntime.PetriTransitionNormal,
			WorkerType: "finalize-worker",
			InputArcs: []factoryruntime.PetriArc{
				newInputArc("report:init", ""),
			},
			OutputArcs: []factoryruntime.PetriArc{
				newOutputArc("report:complete"),
			},
		},
	)
}

func configMultiWorkType() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "request", States: fourStates("init", "validated", "complete", "failed")},
			{Name: "report", States: threeStates("init", "complete", "failed")},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "validation-worker"},
			{Name: "report-worker"},
			{Name: "finalize-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "validator",
				WorkerTypeName: "validation-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "request", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "request", StateName: "validated"}},
			},
			{
				Name:           "report-generator",
				WorkerTypeName: "report-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "request", StateName: "validated"}},
				Outputs: []interfaces.IOConfig{
					{WorkTypeName: "request", StateName: "complete"},
					{WorkTypeName: "report", StateName: "init"},
				},
			},
			{
				Name:           "report-finalizer",
				WorkerTypeName: "finalize-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "report", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "report", StateName: "complete"}},
			},
		},
	}
}

// --- State helpers ---

func threeStates(initial, terminal, failed string) []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: initial, Type: interfaces.StateTypeInitial},
		{Name: terminal, Type: interfaces.StateTypeTerminal},
		{Name: failed, Type: interfaces.StateTypeFailed},
	}
}

func fourStates(initial, processing, terminal, failed string) []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: initial, Type: interfaces.StateTypeInitial},
		{Name: processing, Type: interfaces.StateTypeProcessing},
		{Name: terminal, Type: interfaces.StateTypeTerminal},
		{Name: failed, Type: interfaces.StateTypeFailed},
	}
}

func buildTestNet(id string, workTypes []*factoryruntime.WorkType, resources []*factoryruntime.ResourceDef, transitions ...*factoryruntime.PetriTransition) (*factoryruntime.Net, error) {
	n := &factoryruntime.Net{
		ID:          id,
		Places:      make(map[string]*factoryruntime.PetriPlace),
		Transitions: make(map[string]*factoryruntime.PetriTransition),
		WorkTypes:   make(map[string]*factoryruntime.WorkType),
		Resources:   make(map[string]*factoryruntime.ResourceDef),
	}

	for _, wt := range workTypes {
		n.WorkTypes[wt.ID] = wt
		for _, place := range wt.GeneratePlaces() {
			n.Places[place.ID] = place
		}
	}

	for _, resource := range resources {
		n.Resources[resource.ID] = resource
		place, _ := factoryruntime.GenerateResourcePlaces(resource, time.Unix(1, 0))
		n.Places[place.ID] = place
	}

	for _, transition := range transitions {
		n.Transitions[transition.ID] = transition
	}

	factoryruntime.NormalizeTransitionTopology(n, nil)

	validator := validation.NewCompositeValidator(
		&validation.ReachabilityValidator{},
		&validation.CompletenessValidator{},
		&validation.BoundednessValidator{},
		&validation.TypeSafetyValidator{},
	)

	violations := validator.Validate(n)
	errorMessages := make([]string, 0, len(violations))
	for _, violation := range violations {
		if violation.Level != validation.ViolationError {
			continue
		}
		errorMessages = append(errorMessages,
			fmt.Sprintf("%s - %s (at %s)", violation.Code, violation.Message, violation.Location),
		)
	}
	if len(errorMessages) > 0 {
		return nil, fmt.Errorf("net validation failed: %s", strings.Join(errorMessages, "\n "))
	}

	return n, nil
}

func newAutoCategorizedWorkType(id string, states ...string) *factoryruntime.WorkType {
	definitions := make([]factoryruntime.StateDefinition, 0, len(states))
	lastIndex := len(states) - 1
	for index, stateValue := range states {
		category := factoryruntime.StateCategoryProcessing
		switch {
		case index == 0:
			category = factoryruntime.StateCategoryInitial
		case index == lastIndex:
			category = factoryruntime.StateCategoryFailed
		case index == lastIndex-1:
			category = factoryruntime.StateCategoryTerminal
		}
		definitions = append(definitions, factoryruntime.StateDefinition{
			Value:    stateValue,
			Category: category,
		})
	}

	return &factoryruntime.WorkType{
		ID:     id,
		Name:   id,
		States: definitions,
	}
}

func newInputArc(placeID string, bindingName string) factoryruntime.PetriArc {
	return factoryruntime.PetriArc{
		Name:      bindingName,
		PlaceID:   placeID,
		Direction: factoryruntime.PetriArcInput,
		Mode:      interfaces.ArcModeConsume,
		Cardinality: factoryruntime.PetriArcCardinality{
			Mode: factoryruntime.PetriCardinalityOne,
		},
	}
}

func newGuardedInputArc(placeID string, bindingName string, guard factoryruntime.PetriGuard) factoryruntime.PetriArc {
	arc := newInputArc(placeID, bindingName)
	arc.Guard = guard
	return arc
}

func newObserveAllArc(placeID string, bindingName string, guard factoryruntime.PetriGuard) factoryruntime.PetriArc {
	return factoryruntime.PetriArc{
		Name:      bindingName,
		PlaceID:   placeID,
		Direction: factoryruntime.PetriArcInput,
		Mode:      interfaces.ArcModeObserve,
		Guard:     guard,
		Cardinality: factoryruntime.PetriArcCardinality{
			Mode: factoryruntime.PetriCardinalityAll,
		},
	}
}

func newOutputArc(placeID string) factoryruntime.PetriArc {
	return factoryruntime.PetriArc{
		PlaceID:   placeID,
		Direction: factoryruntime.PetriArcOutput,
		Cardinality: factoryruntime.PetriArcCardinality{
			Mode: factoryruntime.PetriCardinalityOne,
		},
	}
}

// --- Structural equivalence assertions ---

// assertStructuralEquivalence compares two nets for structural equivalence.
// It ignores arc IDs and names (which differ between builder and config mapper)
// but verifies: places, transitions, arc connectivity, modes, cardinalities, and guard types.
func assertStructuralEquivalence(t *testing.T, builderNet, configNet *factoryruntime.Net) {
	t.Helper()

	// Compare places.
	comparePlaces(t, builderNet.Places, configNet.Places)

	// Compare transitions.
	if len(builderNet.Transitions) != len(configNet.Transitions) {
		t.Errorf("transition count: builder=%d, config=%d", len(builderNet.Transitions), len(configNet.Transitions))
	}
	for id, bt := range builderNet.Transitions {
		ct := configNet.Transitions[id]
		if ct == nil {
			t.Errorf("config net missing transition %q", id)
			continue
		}
		compareTransitionStructure(t, id, bt, ct)
	}
	for id := range configNet.Transitions {
		if builderNet.Transitions[id] == nil {
			t.Errorf("builder net missing transition %q (extra in config)", id)
		}
	}
}

func comparePlaces(t *testing.T, builderPlaces, configPlaces map[string]*factoryruntime.PetriPlace) {
	t.Helper()
	if len(builderPlaces) != len(configPlaces) {
		t.Errorf("place count: builder=%d, config=%d", len(builderPlaces), len(configPlaces))
	}
	for id, bp := range builderPlaces {
		cp := configPlaces[id]
		if cp == nil {
			t.Errorf("config net missing place %q", id)
			continue
		}
		if bp.TypeID != cp.TypeID {
			t.Errorf("place %q TypeID: builder=%q, config=%q", id, bp.TypeID, cp.TypeID)
		}
		if bp.State != cp.State {
			t.Errorf("place %q State: builder=%q, config=%q", id, bp.State, cp.State)
		}
	}
	for id := range configPlaces {
		if builderPlaces[id] == nil {
			t.Errorf("builder net missing place %q (extra in config)", id)
		}
	}
}

func compareTransitionStructure(t *testing.T, name string, bt, ct *factoryruntime.PetriTransition) {
	t.Helper()
	if bt.Type != ct.Type {
		t.Errorf("transition %q Type: builder=%q, config=%q", name, bt.Type, ct.Type)
	}
	if bt.WorkerType != ct.WorkerType {
		t.Errorf("transition %q WorkerType: builder=%q, config=%q", name, bt.WorkerType, ct.WorkerType)
	}

	compareArcSets(t, name, "input", bt.InputArcs, ct.InputArcs)
	compareArcSets(t, name, "output", bt.OutputArcs, ct.OutputArcs)
	compareArcSets(t, name, "rejection", bt.RejectionArcs, ct.RejectionArcs)
	compareArcSets(t, name, "failure", bt.FailureArcs, ct.FailureArcs)
}

// arcFingerprint is a structural key for matching arcs by their essential properties.
type arcFingerprint struct {
	PlaceID         string
	Mode            interfaces.ArcMode
	CardinalityMode factoryruntime.PetriCardinalityMode
	GuardType       string
}

func fingerprintArc(a factoryruntime.PetriArc) arcFingerprint {
	// Normalize: CardinalityN with Count<=1 is equivalent to CardinalityOne.
	cardMode := a.Cardinality.Mode
	if cardMode == factoryruntime.PetriCardinalityN && a.Cardinality.Count <= 1 {
		cardMode = factoryruntime.PetriCardinalityOne
	}
	return arcFingerprint{
		PlaceID:         a.PlaceID,
		Mode:            a.Mode,
		CardinalityMode: cardMode,
		GuardType:       guardTypeName(a.Guard),
	}
}

func compareArcSets(t *testing.T, transName, arcType string, builderArcs, configArcs []factoryruntime.PetriArc) {
	t.Helper()
	if len(builderArcs) != len(configArcs) {
		t.Errorf("transition %q %s arc count: builder=%d, config=%d",
			transName, arcType, len(builderArcs), len(configArcs))
		return
	}

	// Build fingerprint → arc maps for both sides.
	builderByFP := make(map[arcFingerprint]factoryruntime.PetriArc)
	for _, a := range builderArcs {
		builderByFP[fingerprintArc(a)] = a
	}
	configByFP := make(map[arcFingerprint]factoryruntime.PetriArc)
	for _, a := range configArcs {
		configByFP[fingerprintArc(a)] = a
	}

	for fp, ba := range builderByFP {
		ca, ok := configByFP[fp]
		if !ok {
			t.Errorf("transition %q: builder has %s arc {place=%q, mode=%d, card=%d, guard=%s} not found in config",
				transName, arcType, fp.PlaceID, fp.Mode, fp.CardinalityMode, fp.GuardType)
			continue
		}
		compareGuardParams(t, transName, fp.PlaceID, ba.Guard, ca.Guard)
	}

	for fp := range configByFP {
		if _, ok := builderByFP[fp]; !ok {
			t.Errorf("transition %q: config has %s arc {place=%q, mode=%d, card=%d, guard=%s} not found in builder",
				transName, arcType, fp.PlaceID, fp.Mode, fp.CardinalityMode, fp.GuardType)
		}
	}
}

func guardTypeName(g factoryruntime.PetriGuard) string {
	if g == nil {
		return ""
	}
	switch g.(type) {
	case *factoryruntime.PetriDependencyGuard:
		return ""
	case *factoryruntime.PetriVisitCountGuard:
		return "VisitCountGuard"
	case *factoryruntime.PetriAllWithParentGuard:
		return "AllWithParentGuard"
	case *factoryruntime.PetriAnyWithParentGuard:
		return "AnyWithParentGuard"
	case *factoryruntime.PetriMatchColorGuard:
		return "MatchColorGuard"
	default:
		return fmt.Sprintf("unknown(%T)", g)
	}
}

func compareGuardParams(t *testing.T, transName, placeID string, bg, cg factoryruntime.PetriGuard) {
	t.Helper()
	if bg == nil && cg == nil {
		return
	}

	switch bv := bg.(type) {
	case *factoryruntime.PetriVisitCountGuard:
		cv := cg.(*factoryruntime.PetriVisitCountGuard)
		if bv.TransitionID != cv.TransitionID {
			t.Errorf("transition %q arc to %q: VisitCountGuard.TransitionID builder=%q, config=%q",
				transName, placeID, bv.TransitionID, cv.TransitionID)
		}
		if bv.MaxVisits != cv.MaxVisits {
			t.Errorf("transition %q arc to %q: VisitCountGuard.MaxVisits builder=%d, config=%d",
				transName, placeID, bv.MaxVisits, cv.MaxVisits)
		}
	case *factoryruntime.PetriAllWithParentGuard:
		cv := cg.(*factoryruntime.PetriAllWithParentGuard)
		if bv.MatchBinding != cv.MatchBinding {
			t.Errorf("transition %q arc to %q: AllWithParentGuard.MatchBinding builder=%q, config=%q",
				transName, placeID, bv.MatchBinding, cv.MatchBinding)
		}
	case *factoryruntime.PetriAnyWithParentGuard:
		cv := cg.(*factoryruntime.PetriAnyWithParentGuard)
		if bv.MatchBinding != cv.MatchBinding {
			t.Errorf("transition %q arc to %q: AnyWithParentGuard.MatchBinding builder=%q, config=%q",
				transName, placeID, bv.MatchBinding, cv.MatchBinding)
		}
	}
}
