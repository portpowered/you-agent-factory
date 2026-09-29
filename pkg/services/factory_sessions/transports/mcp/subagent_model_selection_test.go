package factorysession_test

import (
	"context"
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

func TestSubagentQualifiedOpenCodeModelSelectsProvider(t *testing.T) {
	const model = "opencode/muse-spark-1.3-contributor-free"
	for _, test := range []struct {
		name     string
		provider string
	}{
		{name: "provider inferred"},
		{name: "provider explicit", provider: "opencode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := &subagentTargetFake{}
			response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-model" }, mcpfactorysession.SubagentInput{
				Prompt: "Return OK", Provider: test.provider, Model: model,
			})
			if response.Error != nil || response.Result == nil {
				t.Fatalf("Subagent response = %#v", response)
			}
			if got := target.invoke.Args["workerProvider"]; got != "opencode" {
				t.Fatalf("workerProvider = %#v, want opencode", got)
			}
			if got := target.invoke.Args["workerModel"]; got != model {
				t.Fatalf("workerModel = %#v, want %q", got, model)
			}
			if !target.closed {
				t.Fatal("Factory Session was not closed")
			}
		})
	}
}

func TestSubagentQualifiedOpenCodeModelRejectsConflictingProviderBeforeStart(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-model-conflict" }, mcpfactorysession.SubagentInput{
		Prompt: "Return OK", Provider: "codex", Model: "opencode/muse-spark-1.3-contributor-free",
	})
	if response.Result != nil || response.Error == nil || response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("Subagent response = %#v, want bad request", response)
	}
	if !strings.Contains(response.Error.Message, "requires provider opencode") {
		t.Fatalf("error message = %q, want provider conflict", response.Error.Message)
	}
	if target.started {
		t.Fatal("conflicting provider started a Factory Session")
	}
}
