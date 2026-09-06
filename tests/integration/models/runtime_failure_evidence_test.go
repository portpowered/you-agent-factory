package models_test

import (
	"encoding/json"
	"fmt"
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

func TestPinnedTTSRuntimeEvidenceRejectsAmbiguousOrSensitiveDecisions(t *testing.T) {
	t.Parallel()

	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	validFailure := fmt.Sprintf(
		`{"sequence":1,"kind":"STAGE","stage":"BACKEND_START","outcome":"FAILED","failure_class":"PROCESS_EXITED","duration_millis":1,"cause_sha256":"%s"}
{"sequence":2,"kind":"TERMINAL","stage":"BACKEND_START","outcome":"FAILED","failure_class":"PROCESS_EXITED","duration_millis":2,"cause_sha256":"%s"}
`, digest, digest)
	validSuccess := `{"sequence":1,"kind":"STAGE","stage":"INVOKE","outcome":"COMPLETED","duration_millis":1}
{"sequence":2,"kind":"TERMINAL","stage":"INVOKE","outcome":"COMPLETED","duration_millis":2}
`
	for _, test := range []struct {
		name string
		body []byte
		want bool
	}{
		{name: "valid bounded failure", body: []byte(validFailure), want: true},
		{name: "valid semantic success", body: []byte(validSuccess), want: true},
		{name: "empty", body: nil},
		{name: "missing terminal", body: []byte(strings.Split(validFailure, "\n")[0])},
		{name: "duplicate terminal", body: []byte(validFailure + strings.Split(validFailure, "\n")[1] + "\n")},
		{name: "unordered sequence", body: []byte(strings.Replace(validFailure, `"sequence":2`, `"sequence":3`, 1))},
		{name: "invalid digest", body: []byte(strings.Replace(validFailure, digest, "not-a-digest", 1))},
		{name: "raw prompt", body: []byte(strings.Replace(validFailure, "BACKEND_START", pinnedTTSPrompt, 1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodePinnedTTSRuntimeEvidence(test.body)
			if test.want && err != nil {
				t.Fatalf("decodePinnedTTSRuntimeEvidence() error = %v", err)
			}
			if !test.want && err == nil {
				t.Fatal("decodePinnedTTSRuntimeEvidence() accepted invalid evidence")
			}
		})
	}
}
