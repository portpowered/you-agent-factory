package service

import (
	"strings"
	"testing"
)

func TestNewRejectsRuntimeProjectionBindingDrift(t *testing.T) {
	const base = `{
  "acp": [{
    "name": "cursor-acp",
    "transport": "stdio",
    "command": "cursor-agent acp",
    "arguments": ["acp"],
    "posture": "installed_executable",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }]
}`

	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{
			name: "unknown profile",
			mutate: func(document string) string {
				return strings.Replace(document, `"profile": "cursor-acp"`, `"profile": "missing-profile"`, 1)
			},
			want: "unknown runtime profile",
		},
		{
			name: "wrong implementation kind",
			mutate: func(document string) string {
				return strings.Replace(document, `"kind": "acp_agent"`, `"kind": "native_cli"`, 1)
			},
			want: "unsupported implementation kind",
		},
		{
			name: "catalog-only posture",
			mutate: func(document string) string {
				return strings.Replace(document, `"posture": "installed_executable"`, `"posture": "catalog_only"`, 1)
			},
			want: "invalid launch posture",
		},
		{
			name: "argument drift",
			mutate: func(document string) string {
				return strings.Replace(document, `"arguments": ["acp"]`, `"arguments": ["wrong"]`, 1)
			},
			want: "command arguments drift",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New([]byte(test.mutate(base)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
