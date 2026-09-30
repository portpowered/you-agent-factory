package workstationexecution

import (
	"context"
	"testing"
)

func TestResolveExecutionCatalogAcceptsProviderQualifiedModel(t *testing.T) {
	definition := &FactoryConfig{Workers: []FactoryWorkerConfig{{
		Name: "subagent", Type: "AGENT_WORKER", ModelProvider: "opencode",
		Model: "opencode/muse-spark-1.3-contributor-free",
	}}}
	resolved, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	})
	if err != nil {
		t.Fatalf("resolve qualified OpenCode model: %v", err)
	}
	if got := resolved.Workers["subagent"].Model; got != "opencode/muse-spark-1.3-contributor-free" {
		t.Fatalf("resolved model = %q", got)
	}
}

func TestResolveExecutionCatalogRejectsMalformedQualifiedModel(t *testing.T) {
	definition := &FactoryConfig{Workers: []FactoryWorkerConfig{{
		Name: "subagent", Type: "AGENT_WORKER", ModelProvider: "opencode",
		Model: "opencode//muse-spark-1.3-contributor-free",
	}}}
	if _, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	}); err == nil {
		t.Fatal("malformed qualified model unexpectedly resolved")
	}
}
