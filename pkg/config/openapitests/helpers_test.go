package openapitests

import (
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
)

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

func assertFindingMatch(t *testing.T, findings []Finding, rule string, pathSubstring string, messageSubstring string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule != rule || finding.Severity != SeverityError {
			continue
		}
		if !strings.Contains(finding.Path, pathSubstring) {
			t.Fatalf("finding path = %q, want substring %q", finding.Path, pathSubstring)
		}
		if !strings.Contains(finding.Message, messageSubstring) {
			t.Fatalf("finding message = %q, want substring %q", finding.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}
