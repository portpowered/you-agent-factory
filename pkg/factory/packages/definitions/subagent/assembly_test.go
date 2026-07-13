package builtinsubagent

import (
	"strings"
	"testing"
)

func TestAssembleBuiltInSubagentFactoryJSON_RejectsMalformedFactoryJSON(t *testing.T) {
	t.Run("workers not array", func(t *testing.T) {
		root := map[string]any{
			"workers": "not-an-array",
		}
		_, err := assembleBuiltInSubagentFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workers must be an array") {
			t.Fatalf("assemble error = %v, want workers array validation", err)
		}
	})

	t.Run("worker entry not object", func(t *testing.T) {
		root := map[string]any{
			"workers": []any{"not-an-object"},
		}
		_, err := assembleBuiltInSubagentFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "worker entry must be an object") {
			t.Fatalf("assemble error = %v, want worker object validation", err)
		}
	})

	t.Run("workstations not array", func(t *testing.T) {
		root := map[string]any{
			"workers":      []any{},
			"workstations": "not-an-array",
		}
		_, err := assembleBuiltInSubagentFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstations must be an array") {
			t.Fatalf("assemble error = %v, want workstations array validation", err)
		}
	})

	t.Run("workstation entry not object", func(t *testing.T) {
		root := map[string]any{
			"workers":      []any{},
			"workstations": []any{"not-an-object"},
		}
		_, err := assembleBuiltInSubagentFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstation entry must be an object") {
			t.Fatalf("assemble error = %v, want workstation object validation", err)
		}
	})
}

func TestAssembleBuiltInSubagentFactoryJSON_RejectsInvalidFactoryJSON(t *testing.T) {
	_, err := assembleBuiltInSubagentFactoryJSONFromRoot(map[string]any{"workers": []any{}})
	if err == nil || !strings.Contains(err.Error(), "workstations must be an array") {
		t.Fatalf("assemble error = %v, want workstations array validation", err)
	}
}
