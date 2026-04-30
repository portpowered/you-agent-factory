package contractguard

import (
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		root   string
		path   string
		skips  []string
		expect bool
	}{
		{
			name:   "root stays visible",
			root:   filepath.Clean("pkg"),
			path:   filepath.Clean("pkg"),
			skips:  []string{"api/generated"},
			expect: false,
		},
		{
			name:   "hidden root child is skipped",
			root:   filepath.Clean("pkg"),
			path:   filepath.Join("pkg", ".claude"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "nested hidden worktree metadata is skipped",
			root:   filepath.Clean("pkg"),
			path:   filepath.Join("pkg", "interfaces", ".worktrees"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "generated subtree is skipped",
			root:   filepath.Clean("pkg"),
			path:   filepath.Join("pkg", "api", "generated"),
			skips:  []string{"api/generated"},
			expect: true,
		},
		{
			name:   "normal package directory stays visible",
			root:   filepath.Clean("pkg"),
			path:   filepath.Join("pkg", "config"),
			skips:  []string{"api/generated"},
			expect: false,
		},
		{
			name:   "module root hidden metadata is skipped",
			root:   filepath.Clean("repo"),
			path:   filepath.Join("repo", ".git"),
			skips:  []string{"pkg/api/generated", "ui/dist", "ui/node_modules", "ui/storybook-static"},
			expect: true,
		},
		{
			name:   "module root ui build output is skipped",
			root:   filepath.Clean("repo"),
			path:   filepath.Join("repo", "ui", "dist"),
			skips:  []string{"pkg/api/generated", "ui/dist", "ui/node_modules", "ui/storybook-static"},
			expect: true,
		},
		{
			name:   "module root handwritten source stays visible",
			root:   filepath.Clean("repo"),
			path:   filepath.Join("repo", "pkg", "petri"),
			skips:  []string{"pkg/api/generated", "ui/dist", "ui/node_modules", "ui/storybook-static"},
			expect: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := ShouldSkipDir(test.root, test.path, test.skips...); got != test.expect {
				t.Fatalf("ShouldSkipDir(%q, %q) = %t, want %t", test.root, test.path, got, test.expect)
			}
		})
	}
}
