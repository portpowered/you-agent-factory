package factoryconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeAuthoredFactoryAPIDecodesSuppliedBytes(t *testing.T) {
	factory, err := DecodeAuthoredFactoryAPI([]byte(`{"name":"authored"}`))
	if err != nil {
		t.Fatalf("DecodeAuthoredFactoryAPI() error = %v", err)
	}
	if factory.Name != "authored" {
		t.Fatalf("Name = %q, want authored", factory.Name)
	}
}

func TestDecodeAuthoredFactoryAPIReportsMalformedRepresentation(t *testing.T) {
	_, err := DecodeAuthoredFactoryAPI([]byte(`{"name":`))
	if err == nil || !strings.Contains(err.Error(), "parse factory config") {
		t.Fatalf("DecodeAuthoredFactoryAPI() error = %v", err)
	}
}

func TestFactoryConfigMapperAcceptsFutureFieldsAndReportsSortedPaths(t *testing.T) {
	payload := map[string]any{
		"name": "future",
		"logicalRoundTrip": map[string]any{
			"mode":   "v2",
			"secret": "must-not-leak",
		},
		"workers": []any{
			map[string]any{
				"name":         "worker",
				"futurePolicy": map[string]any{"mode": "v2"},
			},
		},
		"workstations": []any{
			map[string]any{
				"name":         "process",
				"limits":       map[string]any{"futureLimit": 2},
				"futurePolicy": map[string]any{"mode": "v2"},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal future Factory payload: %v", err)
	}

	cfg, diagnostics, err := NewFactoryConfigMapper().ExpandWithDiagnostics(encoded)
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
		payload []byte
	}{
		{name: "known field type", payload: []byte("{\"name\":17}")},
		{name: "trailing value", payload: []byte("{\"name\":\"future\"}{}")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewFactoryConfigMapper().Expand(test.payload); err == nil {
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
