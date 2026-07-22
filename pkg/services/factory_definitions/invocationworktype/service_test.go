package invocationworktype

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDefaultWorkType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *factorydefinitions.FactoryConfig
		want    string
		wantErr string
	}{
		{name: "nil config", wantErr: "factory config is required"},
		{
			name: "single default",
			config: &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{
				{Name: "task"},
				{Name: "story", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
			}},
			want: "story",
		},
		{
			name:    "missing default",
			config:  &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{Name: "story"}}},
			wantErr: "expected exactly one",
		},
		{
			name: "multiple defaults",
			config: &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{
				{Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
				{Name: "story", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
			}},
			wantErr: "found multiple",
		},
	}

	service := NewService()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := service.DefaultWorkType(test.config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("DefaultWorkType() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DefaultWorkType() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("DefaultWorkType() = %q, want %q", got, test.want)
			}
		})
	}
}
