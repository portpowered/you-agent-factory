package runner

import (
	"errors"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type runnerExecutableLocatorFunc func(string) (string, error)

func (locate runnerExecutableLocatorFunc) LookPath(command string) (string, error) {
	return locate(command)
}

func TestBuiltInRunnerCommand_MatchesInternalModelProviderCLI(t *testing.T) {
	cases := []struct {
		runnerID string
		command  string
	}{
		{workerexecution.RunnerIDCodex, string(modelprovider.ProviderCodex)},
		{workerexecution.RunnerIDClaude, string(modelprovider.ProviderClaude)},
		{workerexecution.RunnerIDAntigravity, "agy"},
	}

	for _, tc := range cases {
		t.Run(tc.runnerID, func(t *testing.T) {
			if got := builtInRunnerCommand(tc.runnerID); got != tc.command {
				t.Fatalf("builtInRunnerCommand(%q) = %q, want %q", tc.runnerID, got, tc.command)
			}
		})
	}
}

func TestValidateBuiltInRunnerPrerequisites_UsesExpectedCommand(t *testing.T) {
	var commands []string
	locator := runnerExecutableLocatorFunc(func(file string) (string, error) {
		commands = append(commands, file)
		return "/usr/bin/" + file, nil
	})

	for _, runnerID := range []string{
		workerexecution.RunnerIDCodex,
		workerexecution.RunnerIDClaude,
		workerexecution.RunnerIDAntigravity,
	} {
		if err := ValidateBuiltInRunnerPrerequisites(locator, runnerID); err != nil {
			t.Fatalf("ValidateBuiltInRunnerPrerequisites(%q): %v", runnerID, err)
		}
	}

	want := []string{
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderClaude),
		"agy",
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
	locator := runnerExecutableLocatorFunc(func(file string) (string, error) {
		return "", errors.New("executable file not found in $PATH")
	})

	err := ValidateBuiltInRunnerPrerequisites(locator, workerexecution.RunnerIDCodex)
	if err == nil {
		t.Fatal("expected missing binary validation error")
	}
	if !strings.Contains(err.Error(), `Codex runner requires "codex" on PATH`) {
		t.Fatalf("error = %q, want runner-specific PATH guidance", err.Error())
	}
}

func TestValidateBuiltInRunnerPrerequisites_FailsClosedWithoutExecutableLocator(t *testing.T) {
	err := ValidateBuiltInRunnerPrerequisites(nil, workerexecution.RunnerIDCodex)
	if err == nil || !strings.Contains(err.Error(), "executable locator is required") {
		t.Fatalf("error = %v, want missing executable locator", err)
	}
}
