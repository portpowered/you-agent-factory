package cobracompletion_test

import (
	"context"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/spf13/cobra"
)

func TestFactoryNamesPreservesCatalogOrderFiltersPrefixAndProjectsDescriptions(t *testing.T) {
	var gotRequest factorydefinitions.ListEffectiveFactoriesRequest
	complete := cobracompletion.NewFactoryNames(func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		gotRequest = request
		return factorydefinitions.ListEffectiveFactoriesResult{
			Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
				completionFactoryEntry("alpha", "Alpha Factory"),
				completionFactoryEntry("alpine", ""),
				completionFactoryEntry("beta", "Beta Factory"),
			},
			Diagnostics: []factorydefinitions.EffectiveFactoryCatalogDiagnostic{{
				Name: "broken",
				Code: factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
			}},
		}, nil
	})

	got, directive := complete(t.Context(), cobracompletion.FactoryNamesRequest{
		ProjectRoot:   "project",
		GlobalRoot:    "global",
		EnteredPrefix: "al",
	})

	want := []cobra.Completion{
		cobra.CompletionWithDesc("alpha", "Alpha Factory"),
		"alpine",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("completion = %#v, want %#v", got, want)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want no-file completion", directive)
	}
	if gotRequest.ProjectRoot != "project" || gotRequest.GlobalRoot != "global" {
		t.Fatalf("catalog request = %#v", gotRequest)
	}
}

func TestFactoryNamesReturnsAtomicSensitiveSafeFailure(t *testing.T) {
	calls := 0
	complete := cobracompletion.NewFactoryNames(func(
		ctx context.Context,
		_ factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		calls++
		if calls == 1 {
			return factorydefinitions.ListEffectiveFactoriesResult{}, context.Canceled
		}
		cancelContext(t, ctx)
		return factorydefinitions.ListEffectiveFactoriesResult{
			Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
				completionFactoryEntry("partial-secret", "must not escape"),
			},
		}, nil
	})

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{name: "before discovery", ctx: cancelled},
		{name: "discovery error", ctx: t.Context()},
		{name: "during discovery", ctx: withCancelHook(t.Context())},
	}
	for _, test := range contexts {
		t.Run(test.name, func(t *testing.T) {
			got, directive := complete(test.ctx, cobracompletion.FactoryNamesRequest{})
			assertAtomicFailure(t, got, directive)
		})
	}
	if calls != 2 {
		t.Fatalf("catalog calls = %d, want pre-cancellation to skip discovery", calls)
	}
}

func assertAtomicFailure(
	t *testing.T,
	got []cobra.Completion,
	directive cobra.ShellCompDirective,
) {
	t.Helper()
	if len(got) != 0 {
		t.Fatalf("completion = %#v, want atomic empty result", got)
	}
	wantDirective := cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	if directive != wantDirective {
		t.Fatalf("directive = %v, want %v", directive, wantDirective)
	}
}

func cancelContext(t *testing.T, ctx context.Context) {
	t.Helper()
	cancel, ok := ctx.Value(cancelContextKey{}).(context.CancelFunc)
	if !ok {
		t.Fatal("test context has no cancellation function")
	}
	cancel()
}

type cancelContextKey struct{}

func completionFactoryEntry(
	name string,
	description string,
) factorydefinitions.EffectiveFactoryCatalogEntry {
	definition := &factorydefinitions.FactoryConfig{}
	if description != "" {
		definition.Description = &factorydefinitions.NameValueConfig{Value: description}
	}
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Definition: definition,
	}
}
