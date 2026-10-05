package golangcilintplugin

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
)

func TestPluginConfiguration(t *testing.T) {
	t.Parallel()
	constructor, err := register.GetPlugin("repolint")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := constructor(map[string]any{"defer-stale": []string{"layering"}})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := constructor(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strict.GetLoadMode() != register.LoadModeTypesInfo {
		t.Fatal("plugin must request type information")
	}
	ordinaryRules, _ := ordinary.BuildAnalyzers()
	strictRules, _ := strict.BuildAnalyzers()
	if len(ordinaryRules) == 0 || len(ordinaryRules) != len(strictRules) {
		t.Fatal("configuration must retain the same nonempty rule set")
	}
	for i, rule := range strictRules {
		if rule.Flags.Lookup("check-stale").Value.String() != "true" {
			t.Fatalf("strict %s lost stale checking", rule.Name)
		}
		want := "true"
		if rule.Name == "layering" {
			want = "false"
		}
		if ordinaryRules[i].Flags.Lookup("check-stale").Value.String() != want {
			t.Fatalf("ordinary %s: wrong stale setting", rule.Name)
		}
		if err := rule.Flags.Set("check-stale", "false"); err != nil {
			t.Fatal(err)
		}
		if ordinaryRules[i].Flags.Lookup("check-stale").Value.String() != want {
			t.Fatalf("%s settings leak between invocations", rule.Name)
		}
	}
}

func TestPluginRejectsInvalidSettings(t *testing.T) {
	t.Parallel()
	for _, raw := range []any{
		map[string]any{"unknown": true},
		map[string]any{"defer-stale": "layering"},
		map[string]any{"defer-stale": []string{"missing"}},
		map[string]any{"defer-stale": []string{"layering", "layering"}},
	} {
		if _, err := New(raw); err == nil {
			t.Fatalf("invalid settings accepted: %v", raw)
		}
	}
}
