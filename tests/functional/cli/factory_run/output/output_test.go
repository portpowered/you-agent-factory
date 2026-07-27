package output

import (
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	goalFactoryName = "@you/goal"
	primaryResult   = "mock worker accepted"
)

func TestSuccessfulInvocationOutputModes(t *testing.T) {
	t.Run("human lifecycle followed by final response", func(t *testing.T) {
		stdout := runGoalInvocation(t, nil, []string{"--output", "response-stream"})

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
		stdout := runGoalInvocation(t, nil, []string{"--quiet"})

		if stdout != primaryResult {
			t.Fatalf("stdout = %q, want only raw final result %q", stdout, primaryResult)
		}
	})
}

func runGoalInvocation(t *testing.T, globalArgs, runArgs []string) string {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})

	args := []string{"you"}
	args = append(args, globalArgs...)
	args = append(args,
		"run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
	)
	args = append(args, runArgs...)
	args = append(args, "deterministic output contract")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory

	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	return inputs.Stdout()
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
		"work accepted", "work moved", "Factory Session started", "Factory Session completed",
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

