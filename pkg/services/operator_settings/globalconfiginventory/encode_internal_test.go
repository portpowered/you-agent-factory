package globalconfiginventory

import (
	"strings"
	"testing"
)

func TestMarshalCanonicalJSONReportsUnsupportedValues(t *testing.T) {
	t.Parallel()

	_, err := marshalCanonicalJSON(make(chan int))
	if err == nil {
		t.Fatal("marshalCanonicalJSON() error = nil, want unsupported value error")
	}
	for _, fragment := range []string{"marshal global config topology inventory", "unsupported type"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("marshalCanonicalJSON() error = %q, want fragment %q", err, fragment)
		}
	}
}
