package models_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPinnedTTSEvidenceSerializesBoundedStageFailureWithoutRawCause(t *testing.T) {
	t.Parallel()

	evidence := pinnedTTSEvidence{
		Outcome: "FAIL",
		StageTrace: []pinnedTTSStageEvidence{
			{Stage: "PROTOCOL_LOAD", Outcome: "FAILED", FailureClass: "PROTOCOL_INCOMPATIBLE", DurationMillis: 123},
		},
		Failure: &pinnedTTSFailureEvidence{
			Stage:       "PROTOCOL_LOAD",
			Class:       "PROTOCOL_INCOMPATIBLE",
			CauseSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		Blocker: "bounded failure",
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	serialized := string(body)
	rawCause := "token=sensitive-marker"
	if !strings.Contains(serialized, `"stageTrace"`) || !strings.Contains(serialized, `"failure"`) {
		t.Fatalf("evidence = %s, want stage and failure projections", serialized)
	}
	if strings.Contains(serialized, rawCause) {
		t.Fatalf("evidence copied prohibited raw cause: %s", serialized)
	}
}
