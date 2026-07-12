package commandidentity

import (
	"strings"
	"testing"
)

func TestEnsureUniqueCommandPaths_RejectsDuplicatePath(t *testing.T) {
	err := ensureUniqueCommandPaths([]CommandRecord{
		{Path: "synth nested"},
		{Path: "synth aliased"},
		{Path: "synth nested"},
	})
	if err == nil {
		t.Fatal("ensureUniqueCommandPaths() error = nil, want duplicate path failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate command path "synth nested"`) {
		t.Fatalf("error = %q, want duplicate path diagnostic naming synth nested", got)
	}
}
