package completionprojection_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
)

func TestProjectFlagsPreservesEffectiveSchemaOrderAndAliases(t *testing.T) {
	schema := effectiveSchema(
		namedParameter("alpha", "first", []string{"a", "one"}),
		namedParameter("beta", "second", []string{"b"}),
	)

	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{Target: completionprojection.TargetFlags},
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}

	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		flagCandidate("alpha", "--first", "alpha description"),
		flagCandidate("alpha", "--a", "alpha description"),
		flagCandidate("alpha", "--one", "alpha description"),
		flagCandidate("beta", "--second", "beta description"),
		flagCandidate("beta", "--b", "beta description"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectFlagsDoesNotInventFlagsForPositionalOrStdinBindings(t *testing.T) {
	schema := effectiveSchema(
		climanifest.EffectiveFactoryParameter{
			BindingID:             "position",
			PreferredExternalName: "position",
			Aliases:               []string{"p"},
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind:     work.InvocationParameterBindingKindPositional,
				Position: 1,
			}},
		},
		climanifest.EffectiveFactoryParameter{
			BindingID:             "stdin",
			PreferredExternalName: "stdin",
			Aliases:               []string{"s"},
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind: work.InvocationParameterBindingKindStdin,
			}},
		},
		namedParameter("named", "query", []string{"q"}),
	)

	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{Target: completionprojection.TargetFlags},
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		flagCandidate("named", "--query", "named description"),
		flagCandidate("named", "--q", "named description"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectFlagsHasSelectionParityAndIsRepeatable(t *testing.T) {
	namedSelection := effectiveSchema(
		namedParameter("format", "format", []string{"f"}),
		namedParameter("query", "search", []string{"q", "find"}),
	)
	explicitFileSelection := cloneSchema(namedSelection)
	completionContext := completionprojection.Context{
		Target:        completionprojection.TargetFlags,
		EnteredPrefix: "--se",
	}

	named, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("named Project() error = %v", err)
	}
	fromFile, err := completionprojection.Project(context.Background(), explicitFileSelection, completionContext)
	if err != nil {
		t.Fatalf("explicit-file Project() error = %v", err)
	}
	again, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("repeated Project() error = %v", err)
	}
	if !reflect.DeepEqual(named, fromFile) || !reflect.DeepEqual(named, again) {
		t.Fatalf("equivalent projections differ: named=%#v file=%#v again=%#v", named, fromFile, again)
	}

	named.Candidates[0].Value = "--changed"
	if explicitFileSelection.FactoryParameters[0].PreferredExternalName != "format" {
		t.Fatal("projection shares mutable state with the effective schema")
	}
	repeatedAfterMutation, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("Project() after result mutation error = %v", err)
	}
	if !reflect.DeepEqual(repeatedAfterMutation, again) {
		t.Fatalf("result mutation changed repeated projection: got=%#v want=%#v", repeatedAfterMutation, again)
	}
}

func effectiveSchema(parameters ...climanifest.EffectiveFactoryParameter) climanifest.EffectiveInputSchema {
	return climanifest.EffectiveInputSchema{
		CommandID:         "you.run",
		FactoryInputMode:  climanifest.EffectiveFactoryInputModeSignature,
		FactoryParameters: parameters,
	}
}

func namedParameter(bindingID, externalName string, aliases []string) climanifest.EffectiveFactoryParameter {
	return climanifest.EffectiveFactoryParameter{
		BindingID:             bindingID,
		CanonicalName:         bindingID,
		PreferredExternalName: externalName,
		Aliases:               aliases,
		Description:           bindingID + " description",
		Bindings: []work.InvocationParameterBindingConfig{{
			Kind: work.InvocationParameterBindingKindNamed,
		}},
	}
}

func flagCandidate(bindingID, value, description string) completionprojection.Candidate {
	return completionprojection.Candidate{
		Kind:               completionprojection.CandidateKindFlag,
		ParameterBindingID: bindingID,
		Value:              value,
		Description:        description,
	}
}

func cloneSchema(schema climanifest.EffectiveInputSchema) climanifest.EffectiveInputSchema {
	cloned := schema
	cloned.FactoryParameters = make([]climanifest.EffectiveFactoryParameter, len(schema.FactoryParameters))
	for index, parameter := range schema.FactoryParameters {
		cloned.FactoryParameters[index] = parameter
		cloned.FactoryParameters[index].Aliases = append([]string(nil), parameter.Aliases...)
		cloned.FactoryParameters[index].Bindings = append(
			[]work.InvocationParameterBindingConfig(nil),
			parameter.Bindings...,
		)
	}
	return cloned
}
