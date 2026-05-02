package workers

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestStandardWorkstationType_Kind(t *testing.T) {
	s := &StandardWorkstationType{}
	if s.Kind() != interfaces.WorkstationKindStandard {
		t.Errorf("expected %q, got %q", interfaces.WorkstationKindStandard, s.Kind())
	}
}

func TestStandardWorkstationType_HandleResult_AlwaysAdvances(t *testing.T) {
	s := &StandardWorkstationType{}

	outcomes := []interfaces.WorkOutcome{interfaces.OutcomeAccepted, interfaces.OutcomeContinue, interfaces.OutcomeRejected, interfaces.OutcomeFailed}
	for _, outcome := range outcomes {
		result := interfaces.WorkResult{Outcome: outcome}
		action := s.HandleResult(result)
		if action != ActionAdvance {
			t.Errorf("outcome %q: expected %q, got %q", outcome, ActionAdvance, action)
		}
	}
}

func TestWorkstationTypeRegistry_DefaultHasStandard(t *testing.T) {
	r := NewWorkstationTypeRegistry()
	if !r.IsValid(interfaces.WorkstationKindStandard) {
		t.Error("expected standard type to be registered by default")
	}
	if !r.IsValid(interfaces.WorkstationKindCron) {
		t.Error("expected cron type to be registered by default")
	}
}

func TestWorkstationTypeRegistry_UnknownTypeInvalid(t *testing.T) {
	r := NewWorkstationTypeRegistry()
	if r.IsValid("unknown_type") {
		t.Error("expected unknown type to be invalid")
	}
}

func TestWorkstationTypeRegistry_Get(t *testing.T) {
	r := NewWorkstationTypeRegistry()
	s, ok := r.Get(interfaces.WorkstationKindStandard)
	if !ok {
		t.Fatal("expected standard type to be found")
	}
	if s.Kind() != interfaces.WorkstationKindStandard {
		t.Errorf("expected %q, got %q", interfaces.WorkstationKindStandard, s.Kind())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent type to not be found")
	}
}

func TestWorkstationTypeRegistry_Register(t *testing.T) {
	r := NewWorkstationTypeRegistry()

	// Register a custom type.
	custom := &mockWorkstationType{kind: "custom"}
	r.Register(custom)

	if !r.IsValid("custom") {
		t.Error("expected custom type to be valid after registration")
	}
	s, ok := r.Get("custom")
	if !ok {
		t.Fatal("expected custom type to be found")
	}
	if s.Kind() != "custom" {
		t.Errorf("expected %q, got %q", "custom", s.Kind())
	}
}

func TestWorkstationTypeRegistry_Kinds(t *testing.T) {
	r := NewWorkstationTypeRegistry()
	kinds := r.Kinds()
	if len(kinds) != 3 {
		t.Fatalf("expected 3 kinds (standard + repeater + cron), got %d", len(kinds))
	}
	foundStandard, foundRepeater, foundCron := false, false, false
	for _, k := range kinds {
		if k == interfaces.WorkstationKindStandard {
			foundStandard = true
		}
		if k == interfaces.WorkstationKindRepeater {
			foundRepeater = true
		}
		if k == interfaces.WorkstationKindCron {
			foundCron = true
		}
	}
	if !foundStandard {
		t.Error("expected standard kind in registry")
	}
	if !foundRepeater {
		t.Error("expected repeater kind in registry")
	}
	if !foundCron {
		t.Error("expected cron kind in registry")
	}
}

func TestRepeaterWorkstationType_Kind(t *testing.T) {
	r := &RepeaterWorkstationType{}
	if r.Kind() != interfaces.WorkstationKindRepeater {
		t.Errorf("expected %q, got %q", interfaces.WorkstationKindRepeater, r.Kind())
	}
}

func TestRepeaterWorkstationType_HandleResult(t *testing.T) {
	r := &RepeaterWorkstationType{}

	tests := []struct {
		outcome interfaces.WorkOutcome
		want    PostResultAction
	}{
		{interfaces.OutcomeContinue, ActionRepeat},
		{interfaces.OutcomeRejected, ActionAdvance},
		{interfaces.OutcomeAccepted, ActionAdvance},
		{interfaces.OutcomeFailed, ActionAdvance},
	}

	for _, tt := range tests {
		result := interfaces.WorkResult{Outcome: tt.outcome}
		got := r.HandleResult(result)
		if got != tt.want {
			t.Errorf("outcome %q: expected %q, got %q", tt.outcome, tt.want, got)
		}
	}
}

func TestCronWorkstationType_HandleResult_AlwaysAdvances(t *testing.T) {
	c := &CronWorkstationType{}
	if c.Kind() != interfaces.WorkstationKindCron {
		t.Errorf("expected %q, got %q", interfaces.WorkstationKindCron, c.Kind())
	}

	for _, outcome := range []interfaces.WorkOutcome{interfaces.OutcomeAccepted, interfaces.OutcomeContinue, interfaces.OutcomeRejected, interfaces.OutcomeFailed} {
		result := interfaces.WorkResult{Outcome: outcome}
		if got := c.HandleResult(result); got != ActionAdvance {
			t.Errorf("outcome %q: expected %q, got %q", outcome, ActionAdvance, got)
		}
	}
}

// mockWorkstationType is a test helper for registry extensibility.
type mockWorkstationType struct {
	kind   interfaces.WorkstationKind
	action PostResultAction
}

func (m *mockWorkstationType) Kind() interfaces.WorkstationKind { return m.kind }
func (m *mockWorkstationType) HandleResult(_ interfaces.WorkResult) PostResultAction {
	if m.action == "" {
		return ActionAdvance
	}
	return m.action
}
