package baseline

import (
	"strings"
	"testing"
)

func TestRunWorkflowRemovalContract_RunFlagsBaselineOmitsWorkflow(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}

	for _, serialized := range []struct {
		name string
		text string
	}{
		{name: "fixture", text: mustReadFixtureText(t, "testdata/run_flags.txt")},
		{name: "production", text: observation.Snapshot.RunFlags},
	} {
		t.Run(serialized.name, func(t *testing.T) {
			if runFlagPresent(serialized.text, "workflow") {
				t.Fatalf("%s run flags must not list workflow", serialized.name)
			}
		})
	}
}

func TestRunWorkflowRemovalContract_IntentionalChangesLedgerOmitsRunWorkflowRemoval(t *testing.T) {
	ledger, err := LoadIntentionalChangesLedger(testSourceStore())
	if err != nil {
		t.Fatalf("load intentional changes ledger: %v", err)
	}

	for _, removal := range ledger.PlannedRemovals {
		if removal.Surface == "run_flag" && removal.Name == "workflow" {
			t.Fatalf("planned removal still lists run_flag workflow: %#v", removal)
		}
	}
}

func TestRunWorkflowRemovalContract_RunHelpBaselineDocumentsSupportedContract(t *testing.T) {
	help := mustReadFixtureText(t, "testdata/run_help.txt")

	for _, want := range []string{
		"--dir",
		"--named",
		"--factory",
		"trailing positional text or piped stdin text",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"primary-result-only stdout by default",
		"--output response-stream",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("run help baseline missing %q", want)
		}
	}
	if strings.Contains(help, "run --workflow") {
		t.Fatal("run help baseline must not document run-level --workflow")
	}
}

func mustReadFixtureText(t *testing.T, fixture string) string {
	t.Helper()

	text, err := ReadFixtureText(testSourceStore(), fixture)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	return text
}
