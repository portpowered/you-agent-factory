package agent_test

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot proves the
// standalone Workers root composes and executes without opening Factory Runtime
// or a Factory Session. The direct Execute boundary has no Process.Execute
// transport representation, so this is intentionally a public root test.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: functionalStatelessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-functional",
			RuntimeID:        "runtime-functional",
			GenerationID:     "generation-functional",
			DispatchID:       "functional-stateless-dispatch",
			AttemptID:        "functional-stateless-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "functional-stateless-script",
		},
	})
	if err != nil {
		t.Fatalf("stateless Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "functional-stateless-output" {
		t.Fatalf("stateless result = %#v, want accepted functional output", result)
	}

	promptTemplates, ok := service.(workers.PromptTemplates)
	if !ok {
		t.Fatal("stateless Workers root does not expose prompt template contracts")
	}
	contract := promptTemplates.BuildPromptTemplateContract(1, []string{"factory/docs/guide.md"})
	if contract.InputCount != 1 || len(contract.AvailableVariables) == 0 {
		t.Fatalf("prompt contract = %#v, want selected input variables", contract)
	}
	validation := promptTemplates.ValidatePromptTemplate("{{ .Context.Project }}", 1, nil)
	if !validation.Valid || len(validation.Diagnostics) != 0 {
		t.Fatalf("prompt validation = %#v, want valid detached template", validation)
	}
	fieldResolver, ok := service.(interface {
		ResolveTemplateFields(
			string,
			map[string]string,
			[]workers.Token,
			*workers.Context,
			string,
		) (*workers.ResolvedTemplateFields, error)
	})
	if !ok {
		t.Fatal("stateless Workers root does not expose template field resolution")
	}
	fields, err := fieldResolver.ResolveTemplateFields(
		"{{.Context.WorkDir}}",
		map[string]string{"TOKEN": "{{.Context.Project}}"},
		nil,
		&workers.Context{WorkDirectory: "/workspace", ProjectID: "project-1"},
		"",
	)
	if err != nil || fields.WorkingDirectory != "/workspace" || fields.Env["TOKEN"] != "project-1" {
		t.Fatalf("resolved fields = %#v, error = %v, want detached context values", fields, err)
	}
	if recorder, ok := service.(interface{ RuntimeOwnsModelEventRecording() bool }); !ok || !recorder.RuntimeOwnsModelEventRecording() {
		t.Fatal("stateless Workers root does not own model event recording")
	}

	if service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderRegistrations: []providerswire.Registration{{
			Manifest:    providerswire.Manifest{ID: "codex"},
			Integration: providerswire.ProgressingExternalIntegration("codex", "unused"),
		}},
	}); service != nil || err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("invalid stateless provider registration = (%#v, %v), want provider registry validation failure", service, err)
	}
}

type functionalStatelessCommandRunner struct{}

func (functionalStatelessCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("functional-stateless-output")}, nil
}
