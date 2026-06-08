package apicontract_test

import (
	"encoding/json"
	"os"
	"testing"
)

func loadCanonicalFactoryEventVocabularyFixture(t *testing.T) []map[string]any {
	t.Helper()
	fixtureBytes, err := os.ReadFile("../testdata/canonical-event-vocabulary-stream.json")
	if err != nil {
		t.Fatalf("read canonical event vocabulary fixture: %v", err)
	}
	assertJSONStringLiteralMissing(t, string(fixtureBytes), retiredFactoryEventTypeValues...)

	var fixture []map[string]any
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("parse canonical event vocabulary fixture: %v", err)
	}
	if len(fixture) != len(canonicalFactoryEventTypeValues) {
		t.Fatalf("canonical event vocabulary fixture length = %d, want %d", len(fixture), len(canonicalFactoryEventTypeValues))
	}
	return fixture
}
