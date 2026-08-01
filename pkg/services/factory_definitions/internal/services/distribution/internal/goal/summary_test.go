package goal

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestShouldFormatInvocationSummary_MatchesPackagedInvokeWorkstation(t *testing.T) {
	cases := []struct {
		name        string
		workstation *factorydefinitions.FactoryWorkstationConfig
		want        bool
	}{
		{
			name: "packaged goal execute workstation",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: PackagedExecuteWorkstationName,
				Type: factorydefinitions.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name: "other workstation",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: "other-workstation",
				Type: factorydefinitions.WorkstationTypeModel,
			},
		},
		{
			name: "logical move",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: PackagedInvokeWorkstationName,
				Type: factorydefinitions.WorkstationTypeLogical,
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
