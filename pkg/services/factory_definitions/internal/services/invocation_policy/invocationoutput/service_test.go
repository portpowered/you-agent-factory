package invocationoutput

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPolicy_InvocationOutput(t *testing.T) {
	t.Parallel()

	output := NewService()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name: "execute-goal",
		Type: factorydefinitions.WorkstationTypeModel,
	}
	if !output.ShouldFormatInvocationSummary(workstation) {
		t.Fatal("ShouldFormatInvocationSummary() = false, want true for goal workstation")
	}

	content, err := output.SummaryContentFromWorkerOutput("Final goal summary.\nCOMPLETE", "COMPLETE")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "Final goal summary." {
		t.Fatalf("summary content = %#v, want normalized goal summary text", content)
	}
}
