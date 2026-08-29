package shell_completion_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCompletionFailureBoundariesAndReuse(t *testing.T) {
	t.Run("missing and unsupported shells fall back to identical clean completion help", testCompletionHelpFallback)
	t.Run("duplicate shell input is rejected without partial output", testCompletionDuplicateInput)
	t.Run("scripts and dynamic candidates repeat in both directions", testCompletionReuse)
	t.Run("writer failure recovers with a fresh buffer", testCompletionWriterFailureRecovery)
}

func testCompletionHelpFallback(t *testing.T) {
	invalid := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "tcsh")
	empty := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion")
	requireCompletionHelp(t, invalid, "unsupported shell")
	requireCompletionHelp(t, empty, "missing shell")
	if invalid.stdout != empty.stdout {
		t.Fatalf("unsupported and missing shell help differ:\nunsupported:\n%s\nmissing:\n%s", invalid.stdout, empty.stdout)
	}
	valid := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "bash")
	requireCompletionSuccess(t, valid, "retry supported shell")
	if len(strings.TrimSpace(valid.stdout)) < 100 {
		t.Fatalf("retry supported shell output is unexpectedly empty")
	}
}

func testCompletionDuplicateInput(t *testing.T) {
	result := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "bash", "zsh")
	if result.err == nil {
		t.Fatal("completion with duplicate shell arguments completed successfully")
	}
	if result.stdout != "" {
		t.Fatalf("duplicate shell stdout = %q, want empty", result.stdout)
	}
	valid := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "zsh")
	requireCompletionSuccess(t, valid, "retry after duplicate shell")
	if len(strings.TrimSpace(valid.stdout)) < 100 {
		t.Fatalf("retry after duplicate shell output is unexpectedly empty")
	}
}

func testCompletionReuse(t *testing.T) {
	commands := []struct {
		name        string
		args        []string
		withFactory bool
	}{
		{name: "bash", args: []string{"completion", "bash"}},
		{name: "zsh", args: []string{"completion", "zsh"}},
		{name: "powershell", args: []string{"completion", "powershell"}},
		{name: "factory", args: []string{"__complete", "run", "--named", "shell-fi"}, withFactory: true},
		{name: "mode", args: []string{"__complete", "run", "--named", shellFactoryName, "--mode", "j"}, withFactory: true},
		{name: "file", args: []string{"__complete", "run", "--named", shellFactoryName, "--config", "shell-conf"}, withFactory: true},
	}
	first := make(map[string]string, len(commands))
	for _, command := range commands {
		result := executeCompletionCommand(t, completionProcess(t).invocation(t, command.withFactory), command.args...)
		requireCompletionSuccess(t, result, "first "+command.name+" completion")
		first[command.name] = result.stdout
	}
	for index := len(commands) - 1; index >= 0; index-- {
		command := commands[index]
		result := executeCompletionCommand(t, completionProcess(t).invocation(t, command.withFactory), command.args...)
		requireCompletionSuccess(t, result, "repeated "+command.name+" completion")
		if result.stdout != first[command.name] {
			t.Fatalf("repeated %s completion differs or contains prior output", command.name)
		}
	}
}

func testCompletionWriterFailureRecovery(t *testing.T) {
	want := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "bash")
	requireCompletionSuccess(t, want, "baseline bash completion")
	writer := &boundedCompletionWriter{limit: 32}
	var stderr bytes.Buffer
	err := executeCompletionCommandInto(t, completionProcess(t).invocation(t, false), writer, &stderr, "completion", "bash")
	if !errors.Is(err, errCompletionOutput) {
		t.Fatalf("writer failure error = %v, want injected writer error", err)
	}
	if writer.String() == "" || len(writer.String()) >= len(want.stdout) {
		t.Fatalf("bounded writer output length = %d, want partial output below %d", len(writer.String()), len(want.stdout))
	}
	if writer.writes == 0 {
		t.Fatal("writer failure did not attempt output")
	}
	if writer.writes != 1 {
		t.Fatalf("writer failure writes = %d, want one attempt without an internal retry", writer.writes)
	}
	retry := executeCompletionCommand(t, completionProcess(t).invocation(t, false), "completion", "bash")
	requireCompletionSuccess(t, retry, "fresh-buffer bash retry")
	if retry.stdout != want.stdout {
		t.Fatal("fresh-buffer retry differs from clean completion output")
	}
}

var errCompletionOutput = errors.New("shell completion output writer failed")

type boundedCompletionWriter struct {
	buffer bytes.Buffer
	limit  int
	writes int
}

func (writer *boundedCompletionWriter) Write(p []byte) (int, error) {
	writer.writes++
	remaining := writer.limit - writer.buffer.Len()
	if remaining <= 0 {
		return 0, errCompletionOutput
	}
	if remaining < len(p) {
		_, _ = writer.buffer.Write(p[:remaining])
		return remaining, errCompletionOutput
	}
	return writer.buffer.Write(p)
}

func (writer *boundedCompletionWriter) String() string {
	return writer.buffer.String()
}
