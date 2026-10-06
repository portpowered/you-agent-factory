package customer_journeys_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	CliFactoryRunOutputGoalFactoryName = "@you/goal"
	primaryResult                      = "mock worker accepted"
)

func TestSuccessfulInvocationOutputModes(t *testing.T) {
	t.Parallel()

	t.Run("human lifecycle followed by final response", func(t *testing.T) {
		t.Parallel()

		stdout, stderr := runGoalInvocation(t, nil, []string{"--output", "response-stream"})
		assertStableWorkerProgress(t, stderr)

		lines := nonEmptyLines(stdout)
		if len(lines) < 3 {
			t.Fatalf("stdout lines = %#v, want lifecycle, separator, and final response", lines)
		}
		if lines[len(lines)-2] != "--- primary result ---" {
			t.Fatalf("penultimate stdout line = %q, want primary-result separator\nstdout:\n%s", lines[len(lines)-2], stdout)
		}
		if lines[len(lines)-1] != primaryResult {
			t.Fatalf("final stdout line = %q, want %q\nstdout:\n%s", lines[len(lines)-1], primaryResult, stdout)
		}
		for _, line := range lines[:len(lines)-2] {
			if !isFactoryLifecycleLine(line) {
				t.Fatalf("stdout line %q is not canonical customer lifecycle output\nstdout:\n%s", line, stdout)
			}
		}
	})

	t.Run("quiet raw final result", func(t *testing.T) {
		t.Parallel()

		stdout, stderr := runGoalInvocation(t, nil, []string{"--quiet"})

		if stdout != primaryResult {
			t.Fatalf("stdout = %q, want only raw final result %q", stdout, primaryResult)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want quiet mode to suppress progress", stderr)
		}
	})
}

func runGoalInvocation(t *testing.T, globalArgs, runArgs []string) (string, string) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	providerRunner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("{\"decision\":\"accepted\",\"feedback\":\"\",\"output\":\"mock worker accepted\"}"),
	})
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: providerRunner})
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(homeDir, "models"))
	support.InstallPackagedFactoryWithProcess(t, process, env, workingDirectory, CliFactoryRunOutputGoalFactoryName)

	args := []string{"you"}
	args = append(args, globalArgs...)
	args = append(args,
		"run", "--named", CliFactoryRunOutputGoalFactoryName,
		"--executor-provider", "codex",
		"--executor-model", "gpt-5-codex",
		"--no-record",
	)
	args = append(args, runArgs...)
	args = append(args, "deterministic output contract")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout(), inputs.Stderr()
}

func assertStableWorkerProgress(t *testing.T, stderr string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("redirected progress = %q, want only stdout lifecycle milestones", stderr)
	}
}

func nonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func isFactoryLifecycleLine(line string) bool {
	closingBracket := strings.Index(line, "] ")
	if !strings.HasPrefix(line, "[") || closingBracket < 2 {
		return false
	}
	message := line[closingBracket+2:]
	for _, prefix := range []string{
		"work accepted", "work moved", "factory started", "factory completed",
		"workstation queued", "workstation started", "workstation completed", "workstation failed", "workstation interrupted",
		"inference started", "inference completed", "inference failed", "workflow phase", "workflow checkpoint written",
		"final output updated",
	} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}
