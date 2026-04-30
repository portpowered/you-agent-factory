package contractguard

import "testing"

func TestShouldSkipDir(t *testing.T) {
	t.Parallel()

	moduleRoot := "/repo"
	allowedSkipPaths := []string{
		"pkg/api/generated",
		"ui/dist",
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "module root stays visible",
			path: "/repo",
			want: false,
		},
		{
			name: "explicit generated dir stays caller owned",
			path: "/repo/pkg/api/generated",
			want: true,
		},
		{
			name: "hidden metadata dir is skipped",
			path: "/repo/.claude/worktrees/story",
			want: true,
		},
		{
			name: "nested worktree metadata dir is skipped",
			path: "/repo/pkg/api/.worktrees/story",
			want: true,
		},
		{
			name: "git metadata dir is skipped",
			path: "/repo/.git/objects",
			want: true,
		},
		{
			name: "normal source dir is scanned",
			path: "/repo/pkg/api",
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldSkipDir(moduleRoot, tt.path, allowedSkipPaths...)
			if got != tt.want {
				t.Fatalf("ShouldSkipDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
