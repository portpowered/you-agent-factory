package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
)

// capturingLogger records all log calls for assertion.
type capturingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level string
	msg   string
	args  []any
}

func (l *capturingLogger) Debug(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "debug", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Info(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "info", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Warn(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "warn", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Error(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "error", msg: msg, args: keysAndValues})
}

func (l *capturingLogger) entryMatches(e *logEntry, substr string) bool {
	if strings.Contains(e.msg, substr) {
		return true
	}
	for _, arg := range e.args {
		if s, ok := arg.(string); ok && strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func (l *capturingLogger) findEntry(substr string) *logEntry {
	for i := range l.entries {
		if l.entryMatches(&l.entries[i], substr) {
			return &l.entries[i]
		}
	}
	return nil
}

func (l *capturingLogger) countEntries(substr string) int {
	count := 0
	for i := range l.entries {
		if l.entryMatches(&l.entries[i], substr) {
			count++
		}
	}
	return count
}

func makeTestSnapshot(tokens map[string]*interfaces.Token) petri.MarkingSnapshot {
	placeTokens := make(map[string][]string)
	for id, tok := range tokens {
		if tok.CreatedAt.IsZero() {
			tok.CreatedAt = time.Now()
		}
		if tok.EnteredAt.IsZero() {
			tok.EnteredAt = time.Now()
		}
		placeTokens[tok.PlaceID] = append(placeTokens[tok.PlaceID], id)
	}
	return petri.MarkingSnapshot{
		Tokens:      tokens,
		PlaceTokens: placeTokens,
	}
}

func TestEnablementEvaluator_LogsEnabledTransition(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "agent",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1", Color: interfaces.TokenColor{WorkID: "w1"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}

	// Should have logged the enabled transition.
	entry := logger.findEntry("transition enabled")
	if entry == nil {
		t.Fatal("expected 'transition enabled' log entry")
	}

	// Should have the summary entry.
	summary := logger.findEntry("evaluation complete")
	if summary == nil {
		t.Fatal("expected 'evaluation complete' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledInsufficientTokens(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "do-work",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}

	entry := logger.findEntry("transition disabled")
	if entry == nil {
		t.Fatal("expected 'transition disabled' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledGuardFailed(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-work":   {ID: "p-work"},
			"p-review": {ID: "p-review"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "merge",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p-work", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID: "a2", Name: "review", PlaceID: "p-review", Direction: petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchColorGuard{
							Field:        "parent_id",
							MatchBinding: "work",
							MatchField:   "work_id",
						},
					},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work":   {ID: "tok-work", PlaceID: "p-work", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-review": {ID: "tok-review", PlaceID: "p-review", Color: interfaces.TokenColor{WorkID: "r1", ParentID: "WRONG"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}

	// Should log guard failure reason.
	entry := logger.findEntry("guard failed")
	if entry == nil {
		t.Fatal("expected log entry containing 'guard failed'")
	}
}

func TestEnablementEvaluator_BindsMultipleNamedGuardedInputs(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-req":    {ID: "p-req"},
			"p-design": {ID: "p-design"},
			"p-code":   {ID: "p-code"},
		},
		Transitions: map[string]*petri.Transition{
			"assemble": {
				ID:   "assemble",
				Name: "assemble",
				InputArcs: []petri.Arc{
					{ID: "request-in", Name: "request", PlaceID: "p-req", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "design-in",
						Name:        "design",
						PlaceID:     "p-design",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchColorGuard{Field: "parent_id", MatchBinding: "request", MatchField: "work_id"},
					},
					{
						ID:          "code-in",
						Name:        "code",
						PlaceID:     "p-code",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchColorGuard{Field: "parent_id", MatchBinding: "request", MatchField: "work_id"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-req":          {ID: "tok-req", PlaceID: "p-req", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-design-match": {ID: "tok-design-match", PlaceID: "p-design", Color: interfaces.TokenColor{WorkID: "d1", ParentID: "w1"}},
		"tok-design-other": {ID: "tok-design-other", PlaceID: "p-design", Color: interfaces.TokenColor{WorkID: "d2", ParentID: "other"}},
		"tok-code-match":   {ID: "tok-code-match", PlaceID: "p-code", Color: interfaces.TokenColor{WorkID: "c1", ParentID: "w1"}},
		"tok-code-other":   {ID: "tok-code-other", PlaceID: "p-code", Color: interfaces.TokenColor{WorkID: "c2", ParentID: "other"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if enabled[0].TransitionID != "assemble" {
		t.Fatalf("enabled transition = %q, want assemble", enabled[0].TransitionID)
	}
	wantBindings := map[string]string{
		"request": "tok-req",
		"design":  "tok-design-match",
		"code":    "tok-code-match",
	}
	for binding, want := range wantBindings {
		got := tokenIDs(enabled[0].Bindings[binding])
		if strings.Join(got, ",") != want {
			t.Fatalf("%s binding tokens = %v, want [%s]", binding, got, want)
		}
	}
}

func TestEnablementEvaluator_BindsAllTokensForMatchingParentGuard(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-parent":   {ID: "p-parent"},
			"p-children": {ID: "p-children"},
		},
		Transitions: map[string]*petri.Transition{
			"join-children": {
				ID:   "join-children",
				Name: "join-children",
				InputArcs: []petri.Arc{
					{ID: "parent-in", Name: "parent", PlaceID: "p-parent", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "children-in",
						Name:        "children",
						PlaceID:     "p-children",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityAll},
						Guard:       &petri.AllWithParentGuard{MatchBinding: "parent"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-parent":      {ID: "tok-parent", PlaceID: "p-parent", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-child-a":     {ID: "tok-child-a", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c1", ParentID: "w1"}},
		"tok-child-b":     {ID: "tok-child-b", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c2", ParentID: "w1"}},
		"tok-child-other": {ID: "tok-child-other", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c3", ParentID: "other"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	gotParent := tokenIDs(enabled[0].Bindings["parent"])
	if strings.Join(gotParent, ",") != "tok-parent" {
		t.Fatalf("parent binding tokens = %v, want [tok-parent]", gotParent)
	}
	gotChildren := tokenIDs(enabled[0].Bindings["children"])
	if strings.Join(gotChildren, ",") != "tok-child-a,tok-child-b" {
		t.Fatalf("children binding tokens = %v, want [tok-child-a tok-child-b]", gotChildren)
	}
}

func TestEnablementEvaluator_SameNameGuardEnablesOnMatchingNames(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-items": {
				ID:   "match-items",
				Name: "match-items",
				InputArcs: []petri.Arc{
					{ID: "plan-in", Name: "plan", PlaceID: "plan:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.SameNameGuard{MatchBinding: "plan"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "beta"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["plan"]); strings.Join(got, ",") != "plan-alpha" {
		t.Fatalf("plan binding tokens = %v, want [plan-alpha]", got)
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-alpha" {
		t.Fatalf("task binding tokens = %v, want [task-alpha]", got)
	}
}

func TestEnablementEvaluator_SameNameGuardBlocksNonMatchingNames(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-items": {
				ID:   "match-items",
				Name: "match-items",
				InputArcs: []petri.Arc{
					{ID: "plan-in", Name: "plan", PlaceID: "plan:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.SameNameGuard{MatchBinding: "plan"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "beta"}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled transitions = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardEnablesSingleInputWhenSelectorResolves(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-single": {
				ID:   "match-single",
				Name: "match-single",
				InputArcs: []petri.Arc{{
					ID:          "task-in",
					Name:        "task",
					PlaceID:     "task:ready",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard:       &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
				}},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"task-alpha": {
			ID:      "task-alpha",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-alpha" {
		t.Fatalf("task binding tokens = %v, want [task-alpha]", got)
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardEnablesOnMatchingTwoInputValues(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-pair": {
				ID:   "match-pair",
				Name: "match-pair",
				InputArcs: []petri.Arc{
					{
						ID:          "plan-in",
						Name:        "plan",
						PlaceID:     "plan:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
					},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchesFieldsGuard{
							InputKey:     `.Tags["_last_output"]`,
							MatchBinding: "plan",
						},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {
			ID:      "plan-alpha",
			PlaceID: "plan:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"task-alpha": {
			ID:      "task-alpha",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"task-beta": {
			ID:      "task-beta",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "beta",
			}},
		},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["plan"]); strings.Join(got, ",") != "plan-alpha" {
		t.Fatalf("plan binding tokens = %v, want [plan-alpha]", got)
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-alpha" {
		t.Fatalf("task binding tokens = %v, want [task-alpha]", got)
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardBlocksMismatchedTwoInputValues(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-pair": {
				ID:   "match-pair",
				Name: "match-pair",
				InputArcs: []petri.Arc{
					{
						ID:          "plan-in",
						Name:        "plan",
						PlaceID:     "plan:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
					},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchesFieldsGuard{
							InputKey:     `.Tags["_last_output"]`,
							MatchBinding: "plan",
						},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {
			ID:      "plan-alpha",
			PlaceID: "plan:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"task-beta": {
			ID:      "task-beta",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "beta",
			}},
		},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled transitions = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardRequiresAllInputsToMatchSourceValue(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready":  {ID: "plan:ready"},
			"task:ready":  {ID: "task:ready"},
			"asset:ready": {ID: "asset:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-triplet": {
				ID:   "match-triplet",
				Name: "match-triplet",
				InputArcs: []petri.Arc{
					{
						ID:          "plan-in",
						Name:        "plan",
						PlaceID:     "plan:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
					},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchesFieldsGuard{
							InputKey:     `.Tags["_last_output"]`,
							MatchBinding: "plan",
						},
					},
					{
						ID:          "asset-in",
						Name:        "asset",
						PlaceID:     "asset:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchesFieldsGuard{
							InputKey:     `.Tags["_last_output"]`,
							MatchBinding: "plan",
						},
					},
				},
			},
		},
	}

	matching := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {
			ID:      "plan-alpha",
			PlaceID: "plan:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"task-alpha": {
			ID:      "task-alpha",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"asset-alpha": {
			ID:      "asset-alpha",
			PlaceID: "asset:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"asset-beta": {
			ID:      "asset-beta",
			PlaceID: "asset:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "beta",
			}},
		},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &matching)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["asset"]); strings.Join(got, ",") != "asset-alpha" {
		t.Fatalf("asset binding tokens = %v, want [asset-alpha]", got)
	}

	mismatched := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {
			ID:      "plan-alpha",
			PlaceID: "plan:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"task-alpha": {
			ID:      "task-alpha",
			PlaceID: "task:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "alpha",
			}},
		},
		"asset-beta": {
			ID:      "asset-beta",
			PlaceID: "asset:ready",
			Color: interfaces.TokenColor{Tags: map[string]string{
				"_last_output": "beta",
			}},
		},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &mismatched); len(enabled) != 0 {
		t.Fatalf("enabled transitions with mismatched third input = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_VisitCountGuardEnablesAtThreshold(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init":   {ID: "p-init"},
			"p-failed": {ID: "p-failed"},
		},
		Transitions: map[string]*petri.Transition{
			"exhaust-review": {
				ID:   "exhaust-review",
				Name: "exhaust-review",
				Type: petri.TransitionExhaustion,
				InputArcs: []petri.Arc{
					{
						ID:          "work-in",
						Name:        "work",
						PlaceID:     "p-init",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.VisitCountGuard{TransitionID: "review", MaxVisits: 3},
					},
				},
				OutputArcs: []petri.Arc{
					{ID: "failed-out", Name: "failed", PlaceID: "p-failed", Direction: petri.ArcOutput},
				},
			},
		},
	}

	belowThreshold := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"review": 2},
			},
		},
	})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &belowThreshold); len(enabled) != 0 {
		t.Fatalf("enabled transitions below threshold = %d, want 0", len(enabled))
	}

	atThreshold := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"review": 3},
			},
		},
	})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &atThreshold)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions at threshold = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["work"]); strings.Join(got, ",") != "tok-work" {
		t.Fatalf("work binding tokens = %v, want [tok-work]", got)
	}
}

func TestEnablementEvaluator_LogsNoInputArcs(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t1": {ID: "t1", Name: "empty", InputArcs: nil},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}

	entry := logger.findEntry("no input arcs")
	if entry == nil {
		t.Fatal("expected log entry containing 'no input arcs'")
	}
}

func TestEnablementEvaluator_NilLoggerDoesNotPanic(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1"},
	})

	// Should not panic with nil logger.
	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}
}

func TestEnablementEvaluator_MultipleTransitions_LogsEach(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
			"p2": {ID: "p2"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "first",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
			"t2": {
				ID:   "t2",
				Name: "second",
				InputArcs: []petri.Arc{
					{ID: "a2", Name: "input", PlaceID: "p2", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	// Only p1 has a token → t1 enabled, t2 disabled.
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1"},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}

	enabledCount := logger.countEntries("transition enabled")
	disabledCount := logger.countEntries("transition disabled")
	if enabledCount != 1 {
		t.Errorf("expected 1 'transition enabled' log, got %d", enabledCount)
	}
	if disabledCount != 1 {
		t.Errorf("expected 1 'transition disabled' log, got %d", disabledCount)
	}
}

func TestEnablementEvaluator_ContextPassedThrough(t *testing.T) {
	// Verify the evaluator accepts context without error. Context is currently
	// threaded through for future use (e.g., cancellation, tracing).
	eval := NewEnablementEvaluator(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := &state.Net{
		Transitions: map[string]*petri.Transition{},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{})

	enabled := eval.FindEnabledTransitions(ctx, n, &marking)
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
}

func TestEnablementEvaluator_UsesInjectedClockForCronTimeWindowGuard(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(2 * time.Minute)
	expiresAt := base.Add(7 * time.Minute)
	currentTime := dueAt.Add(-time.Nanosecond)
	eval := NewEnablementEvaluator(nil, WithEnablementClock(func() time.Time {
		return currentTime
	}))

	n := &state.Net{
		Places: map[string]*petri.Place{
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"cron-refresh": {
				ID:         "cron-refresh",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{
						ID:          "cron-time",
						Name:        "time",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.CronTimeWindowGuard{Workstation: "refresh"},
					},
				},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"time-refresh": schedulerCronTimeToken("time-refresh", "refresh", dueAt, expiresAt),
		},
		PlaceTokens: map[string][]string{
			interfaces.SystemTimePendingPlaceID: {"time-refresh"},
		},
	}

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled before due = %d, want 0", len(enabled))
	}

	currentTime = dueAt
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
		t.Fatalf("enabled at due = %d, want 1", len(enabled))
	}

	currentTime = expiresAt.Add(-time.Nanosecond)
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
		t.Fatalf("enabled before expiry = %d, want 1", len(enabled))
	}

	currentTime = expiresAt
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled at expiry = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_OrdersEnabledTransitionsByID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-alpha": {ID: "p-alpha"},
			"p-beta":  {ID: "p-beta"},
			"p-zeta":  {ID: "p-zeta"},
		},
		Transitions: map[string]*petri.Transition{
			"transition-zeta": {
				ID:         "transition-zeta",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-zeta", Name: "work", PlaceID: "p-zeta", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
			"transition-alpha": {
				ID:         "transition-alpha",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-alpha", Name: "work", PlaceID: "p-alpha", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
			"transition-beta": {
				ID:         "transition-beta",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-beta", Name: "work", PlaceID: "p-beta", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-zeta":  {ID: "tok-zeta", PlaceID: "p-zeta"},
			"tok-alpha": {ID: "tok-alpha", PlaceID: "p-alpha"},
			"tok-beta":  {ID: "tok-beta", PlaceID: "p-beta"},
		},
		PlaceTokens: map[string][]string{
			"p-zeta":  {"tok-zeta"},
			"p-alpha": {"tok-alpha"},
			"p-beta":  {"tok-beta"},
		},
	}

	for i := 0; i < 10; i++ {
		enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
		got := transitionIDs(enabled)
		want := []string{"transition-alpha", "transition-beta", "transition-zeta"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("iteration %d enabled transition order = %v, want %v", i, got, want)
		}
	}
}

func TestEnablementEvaluator_SelectsOrdinaryTokensByStableID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-work": {ID: "p-work"},
		},
		Transitions: map[string]*petri.Transition{
			"transition-work": {
				ID:         "transition-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-work", Name: "work", PlaceID: "p-work", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 2}},
				},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-c": {ID: "tok-c", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"tok-a": {ID: "tok-a", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"tok-b": {ID: "tok-b", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
		},
		PlaceTokens: map[string][]string{
			"p-work": {"tok-c", "tok-a", "tok-b"},
		},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	got := tokenIDs(enabled[0].Bindings["work"])
	want := []string{"tok-a", "tok-b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("bound ordinary tokens = %v, want %v", got, want)
	}
}

func TestEnablementEvaluator_SelectsResourceTokensByStableID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"slot:available": {ID: "slot:available"},
		},
		Transitions: map[string]*petri.Transition{
			"transition-slot": {
				ID:         "transition-slot",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-slot", Name: "slot", PlaceID: "slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"slot-2": {ID: "slot-2", PlaceID: "slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
			"slot-1": {ID: "slot-1", PlaceID: "slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
		},
		PlaceTokens: map[string][]string{
			"slot:available": {"slot-2", "slot-1"},
		},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	got := tokenIDs(enabled[0].Bindings["slot"])
	want := []string{"slot-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("bound resource tokens = %v, want %v", got, want)
	}
}

func TestEnablementEvaluator_ExpandsRepeatedWorkAndResourceBindingsForSameTransition(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":               {ID: "task:init"},
			"executor-slot:available": {ID: "executor-slot:available"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID:         "process",
				WorkerType: "processor",
				InputArcs: []petri.Arc{
					{ID: "work-in", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.DependencyGuard{}},
					{ID: "slot-in", Name: "slot", PlaceID: "executor-slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 1}},
				},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"work-b": {ID: "work-b", PlaceID: "task:init", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"work-a": {ID: "work-a", PlaceID: "task:init", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"slot-2": {ID: "slot-2", PlaceID: "executor-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
			"slot-1": {ID: "slot-1", PlaceID: "executor-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
		},
		PlaceTokens: map[string][]string{
			"task:init":               {"work-b", "work-a"},
			"executor-slot:available": {"slot-2", "slot-1"},
		},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("base enabled candidates = %d, want 1", len(enabled))
	}
	expanded := ExpandRepeatedBindings(n, &marking, enabled)
	if len(expanded) != 2 {
		t.Fatalf("expanded candidates = %d, want 2", len(expanded))
	}
	gotFirst := append(tokenIDs(expanded[0].Bindings["work"]), tokenIDs(expanded[0].Bindings["slot"])...)
	gotSecond := append(tokenIDs(expanded[1].Bindings["work"]), tokenIDs(expanded[1].Bindings["slot"])...)
	if strings.Join(gotFirst, ",") != "work-a,slot-1" {
		t.Fatalf("first candidate tokens = %v, want [work-a slot-1]", gotFirst)
	}
	if strings.Join(gotSecond, ",") != "work-b,slot-2" {
		t.Fatalf("second candidate tokens = %v, want [work-b slot-2]", gotSecond)
	}
}

func schedulerCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			},
		},
	}
}

func transitionIDs(enabled []interfaces.EnabledTransition) []string {
	ids := make([]string, len(enabled))
	for i := range enabled {
		ids[i] = enabled[i].TransitionID
	}
	return ids
}

func tokenIDs(tokens []interfaces.Token) []string {
	ids := make([]string, len(tokens))
	for i := range tokens {
		ids[i] = tokens[i].ID
	}
	return ids
}
