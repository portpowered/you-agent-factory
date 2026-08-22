package workflowvalidation

import "testing"

func TestValidateAcceptsTopLevelReturnLikeRuntimeExecution(t *testing.T) {
	t.Parallel()

	result := Validate(Request{
		Source: `agent.run({ prompt: "review", label: "child" });
return { ok: true };`,
		SourceRef: "workflow.js",
	})
	if result.HasIssues() {
		t.Fatalf("Validate() issues = %#v, want none for workflow script return semantics", result.Issues)
	}
}

func TestValidateAgentRunSkipPermissionsBooleanShape(t *testing.T) {
	t.Parallel()

	accepted := Validate(Request{
		Source:    `return agent.run({ prompt: "review", skipPermissions: true });`,
		SourceRef: "workflow.js",
	})
	if accepted.HasIssues() {
		t.Fatalf("Validate(true) issues = %#v, want none", accepted.Issues)
	}

	rejected := Validate(Request{
		Source:    `return agent.run({ prompt: "review", skipPermissions: "true" });`,
		SourceRef: "workflow.js",
	})
	if !rejected.HasIssues() {
		t.Fatal("Validate(string) issues = nil, want boolean shape error")
	}
}

func TestValidateRemapsSyntaxErrorsToAuthoredLineNumbers(t *testing.T) {
	t.Parallel()

	result := Validate(Request{
		Source:    "workflow.final(\"ok\");\nphase(\"setup\";\n",
		SourceRef: "workflow.js",
	})
	if !result.HasIssues() {
		t.Fatal("Validate() issues = nil, want syntax error")
	}
	if result.Issues[0].Line != 2 {
		t.Fatalf("syntax line = %d, want authored line 2", result.Issues[0].Line)
	}
}
