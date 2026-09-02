package docs_test

import (
	"strings"
	"testing"
)

// TestDocsAliasReturnsCanonicalCustomerDocumentation proves one representative
// public alias resolves to the same usable documentation as its canonical topic.
// Exact topic and alias membership is a static contract/lint concern.
func TestDocsAliasReturnsCanonicalCustomerDocumentation(t *testing.T) {
	canonical := executeDocsCommand(t, "docs", "workstations")
	alias := executeDocsCommand(t, "docs", "workstation")
	if !strings.Contains(alias, "# ") {
		t.Fatalf("you docs workstation returned empty markdown:\n%s", alias)
	}
	if alias != canonical {
		t.Fatal("you docs workstation differs from canonical workstations documentation")
	}
}

func executeDocsCommand(t *testing.T, args ...string) string {
	t.Helper()
	process := documentationProcess(t)
	result := executeDocumentationCommandResult(
		t,
		process.process,
		isolatedDocumentationEnvironment(t),
		process.tempDir(t),
		args...,
	)
	if result.err != nil {
		t.Fatalf("execute root command %v: %v\nstdout:\n%s\nstderr:\n%s", args, result.err, result.stdout, result.stderr)
	}
	return result.stdout
}
