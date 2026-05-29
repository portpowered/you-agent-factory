package workers

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidateBuiltInRunnerPrerequisites_UsesExpectedCommand(t *testing.T) {
	originalLookPath := lookPath
	defer func() {
		lookPath = originalLookPath
	}()

	var commands []string
	lookPath = func(file string) (string, error) {
		commands = append(commands, file)
		return "/usr/bin/" + file, nil
	}

	for _, runnerID := range []string{
		interfaces.RunnerIDCodex,
		interfaces.RunnerIDGemini,
		interfaces.RunnerIDKiro,
		interfaces.RunnerIDCursorCLI,
		interfaces.RunnerIDOpenCode,
	} {
		if err := ValidateBuiltInRunnerPrerequisites(runnerID); err != nil {
			t.Fatalf("ValidateBuiltInRunnerPrerequisites(%q): %v", runnerID, err)
		}
	}

	want := []string{
		string(ModelProviderCodex),
		string(ModelProviderGemini),
		string(ModelProviderKiro),
		string(ModelProviderCursor),
		string(ModelProviderOpenCode),
	}
	if len(commands) != len(want) {
		t.Fatalf("lookPath calls = %#v, want %#v", commands, want)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("lookPath command[%d] = %q, want %q", i, commands[i], want[i])
		}
	}
}

func TestValidateBuiltInRunnerPrerequisites_ReportsMissingBinary(t *testing.T) {
	originalLookPath := lookPath
	defer func() {
		lookPath = originalLookPath
	}()

	lookPath = func(file string) (string, error) {
		return "", errors.New("executable file not found in $PATH")
	}

	err := ValidateBuiltInRunnerPrerequisites(interfaces.RunnerIDCursorCLI)
	if err == nil {
		t.Fatal("expected missing binary validation error")
	}
	if !strings.Contains(err.Error(), `Cursor CLI runner requires "agent" on PATH`) {
		t.Fatalf("error = %q, want runner-specific PATH guidance", err.Error())
	}
}
