package factoryconfig

import (
	"strings"
	"testing"
)

func TestFactoryConfigMapperAcceptsFutureFieldsAndReportsSortedPaths(t *testing.T) {
	payload := []byte(`{
		"name": "future",
		"logicalRoundTrip": {"mode": "v2", "secret": "must-not-leak"},
		"workers": [{
			"name": "worker",
			"futurePolicy": {"mode": "v2"}
		}],
		"workstations": [{
			"name": "process",
			"limits": {"futureLimit": 2},
			"futurePolicy": {"mode": "v2"}
		}]
	}`)

	cfg, diagnostics, err := NewFactoryConfigMapper().ExpandWithDiagnostics(payload)
	if err != nil {
		t.Fatalf("ExpandWithDiagnostics() error = %v", err)
	}
	if cfg == nil || cfg.Name != "future" {
		t.Fatalf("decoded config = %#v, want future config", cfg)
	}
	want := []string{
		"$.logicalRoundTrip",
		"$.workers[0].futurePolicy",
		"$.workstations[0].futurePolicy",
		"$.workstations[0].limits.futureLimit",
	}
	if got := diagnostics.Paths(); !equalStrings(got, want) {
		t.Fatalf("diagnostics paths = %#v, want %#v", got, want)
	}
	if got := cfg.IgnoredJSONPaths(); !equalStrings(got, want) {
		t.Fatalf("config paths = %#v, want %#v", got, want)
	}
	if strings.Contains(strings.Join(diagnostics.Paths(), "\n"), "secret") {
		t.Fatal("diagnostics retained an ignored field value")
	}
}

func TestFactoryConfigMapperPreservesKnownValidationAndTrailingRejection(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "known field type", payload: `{"name":17}`},
		{name: "trailing value", payload: `{"name":"future"}{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewFactoryConfigMapper().Expand([]byte(test.payload)); err == nil {
				t.Fatalf("Expand(%s) error = nil", test.payload)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
