package workflowvalidation

import (
	"strings"
	"testing"
)

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

func TestValidateAgentRunPermissionsEnumShape(t *testing.T) {
	t.Parallel()

	for _, permissions := range []string{"DEFAULT", "SKIP_PERMISSIONS"} {
		accepted := Validate(Request{
			Source:    `return agent.run({ prompt: "review", permissions: "` + permissions + `" });`,
			SourceRef: "workflow.js",
		})
		if accepted.HasIssues() {
			t.Fatalf("Validate(%q) issues = %#v, want none", permissions, accepted.Issues)
		}
	}
	singleQuoted := Validate(Request{
		Source:    `return agent.run({ prompt: 'review', permissions: 'DEFAULT' });`,
		SourceRef: "workflow.js",
	})
	if singleQuoted.HasIssues() {
		t.Fatalf("Validate(single-quoted permissions) issues = %#v, want none", singleQuoted.Issues)
	}

	for _, source := range []string{
		`return agent.run({ prompt: "review", permissions: "READ_ONLY" });`,
		`return agent.run({ prompt: "review", permissions: true });`,
	} {
		rejected := Validate(Request{Source: source, SourceRef: "workflow.js"})
		if !rejected.HasIssues() {
			t.Fatalf("Validate(%q) issues = nil, want permissions diagnostic", source)
		}
		if rejected.Issues[0].Message == "" || !strings.Contains(rejected.Issues[0].Message, "permissions") {
			t.Fatalf("Validate(%q) issue = %#v, want field-specific permissions diagnostic", source, rejected.Issues[0])
		}
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
