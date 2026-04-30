package contractguard

import (
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	t.Parallel()

	root := filepath.Clean("pkg")
	tests := []struct {
		name   string
		path   string
		skips  []string
		expect bool
	}{
		{
			name:   "root stays visible",
			path:   root,
			skips:  []string{"api/generated"},
			expect: false,
		},
		{
			name:   "hidden root child is skipped",
			path:   filepath.Join(root, ".claude"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "nested hidden worktree metadata is skipped",
			path:   filepath.Join(root, "interfaces", ".worktrees"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "generated subtree is skipped",
			path:   filepath.Join(root, "api", "generated"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "normal package directory stays visible",
			path:   filepath.Join(root, "config"),
			skips:  []string{"api/generated"},
			expect: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := ShouldSkipDir(root, test.path, test.skips...); got != test.expect {
				t.Fatalf("ShouldSkipDir(%q, %q) = %t, want %t", root, test.path, got, test.expect)
			}
		})
	}
}
