//go:build functionallong

package providers

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestTemplateTests_ScriptWrapClaudeResolvesWorkstationExecutionTemplates(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation execution-template provider smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(modelprovider.ProviderClaude, "test-claude-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)

	req := runner.LastRequest()
	assertCommandArgs(t, req, append([]string{
		"-p",
		"--worktree", "worktrees/feature-token-branch/work-execution-template",
		"--system-prompt", "Process the input task.",
		"--model", "test-claude-model",
	}, executionTemplateWantPrompt(dir)))
	assertProviderStdin(t, req, "")
	assertProviderExecutionFields(t, dir, req)
}

func TestTemplateTests_ScriptWrapCodexResolvesWorkstationExecutionTemplates(t *testing.T) {
	support.SkipLongFunctional(t, "slow codex execution-template provider smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(modelprovider.ProviderCodex, "test-codex-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)

	req := runner.LastRequest()
	assertCommandArgs(t, req, []string{"exec", "--model", "test-codex-model", "-"})
	assertProviderStdin(t, req, executionTemplateWantPrompt(dir))
	assertProviderExecutionFields(t, dir, req)
}
