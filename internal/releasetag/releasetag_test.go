package releasetag

import (
	"reflect"
	"testing"
)

func TestRequireSemver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     string
		want    string
		wantErr bool
	}{
		{name: "accepts semver", tag: "v1.2.3", want: "v1.2.3"},
		{name: "trims whitespace", tag: " v4.5.6\n", want: "v4.5.6"},
		{name: "rejects empty", tag: "", wantErr: true},
		{name: "rejects bare prefix", tag: "vnext", wantErr: true},
		{name: "rejects prerelease", tag: "v1.2.3-rc1", wantErr: true},
		{name: "rejects missing v", tag: "1.2.3", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := RequireSemver(tt.tag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RequireSemver(%q) error = nil, want non-nil", tt.tag)
				}
				return
			}
			if err != nil {
				t.Fatalf("RequireSemver(%q) error = %v", tt.tag, err)
			}
			if got != tt.want {
				t.Fatalf("RequireSemver(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestFilterSemver(t *testing.T) {
	t.Parallel()

	got := FilterSemver([]string{"v1.2.3", "vnext", " v2.0.0 ", "1.0.0", "v3.4.5-rc1"})
	want := []string{"v1.2.3", "v2.0.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterSemver() = %#v, want %#v", got, want)
	}
}
