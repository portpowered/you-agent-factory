package token

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestCloneDetachesNestedTokenState(t *testing.T) {
	original := Token{Color: Color{
		Tags:                map[string]string{"owner": "original"},
		Content:             []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`)}},
		InvocationArguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{"input": {Values: []string{"a"}}}},
	}, History: History{TotalVisits: map[string]int{"ready": 1}, FailureLog: []Failure{{Error: "original"}}}}

	clone := Clone(original)
	clone.Color.Tags["owner"] = "clone"
	clone.Color.Content[0].JSON[0] = '['
	clone.Color.InvocationArguments.Arguments["input"] = work.InvocationArgument{Values: []string{"b"}}
	clone.History.TotalVisits["ready"] = 2
	clone.History.FailureLog[0].Error = "clone"

	if original.Color.Tags["owner"] != "original" || string(original.Color.Content[0].JSON) != `{"ok":true}` || original.History.TotalVisits["ready"] != 1 || original.History.FailureLog[0].Error != "original" {
		t.Fatalf("Clone() mutated original nested state: %#v", original)
	}
	if original.Color.InvocationArguments.Arguments["input"].Values[0] != "a" {
		t.Fatalf("Clone() mutated original invocation arguments: %#v", original.Color.InvocationArguments)
	}
}
