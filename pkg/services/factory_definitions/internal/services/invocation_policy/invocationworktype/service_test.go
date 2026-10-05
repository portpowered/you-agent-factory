package invocationworktype

import (
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDefaultWorkTypeRequiresOneDefault(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config *definitions.FactoryConfig
		want   string
	}{
		{name: "nil"},
		{name: "missing", config: &definitions.FactoryConfig{}},
		{name: "single", config: &definitions.FactoryConfig{WorkTypes: []definitions.WorkTypeConfig{{Name: "ignored", HandlingBehavior: []string{"QUEUE"}}, {Name: "task", HandlingBehavior: []string{"DEFAULT"}}}}, want: "task"},
		{name: "duplicate", config: &definitions.FactoryConfig{WorkTypes: []definitions.WorkTypeConfig{{Name: "a", HandlingBehavior: []string{"DEFAULT"}}, {Name: "b", HandlingBehavior: []string{"DEFAULT"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Service{}).DefaultWorkType(tc.config)
			if got != tc.want || (err == nil) != (tc.want != "") {
				t.Fatalf("DefaultWorkType() = %q, %v", got, err)
			}
		})
	}
}
