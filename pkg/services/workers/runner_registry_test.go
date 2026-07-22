package workers

import (
	"errors"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
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
		{RunnerIDCodex, string(modelprovider.ProviderCodex)},
		{RunnerIDGemini, string(modelprovider.ProviderGemini)},
		{RunnerIDKiro, string(modelprovider.ProviderKiro)},
		{RunnerIDCursorCLI, string(modelprovider.ProviderCursor)},
		{RunnerIDOpenCode, string(modelprovider.ProviderOpenCode)},
		{RunnerIDPi, string(modelprovider.ProviderPi)},
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
		RunnerIDCodex,
		RunnerIDGemini,
		RunnerIDKiro,
		RunnerIDCursorCLI,
		RunnerIDOpenCode,
		RunnerIDPi,
	} {
		if err := ValidateBuiltInRunnerPrerequisites(locator, runnerID); err != nil {
			t.Fatalf("ValidateBuiltInRunnerPrerequisites(%q): %v", runnerID, err)
		}
	}

	want := []string{
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderGemini),
		string(modelprovider.ProviderKiro),
		string(modelprovider.ProviderCursor),
		string(modelprovider.ProviderOpenCode),
		string(modelprovider.ProviderPi),
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

	err := ValidateBuiltInRunnerPrerequisites(locator, RunnerIDCursorCLI)
	if err == nil {
		t.Fatal("expected missing binary validation error")
	}
	if !strings.Contains(err.Error(), `Cursor CLI runner requires "agent" on PATH`) {
		t.Fatalf("error = %q, want runner-specific PATH guidance", err.Error())
	}
}

func TestValidateBuiltInRunnerPrerequisites_FailsClosedWithoutExecutableLocator(t *testing.T) {
	err := ValidateBuiltInRunnerPrerequisites(nil, RunnerIDCursorCLI)
	if err == nil || !strings.Contains(err.Error(), "executable locator is required") {
		t.Fatalf("error = %v, want missing executable locator", err)
	}
}
