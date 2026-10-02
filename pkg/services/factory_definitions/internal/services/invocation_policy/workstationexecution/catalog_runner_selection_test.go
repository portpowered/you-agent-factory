package workstationexecution

import (
	"context"
	"errors"
	"testing"
)

// customProviderDefinition models one named ACP peer worker: the authored
// definition names a registered custom provider and no runner at all.
func customProviderDefinition(provider string) *FactoryConfig {
	return &FactoryConfig{
		Workers: []FactoryWorkerConfig{{
			Name: "subagent-worker", Type: "AGENT_WORKER", ModelProvider: provider,
		}},
		Workstations: []FactoryWorkstationConfig{{
			Name: "run-subagent", Type: "AGENT_RUN", WorkerTypeName: "subagent-worker",
		}},
	}
}

func resolveRunnerSelection(
	t *testing.T,
	definition *FactoryConfig,
	references ExecutionCatalogReferenceCatalog,
) (ResolvedWorkstationDefinition, ResolvedWorkerDefinition) {
	t.Helper()
	resolved, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
		References:          references,
	})
	if err != nil {
		t.Fatalf("resolve execution catalog: %v (%+v)", err, resolved.Diagnostics)
	}
	return resolved.Workstations["run-subagent"], resolved.Workers["subagent-worker"]
}

// A registered custom provider is a Providers identity, not a runner identity.
// The implicit legacy compatibility mapping must not promote it to a runner, so
// the default runner applies while the authored model provider identity survives
// for canonical provider resolution downstream.
func TestResolveExecutionCatalogKeepsCustomProviderIdentityOnDefaultRunner(t *testing.T) {
	workstation, worker := resolveRunnerSelection(t, customProviderDefinition("eof-peer"), ExecutionCatalogReferenceCatalog{})
	if worker.ModelProvider != "eof-peer" {
		t.Fatalf("resolved model provider = %q, want %q", worker.ModelProvider, "eof-peer")
	}
	if workstation.Runner != "codex" {
		t.Fatalf("resolved runner = %q, want %q", workstation.Runner, "codex")
	}
	if workstation.RunnerSelectionSource != "default" {
		t.Fatalf("runner selection source = %q, want %q", workstation.RunnerSelectionSource, "default")
	}
}

// The same custom provider must resolve unchanged when composition supplies a
// detached Providers catalog that contains it, and must be rejected when that
// catalog does not. Definitions validates membership only; it never queries
// Providers.
func TestResolveExecutionCatalogChecksCustomProviderMembershipWhenSupplied(t *testing.T) {
	registered := ExecutionCatalogReferenceCatalog{Providers: map[string]struct{}{
		"codex": {}, "eof-peer": {},
	}}
	workstation, worker := resolveRunnerSelection(t, customProviderDefinition("eof-peer"), registered)
	if worker.ModelProvider != "eof-peer" || workstation.Runner != "codex" ||
		workstation.RunnerSelectionSource != "default" {
		t.Fatalf("registered custom provider resolved as worker=%+v workstation=%+v", worker, workstation)
	}

	_, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: customProviderDefinition("customprovider"),
		References:          ExecutionCatalogReferenceCatalog{Providers: map[string]struct{}{"codex": {}}},
	})
	assertDiagnostic(t, err, ExecutionCatalogDiagnosticUnknownProvider, "workers.subagent-worker.modelProvider")
}

// Built-in compatibility aliases keep their legacy runner mapping.
func TestResolveExecutionCatalogPreservesBuiltInProviderRunnerFallback(t *testing.T) {
	for _, test := range []struct {
		provider string
		want     string
	}{
		{provider: "claude", want: "claude"},
		{provider: "agy", want: "antigravity"},
		{provider: "OpenCode", want: "opencode"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			workstation, worker := resolveRunnerSelection(t, customProviderDefinition(test.provider), ExecutionCatalogReferenceCatalog{})
			if worker.ModelProvider != test.provider {
				t.Fatalf("resolved model provider = %q, want %q", worker.ModelProvider, test.provider)
			}
			if workstation.Runner != test.want {
				t.Fatalf("resolved runner = %q, want %q", workstation.Runner, test.want)
			}
			if workstation.RunnerSelectionSource != "legacy_provider" {
				t.Fatalf("runner selection source = %q, want %q", workstation.RunnerSelectionSource, "legacy_provider")
			}
		})
	}
}

// An explicitly authored runner stays a built-in identity requirement; the
// implicit fallback repair must not authorize arbitrary runner identities.
func TestResolveExecutionCatalogRejectsExplicitUnknownRunner(t *testing.T) {
	definition := customProviderDefinition("codex")
	definition.Workstations[0].Runner = "eof-peer"
	_, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	})
	assertDiagnostic(t, err, ExecutionCatalogDiagnosticUnknownRunner, "workstations.run-subagent.runner")
}

// An authored factory runner default is validated the same way and is not
// repaired by the implicit compatibility fallback.
func TestResolveExecutionCatalogRejectsUnknownFactoryRunner(t *testing.T) {
	definition := customProviderDefinition("eof-peer")
	definition.Runner = "eof-peer"
	_, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	})
	assertDiagnostic(t, err, ExecutionCatalogDiagnosticUnknownRunner, "definition.runner")
}

func assertDiagnostic(
	t *testing.T,
	err error,
	wantCode ExecutionCatalogDiagnosticCode,
	wantPath string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %q diagnostic at %q", wantCode, wantPath)
	}
	catalogError := &ExecutionCatalogError{}
	if !errors.As(err, &catalogError) {
		t.Fatalf("error %v is not an ExecutionCatalogError", err)
	}
	for _, diagnostic := range catalogError.Diagnostics {
		if diagnostic.Code == wantCode && diagnostic.Path == wantPath {
			return
		}
	}
	t.Fatalf("diagnostics %+v do not contain %q at %q", catalogError.Diagnostics, wantCode, wantPath)
}
