package agy_test

import (
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
)

func TestParseFinalOutputRejectsEmptyAndInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: nil},
		{name: "invalid utf8", input: []byte{0xff}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			content, failure := agy.ParseFinalOutputForTest(test.input)
			if content != "" || failure == nil ||
				failure.Kind != providers.ExecuteFailureKindUnknown {
				t.Fatalf("parse = (%q, %#v), want empty content and unknown failure", content, failure)
			}
		})
	}
}

func TestParseFinalOutputTrimsAndReturnsAuthoritativeContent(t *testing.T) {
	t.Parallel()

	content, failure := agy.ParseFinalOutputForTest([]byte("  Complete response\n"))
	if failure != nil {
		t.Fatalf("failure = %#v, want nil", failure)
	}
	if content != "Complete response" {
		t.Fatalf("content = %q, want trimmed authoritative response", content)
	}
}

func TestParseFinalOutputBoundsLargeResponses(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("a", 300*1024)
	content, failure := agy.ParseFinalOutputForTest([]byte(large))
	if failure != nil {
		t.Fatalf("failure = %#v, want nil", failure)
	}
	if len(content) > 256*1024 {
		t.Fatalf("content length = %d, want bounded publish size", len(content))
	}
}
