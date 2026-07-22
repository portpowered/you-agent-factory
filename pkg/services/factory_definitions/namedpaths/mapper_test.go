package namedpaths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapDir_UnscopedName(t *testing.T) {
	root := filepath.Join("home", ".you-agent-factory", "factories")
	got, err := MapDir(root, "alpha")
	if err != nil {
		t.Fatalf("MapDir(alpha): %v", err)
	}
	want := filepath.Join(root, "alpha")
	if got != want {
		t.Fatalf("MapDir(alpha) = %q, want %q", got, want)
	}
}

func TestMapDir_ScopedName(t *testing.T) {
	root := filepath.Join("home", ".you-agent-factory", "factories")
	got, err := MapDir(root, "@you/goal")
	if err != nil {
		t.Fatalf("MapDir(@you/goal): %v", err)
	}
	want := filepath.Join(root, "@you", "goal")
	if got != want {
		t.Fatalf("MapDir(@you/goal) = %q, want %q", got, want)
	}
	if strings.Contains(got, "%2F") {
		t.Fatalf("mapped path %q must not use percent-encoded scoped leaf names", got)
	}
}

func TestPathSegments_ValidCases(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{name: "alpha", want: []string{"alpha"}},
		{name: "@you/goal", want: []string{"@you", "goal"}},
		{name: "@you/tts", want: []string{"@you", "tts"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PathSegments(tt.name)
			if err != nil {
				t.Fatalf("PathSegments: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("segments = %#v, want %#v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("segment[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPathSegments_RejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name       string
		wantSubstr string
	}{
		{name: "", wantSubstr: "factory name is required"},
		{name: "../alpha", wantSubstr: "cannot contain path separators"},
		{name: "alpha/beta", wantSubstr: "cannot contain path separators"},
		{name: `alpha\beta`, wantSubstr: "cannot contain path separators"},
		{name: ".", wantSubstr: "not a valid directory name"},
		{name: "..", wantSubstr: "not a valid directory name"},
		{name: "@you", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/tts/extra", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/../goal", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/./goal", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/..", wantSubstr: "not a valid directory name"},
		{name: "@you/.", wantSubstr: "not a valid directory name"},
		{name: "@you/foo/bar", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/foo\\bar", wantSubstr: "cannot contain path separators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PathSegments(tt.name)
			if err == nil {
				t.Fatalf("PathSegments(%q) expected error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("PathSegments(%q) error = %v, want substring %q", tt.name, err, tt.wantSubstr)
			}
		})
	}
}

func TestPathSegments_RoundTrip(t *testing.T) {
	names := []string{"alpha", "@you/goal", "@you/tts"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			segments, err := PathSegments(name)
			if err != nil {
				t.Fatalf("PathSegments(%q): %v", name, err)
			}
			roundTrip, err := NameFromPathSegments(segments)
			if err != nil {
				t.Fatalf("NameFromPathSegments(%#v): %v", segments, err)
			}
			if roundTrip != name {
				t.Fatalf("round trip = %q, want %q", roundTrip, name)
			}
		})
	}
}

func TestNameFromPathSegments_RejectsInvalidLayouts(t *testing.T) {
	tests := []struct {
		name       string
		segments   []string
		wantSubstr string
	}{
		{name: "empty", segments: nil, wantSubstr: "factory path segments are required"},
		{name: "scope-only", segments: []string{"@you"}, wantSubstr: "not a valid hierarchical layout"},
		{name: "too-many", segments: []string{"@you", "goal", "extra"}, wantSubstr: "not a valid hierarchical layout"},
		{name: "unscoped-pair", segments: []string{"alpha", "beta"}, wantSubstr: "not a valid hierarchical layout"},
		{name: "unsafe-segment", segments: []string{".."}, wantSubstr: "not a valid directory name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NameFromPathSegments(tt.segments)
			if err == nil {
				t.Fatalf("NameFromPathSegments(%#v) expected error", tt.segments)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}

func TestMapDir_RejectsEmptyRoot(t *testing.T) {
	_, err := MapDir("", "alpha")
	if err == nil {
		t.Fatal("expected empty factory root to fail")
	}
	if !strings.Contains(err.Error(), "factory root is required") {
		t.Fatalf("error = %v, want factory root required", err)
	}
}
