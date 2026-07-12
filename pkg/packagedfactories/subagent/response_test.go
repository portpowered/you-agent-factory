package subagent

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestShouldFormatInvocationResponse_MatchesPackagedRunSubagentWorkstation(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           PackagedRunWorkstationName,
		Type:           interfaces.WorkstationTypeAgent,
		WorkerTypeName: PackagedWorkerName,
	}
	if !ShouldFormatInvocationResponse(workstation) {
		t.Fatal("expected packaged run-subagent workstation to format invocation response")
	}
}

func TestShouldFormatInvocationResponse_RejectsUnrelatedWorkstations(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           "other-workstation",
		Type:           interfaces.WorkstationTypeAgent,
		WorkerTypeName: PackagedWorkerName,
	}
	if ShouldFormatInvocationResponse(workstation) {
		t.Fatal("expected unrelated workstation not to format invocation response")
	}
}

func TestResponseContentFromWorkerOutput_NormalizesStopToken(t *testing.T) {
	content, err := ResponseContentFromWorkerOutput("mock worker accepted\nCOMPLETE", "COMPLETE")
	if err != nil {
		t.Fatalf("ResponseContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "mock worker accepted" {
		t.Fatalf("content = %#v, want normalized agent response text", content)
	}
}
