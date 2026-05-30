package submit

import (
	"strings"
	"testing"
)

func TestResolveBatchFileInput_RejectsUnsupportedModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  BatchConfig
		want string
	}{
		{
			name: "no args",
			cfg:  BatchConfig{},
			want: "batch input required",
		},
		{
			name: "stdin dash",
			cfg:  BatchConfig{Args: []string{"-"}},
			want: "stdin input is not yet supported",
		},
		{
			name: "inline json",
			cfg:  BatchConfig{Args: []string{`{"requestId":"x"}`}},
			want: "inline JSON is not yet supported",
		},
		{
			name: "file flag",
			cfg:  BatchConfig{FileFlag: "./batch.json"},
			want: "--file is not yet supported",
		},
		{
			name: "too many args",
			cfg:  BatchConfig{Args: []string{"a.json", "b.json"}},
			want: "at most one positional",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolveBatchFileInput(tc.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
