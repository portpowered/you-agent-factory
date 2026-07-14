package apitypes

import (
	"encoding/json"
	"testing"
)

func TestInt64String_MarshalJSONUsesDecimalString(t *testing.T) {
	payload, err := json.Marshal(Int64String(1779941481569583433))
	if err != nil {
		t.Fatalf("Marshal(Int64String): %v", err)
	}
	if string(payload) != `"1779941481569583433"` {
		t.Fatalf("Marshal(Int64String) = %s, want quoted decimal string", payload)
	}
}

func TestInt64String_UnmarshalJSONAcceptsStringAndNumber(t *testing.T) {
	for _, input := range []string{`"1779941481569583433"`, `1779941481569583433`} {
		var got Int64String
		if err := json.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", input, err)
		}
		if got.Int64() != 1779941481569583433 {
			t.Fatalf("Unmarshal(%s) = %d, want 1779941481569583433", input, got.Int64())
		}
	}
}
