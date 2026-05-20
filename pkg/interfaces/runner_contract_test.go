package interfaces

import "testing"

func TestV1RunnerBaselineCapabilities_AreExplicitAndLimited(t *testing.T) {
	t.Parallel()

	got := V1RunnerBaselineCapabilities()
	want := []RunnerBaselineCapability{
		RunnerBaselineCapabilityPromptSubmission,
		RunnerBaselineCapabilityToolExecution,
	}

	if len(got) != len(want) {
		t.Fatalf("baseline capability count = %d, want %d", len(got), len(want))
	}
	for index, capability := range want {
		if got[index] != capability {
			t.Fatalf("baseline[%d] = %q, want %q", index, got[index], capability)
		}
	}

	got[0] = RunnerBaselineCapabilityToolExecution
	if fresh := V1RunnerBaselineCapabilities(); fresh[0] != RunnerBaselineCapabilityPromptSubmission {
		t.Fatalf("baseline capabilities were not detached: %#v", fresh)
	}
}

func TestNewRunnerCapabilities_ClonesOptionalSupport(t *testing.T) {
	t.Parallel()

	original := []RunnerOptionalCapabilitySupport{
		{
			Capability: RunnerOptionalCapabilityStructuredOutput,
			Status:     RunnerOptionalCapabilityStatusUnsupported,
			Detail:     "schema-guided output is not available",
		},
	}

	capabilities := NewRunnerCapabilities(original...)
	if len(capabilities.Baseline) != 2 {
		t.Fatalf("baseline = %#v, want two explicit baseline capabilities", capabilities.Baseline)
	}
	if len(capabilities.Optional) != 1 {
		t.Fatalf("optional = %#v, want one entry", capabilities.Optional)
	}

	original[0].Status = RunnerOptionalCapabilityStatusSupported
	if capabilities.Optional[0].Status != RunnerOptionalCapabilityStatusUnsupported {
		t.Fatalf("optional capability status mutated through source slice: %#v", capabilities.Optional)
	}
}
