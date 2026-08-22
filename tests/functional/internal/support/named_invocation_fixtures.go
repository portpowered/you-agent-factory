package support

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// RequestContainsInterpolation reports whether a provider command edge saw an
// unresolved definition variable in a product-controlled field. Environment
// entries are compared against the Factory-authored placeholders guarded by
// the scenario because the process also inherits unrelated values.
func RequestContainsInterpolation(
	request platformprocess.CommandRequest,
	placeholders ...string,
) bool {
	values := append([]string{request.Command, string(request.Stdin), request.WorkDir}, request.Args...)
	for _, value := range values {
		if strings.Contains(value, "${") {
			return true
		}
	}
	for _, variable := range request.Env {
		for _, placeholder := range placeholders {
			if strings.Contains(variable, placeholder) {
				return true
			}
		}
	}
	return false
}

// CodexDecisionCommandResult returns a deterministic successful Codex stream.
func CodexDecisionCommandResult(output string) platformprocess.CommandResult {
	decision, _ := json.Marshal(map[string]string{
		"decision": "accepted",
		"output":   output,
	})
	message, _ := json.Marshal(string(decision))
	return platformprocess.CommandResult{Stdout: []byte(
		`{"type":"turn.started"}` + "\n" +
			`{"type":"item.completed","item":{"id":"message","type":"agent_message","text":` + string(message) + `}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n",
	)}
}

// RemoveInvocationSignatureFixture turns a materialized Factory into a
// no-signature compatibility fixture without changing its authored files.
func RemoveInvocationSignatureFixture(t testing.TB, factoryPath string) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	delete(factory, "invocationSignature")
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode no-signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write no-signature Factory fixture: %v", err)
	}
}

// ReplaceGoalWorkerInstructions changes the materialized packaged goal worker
// prompt used by invocation interpolation tests.
func ReplaceGoalWorkerInstructions(t testing.TB, factoryPath, instructions string) {
	t.Helper()
	workerInstructions := filepath.Join(
		filepath.Dir(factoryPath),
		"workers",
		"goal-executor",
		"AGENTS.md",
	)
	if err := os.WriteFile(workerInstructions, []byte(instructions), 0o600); err != nil {
		t.Fatalf("write interpolated worker instructions: %v", err)
	}
}

// ReplaceGoalWorkstationPrompt changes the materialized packaged goal prompt
// file so the provider command edge observes the resolved invocation text.
func ReplaceGoalWorkstationPrompt(t testing.TB, factoryPath, prompt string) {
	t.Helper()
	promptPath := filepath.Join(
		filepath.Dir(factoryPath),
		"workstations",
		"execute-goal",
		"prompts",
		"executor.md",
	)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		t.Fatalf("write interpolated workstation prompt: %v", err)
	}
}
