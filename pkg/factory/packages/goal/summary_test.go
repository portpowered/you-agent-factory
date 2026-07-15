package goal

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestShouldFormatInvocationSummary_MatchesPackagedInvokeWorkstation(t *testing.T) {
	cases := []struct {
		name        string
		workstation *interfaces.FactoryWorkstationConfig
		want        bool
	}{
		{
			name: "packaged goal execute workstation",
			workstation: &interfaces.FactoryWorkstationConfig{
				Name: PackagedExecuteWorkstationName,
				Type: interfaces.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name: "other workstation",
			workstation: &interfaces.FactoryWorkstationConfig{
				Name: "other-workstation",
				Type: interfaces.WorkstationTypeModel,
			},
		},
		{
			name: "logical move",
			workstation: &interfaces.FactoryWorkstationConfig{
				Name: PackagedInvokeWorkstationName,
				Type: interfaces.WorkstationTypeLogical,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldFormatInvocationSummary(tc.workstation); got != tc.want {
				t.Fatalf("ShouldFormatInvocationSummary() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSummaryContentFromWorkerOutput_StripsStopTokenAndReturnsTextSummary(t *testing.T) {
	got, err := SummaryContentFromWorkerOutput("Final goal summary.\n<COMPLETE>", "<COMPLETE>")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("summary content = %#v, want one text part", got)
	}
	if got[0].Text != "Final goal summary." {
		t.Fatalf("summary text = %q, want trimmed worker summary", got[0].Text)
	}
}

func TestSummaryContentFromWorkerOutput_RejectsEmptySummary(t *testing.T) {
	if _, err := SummaryContentFromWorkerOutput("   \n<COMPLETE>", "<COMPLETE>"); err == nil {
		t.Fatal("expected empty normalized summary to fail")
	}
}
