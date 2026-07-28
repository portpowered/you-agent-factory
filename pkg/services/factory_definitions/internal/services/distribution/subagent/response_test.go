package subagent

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestShouldFormatInvocationResponse_MatchesPackagedRunSubagentWorkstation(t *testing.T) {
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name:           PackagedRunWorkstationName,
		Type:           factorydefinitions.WorkstationTypeAgent,
		WorkerTypeName: PackagedWorkerName,
	}
	if !ShouldFormatInvocationResponse(workstation) {
		t.Fatal("expected packaged run-subagent workstation to format invocation response")
	}
}

func TestShouldFormatInvocationResponse_RejectsUnrelatedWorkstations(t *testing.T) {
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name:           "other-workstation",
		Type:           factorydefinitions.WorkstationTypeAgent,
		WorkerTypeName: PackagedWorkerName,
	}
	if ShouldFormatInvocationResponse(workstation) {
		t.Fatal("expected unrelated workstation not to format invocation response")
	}
}

func TestShouldFormatInvocationResponse_RejectsNilAndNonAgentTypes(t *testing.T) {
	if ShouldFormatInvocationResponse(nil) {
		t.Fatal("expected nil workstation to be rejected")
	}
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name: PackagedRunWorkstationName,
		Type: factorydefinitions.WorkstationTypeLogical,
	}
	if ShouldFormatInvocationResponse(workstation) {
		t.Fatal("expected logical workstation type to be rejected")
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

func TestResponseContentFromWorkerOutput_PreservesTextWithoutStopToken(t *testing.T) {
	content, err := ResponseContentFromWorkerOutput("  agent response  ", "")
	if err != nil {
		t.Fatalf("ResponseContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "agent response" {
		t.Fatalf("content = %#v, want trimmed response without stop token", content)
	}
}

func TestResponseContentFromWorkerOutput_StripsNewlineStopToken(t *testing.T) {
	content, err := ResponseContentFromWorkerOutput("agent response\nSTOP", "STOP")
	if err != nil {
		t.Fatalf("ResponseContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "agent response" {
		t.Fatalf("content = %#v, want response before newline stop token", content)
	}
}

func TestResponseContentFromWorkerOutput_RejectsEmptyResponse(t *testing.T) {
	if _, err := ResponseContentFromWorkerOutput("   \nSTOP", "STOP"); err == nil {
		t.Fatal("expected empty normalized response to fail")
	}
}
