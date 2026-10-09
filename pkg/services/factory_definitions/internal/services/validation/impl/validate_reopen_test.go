package impl

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestReopenPolicyValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*factorydefinitions.StateConfig)
		want   string
	}{
		{"valid", func(*factorydefinitions.StateConfig) {}, ""},
		{"absent", func(s *factorydefinitions.StateConfig) { s.OnReopen = nil }, ""},
		{"terminal source", func(s *factorydefinitions.StateConfig) { s.Type = factorydefinitions.StateTypeTerminal }, "source"},
		{"failed source", func(s *factorydefinitions.StateConfig) { s.Type = factorydefinitions.StateTypeFailed }, "source"},
		{"zero budget", func(s *factorydefinitions.StateConfig) { s.OnReopen.MaxWaits = 0 }, "positive"},
		{"negative budget", func(s *factorydefinitions.StateConfig) { s.OnReopen.MaxWaits = -1 }, "positive"},
		{"blank retry", func(s *factorydefinitions.StateConfig) { s.OnReopen.State = " " }, "name destinations"},
		{"self retry", func(s *factorydefinitions.StateConfig) { s.OnReopen.State = "waiting" }, "distinct"},
		{"self exhaust", func(s *factorydefinitions.StateConfig) { s.OnReopen.ExhaustedState = "waiting" }, "distinct"},
		{"equal targets", func(s *factorydefinitions.StateConfig) { s.OnReopen.ExhaustedState = "ready" }, "distinct"},
		{"missing retry", func(s *factorydefinitions.StateConfig) { s.OnReopen.State = "other" }, "same work type"},
		{"terminal retry", func(s *factorydefinitions.StateConfig) { s.OnReopen.State = "complete" }, "PROCESSING"},
		{"processing exhaust", func(s *factorydefinitions.StateConfig) {
			s.OnReopen.ExhaustedState = "ready"
			s.OnReopen.State = "another"
		}, "FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := factorydefinitions.StateConfig{Name: "waiting", Type: factorydefinitions.StateTypeProcessing,
				OnReopen: &factorydefinitions.StateReopenConfig{State: "ready", MaxWaits: 3, ExhaustedState: "failed"}}
			tc.change(&state)
			states := []factorydefinitions.StateConfig{state,
				{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				{Name: "another", Type: factorydefinitions.StateTypeProcessing},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed}}
			cfg := &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{Name: "mission", States: states}}}
			got := workStateReopenTargets(cfg)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected diagnostics: %#v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Code != CodeWorkStateInvalidReopen || !strings.Contains(got[0].Message, tc.want) {
				t.Fatalf("diagnostics = %#v, want %q", got, tc.want)
			}
			if got[0].Path != "factory.workTypes[0].states[0].onReopen" || got[0].Severity != SeverityError {
				t.Fatalf("diagnostic did not identify authored rule: %#v", got[0])
			}
		})
	}
}
