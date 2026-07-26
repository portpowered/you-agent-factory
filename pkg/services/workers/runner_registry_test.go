package workers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

type runnerExecutableLocatorFunc func(string) (string, error)

func (locate runnerExecutableLocatorFunc) LookPath(command string) (string, error) {
	return locate(command)
}

func TestCloneProviderInferenceRequestDeeplyDetachesInputTokens(t *testing.T) {
	t.Parallel()

	source := ProviderInferenceRequest{
		InputTokens: []any{
			[]any{
				map[string]any{
					"strings": []string{"alpha"},
					"bytes":   []byte("beta"),
					"labels":  map[string]string{"kind": "original"},
					"groups":  map[string][]string{"items": {"first"}},
					"scalar":  "unchanged",
				},
			},
		},
	}

	cloned := CloneProviderInferenceRequest(source)
	clonedMap := cloned.InputTokens[0].([]any)[0].(map[string]any)
	clonedMap["strings"].([]string)[0] = "changed"
	clonedMap["bytes"].([]byte)[0] = 'X'
	clonedMap["labels"].(map[string]string)["kind"] = "changed"
	clonedMap["groups"].(map[string][]string)["items"][0] = "changed"
	clonedMap["scalar"] = "changed"

	sourceMap := source.InputTokens[0].([]any)[0].(map[string]any)
	if got := sourceMap["strings"].([]string)[0]; got != "alpha" {
		t.Fatalf("source strings = %q, want detached original", got)
	}
	if got := string(sourceMap["bytes"].([]byte)); got != "beta" {
		t.Fatalf("source bytes = %q, want detached original", got)
	}
	if got := sourceMap["labels"].(map[string]string)["kind"]; got != "original" {
		t.Fatalf("source label = %q, want detached original", got)
	}
	if got := sourceMap["groups"].(map[string][]string)["items"][0]; got != "first" {
		t.Fatalf("source group = %q, want detached original", got)
	}
	if got := sourceMap["scalar"]; got != "unchanged" {
		t.Fatalf("source scalar = %#v, want original", got)
	}
}

func TestRequestClonesNormalizeEmptyInputTokensToNil(t *testing.T) {
	t.Parallel()

	if got := CloneWorkstationExecutionRequest(WorkstationExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("workstation input tokens = %#v, want nil", got)
	}
	if got := CloneProviderInferenceRequest(ProviderInferenceRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("provider input tokens = %#v, want nil", got)
	}
	if got := CloneSubprocessExecutionRequest(SubprocessExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("subprocess input tokens = %#v, want nil", got)
	}
}

func TestCloneProviderInferenceRequestPreservesScalarInputTokenValues(t *testing.T) {
	t.Parallel()

	source := ProviderInferenceRequest{
		InputTokens: []any{"text", float64(3), true, nil},
	}
	cloned := CloneProviderInferenceRequest(source)

	if !reflect.DeepEqual(cloned.InputTokens, source.InputTokens) {
		t.Fatalf("cloned input tokens = %#v, want %#v", cloned.InputTokens, source.InputTokens)
	}
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
