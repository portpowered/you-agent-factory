package invocationinterpolation

import (
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestInterpolatePromptWithProvenanceKeepsOnlyDeclaredSensitiveSpans(t *testing.T) {
	got, spans, err := InterpolatePromptWithProvenance(
		"prefix=${visible};token=${secret};suffix",
		&work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
			"visible": {Values: []string{"shown"}},
			"secret":  {Values: []string{"hidden"}, Sensitive: true},
		}},
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolatePromptWithProvenance() error = %v", err)
	}
	if got != "prefix=shown;token=hidden;suffix" {
		t.Fatalf("interpolated prompt = %q, want adjacent visible text preserved", got)
	}
	want := []factorydefinitions.InvocationSensitiveTextSpan{{
		Start: len("prefix=shown;token="),
		End:   len("prefix=shown;token=hidden"),
	}}
	if !reflect.DeepEqual(spans, want) {
		t.Fatalf("sensitive spans = %#v, want %#v", spans, want)
	}
}

func TestInterpolatePromptWithProvenanceDoesNotClassifyEmptySensitiveValue(t *testing.T) {
	got, spans, err := InterpolatePromptWithProvenance(
		"before=${secret};after",
		&work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
			"secret": {Values: []string{""}, Sensitive: true},
		}},
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolatePromptWithProvenance() error = %v", err)
	}
	if got != "before=;after" {
		t.Fatalf("interpolated prompt = %q, want empty sensitive value omitted", got)
	}
	if len(spans) != 0 {
		t.Fatalf("sensitive spans = %#v, want none for an empty value", spans)
	}
}

func TestPolicy_InvocationInterpolation(t *testing.T) {
	t.Parallel()

	interpolation := NewService()
	workstation, err := interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${input} now",
		},
		&work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"input": {Values: []string{"draft"}},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig: %v", err)
	}
	if workstation.PromptTemplate != "Use draft now" {
		t.Fatalf("PromptTemplate = %q, want interpolated draft", workstation.PromptTemplate)
	}

	_, err = interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${missing} now",
		},
		&work.InvocationArguments{},
		nil,
	)
	if err == nil {
		t.Fatal("InterpolateWorkstationConfig error = nil, want invalid interpolation")
	}
}
