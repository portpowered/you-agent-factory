package factory_builder

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// greetingCommandRunner answers the routing classification with "help" and
// captures the prompt the help workstation then receives.
type greetingCommandRunner struct {
	mu           sync.Mutex
	routingCalls int
	helpPrompts  []string
	buildPrompts []string
}

func (runner *greetingCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := string(request.Stdin)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	switch {
	case isBuilderRoutingPrompt(request):
		runner.routingCalls++
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("help")}, nil
	case strings.Contains(prompt, "You are Factory Builder."):
		runner.buildPrompts = append(runner.buildPrompts, prompt)
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("should not build")}, nil
	default:
		runner.helpPrompts = append(runner.helpPrompts, prompt)
		return platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("Factory Builder creates one reusable Factory from a description."),
		}, nil
	}
}

func (runner *greetingCommandRunner) snapshot() (routing int, help, build []string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.routingCalls,
		append([]string(nil), runner.helpPrompts...),
		append([]string(nil), runner.buildPrompts...)
}

// TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding proves problems.md
// 4.1: a customer's first vague message reaches Factory Builder's usage
// guidance instead of immediately attempting to author and install a Factory.
func TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding(t *testing.T) {
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runner := &greetingCommandRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factoryBuilderName, "--no-record",
		"--builder-provider", "CODEX", "--builder-model", "gpt-5",
		"what can you do?",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}

	routing, helpPrompts, buildPrompts := runner.snapshot()
	if routing != 1 {
		t.Fatalf("routing classifications = %d, want exactly 1", routing)
	}
	if len(buildPrompts) != 0 {
		t.Fatalf("build workstation ran %d times for a vague first turn, want 0", len(buildPrompts))
	}
	if len(helpPrompts) != 1 {
		t.Fatalf("help workstation ran %d times, want exactly 1", len(helpPrompts))
	}

	// The guidance is authored, not model-invented, so the worker's prompt is
	// where its accuracy is actually assertable.
	for _, fragment := range []string{
		"you run --named @you/factory-builder --to",
		"--orchestrator graph|javascript",
		"you docs authoring-factories",
		"you docs javascript-workflows",
		"Do not read the workspace or run any command.",
	} {
		if !strings.Contains(helpPrompts[0], fragment) {
			t.Fatalf("help prompt is missing %q; got:\n%s", fragment, helpPrompts[0])
		}
	}
}

// TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing proves a bare
// invocation is admitted and answered rather than rejected for a missing
// required input, which is the CLI analogue of an ACP client's first turn.
func TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing(t *testing.T) {
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runner := &greetingCommandRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factoryBuilderName, "--no-record",
		"--builder-provider", "CODEX", "--builder-model", "gpt-5",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED for a bare invocation", response.Status)
	}
	if _, _, buildPrompts := runner.snapshot(); len(buildPrompts) != 0 {
		t.Fatalf("build workstation ran %d times with no request, want 0", len(buildPrompts))
	}
}
