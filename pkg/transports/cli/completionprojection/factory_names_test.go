package completionprojection_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
)

func TestProjectFactoryNamesPreservesEffectiveCatalogNamesOrderAndMetadata(t *testing.T) {
	catalog := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			factoryNameEntry("@you/goal", "Packaged description"),
			factoryNameEntry("alpha", "  Project description  "),
			{Name: "zeta"},
		},
		Diagnostics: []factorydefinitions.EffectiveFactoryCatalogDiagnostic{{
			Source:  factorydefinitions.EffectiveFactoryCatalogSourceGlobal,
			Name:    "broken",
			Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
			Message: "Factory definition is malformed",
		}},
	}

	got, err := completionprojection.ProjectFactoryNames(context.Background(), catalog)
	if err != nil {
		t.Fatalf("ProjectFactoryNames() error = %v", err)
	}
	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		factoryNameCandidate("@you/goal", "Packaged description"),
		factoryNameCandidate("alpha", "Project description"),
		factoryNameCandidate("zeta", ""),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectFactoryNames() = %#v, want %#v", got, want)
	}
	if len(got.Directives) != 0 {
		t.Fatalf("ProjectFactoryNames() directives = %#v, want none", got.Directives)
	}
}

func TestProjectFactoryNamesIsRepeatableAndDetached(t *testing.T) {
	catalog := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			factoryNameEntry("alpha", "Alpha description"),
			factoryNameEntry("beta", "Beta description"),
		},
	}

	first, err := completionprojection.ProjectFactoryNames(context.Background(), catalog)
	if err != nil {
		t.Fatalf("first ProjectFactoryNames() error = %v", err)
	}
	again, err := completionprojection.ProjectFactoryNames(context.Background(), catalog)
	if err != nil {
		t.Fatalf("repeated ProjectFactoryNames() error = %v", err)
	}
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("repeated projections differ: first=%#v again=%#v", first, again)
	}

	first.Candidates[0].Value = "changed"
	first.Candidates[0].Description = "changed"
	if catalog.Entries[0].Name != "alpha" ||
		catalog.Entries[0].Definition.Description.Value != "Alpha description" {
		t.Fatal("projected candidate mutation changed the effective catalog")
	}
}

func TestProjectFactoryNamesCancellationIsAtomic(t *testing.T) {
	catalog := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			factoryNameEntry("alpha", "Alpha"),
			factoryNameEntry("beta", "Beta"),
			factoryNameEntry("gamma", "Gamma"),
		},
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	for name, ctx := range map[string]context.Context{
		"before": cancelled,
		"during": newCancelAfterErrContext(3),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := completionprojection.ProjectFactoryNames(ctx, catalog)
			if !errors.Is(err, context.Canceled) ||
				!errors.Is(err, completionprojection.ErrCancelled) {
				t.Fatalf("ProjectFactoryNames() error = %v, want cancellation", err)
			}
			if !reflect.DeepEqual(got, completionprojection.Projection{}) {
				t.Fatalf("ProjectFactoryNames() = %#v, want atomic empty result", got)
			}
		})
	}
}

func factoryNameEntry(name string, description string) factorydefinitions.EffectiveFactoryCatalogEntry {
	definition := &factorydefinitions.FactoryConfig{}
	if description != "" {
		definition.Description = &factorydefinitions.NameValueConfig{Value: description}
	}
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Definition: definition,
	}
}

func factoryNameCandidate(name string, description string) completionprojection.Candidate {
	return completionprojection.Candidate{
		Kind:               completionprojection.CandidateKindValue,
		ParameterBindingID: completionprojection.FactoryNameParameterBindingID,
		Value:              name,
		Description:        description,
	}
}
