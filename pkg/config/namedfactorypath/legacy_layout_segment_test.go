package namedfactorypath

import (
	"strings"
	"testing"
)

func TestLegacyLayoutSegment_UnscopedName(t *testing.T) {
	got, err := LegacyLayoutSegment("alpha")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(alpha): %v", err)
	}
	if got != "alpha" {
		t.Fatalf("LegacyLayoutSegment(alpha) = %q, want alpha", got)
	}
}

func TestLegacyLayoutSegment_ScopedName(t *testing.T) {
	got, err := LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	want := "@you%2Fgoal"
	if got != want {
		t.Fatalf("LegacyLayoutSegment(@you/goal) = %q, want %q", got, want)
	}
}

func TestLegacyLayoutSegment_RejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name       string
		wantSubstr string
	}{
		{name: "", wantSubstr: "factory name is required"},
		{name: "../alpha", wantSubstr: "cannot contain path separators"},
		{name: "@you", wantSubstr: "must be scoped as @scope/name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LegacyLayoutSegment(tt.name)
			if err == nil {
				t.Fatalf("LegacyLayoutSegment(%q) expected error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}

func TestLegacyLayoutSegmentToName_RoundTrip(t *testing.T) {
	names := []string{"alpha", "@you/goal", "@you/tts"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			segment, err := LegacyLayoutSegment(name)
			if err != nil {
				t.Fatalf("LegacyLayoutSegment(%q): %v", name, err)
			}
			got, err := LegacyLayoutSegmentToName(segment)
			if err != nil {
				t.Fatalf("LegacyLayoutSegmentToName(%q): %v", segment, err)
			}
			if got != name {
				t.Fatalf("round trip = %q, want %q", got, name)
			}
		})
	}
}

func TestLegacyLayoutSegmentToName_RejectsInvalidSegment(t *testing.T) {
	tests := []struct {
		name       string
		segment    string
		wantSubstr string
	}{
		{name: "empty", segment: "", wantSubstr: "factory name is required"},
		{name: "traversal", segment: "..", wantSubstr: "not a valid directory name"},
		{name: "non-canonical-encoded", segment: "@you%2Fgoal%2Fextra", wantSubstr: "must be scoped as @scope/name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LegacyLayoutSegmentToName(tt.segment)
			if err == nil {
				t.Fatalf("LegacyLayoutSegmentToName(%q) expected error", tt.segment)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}
