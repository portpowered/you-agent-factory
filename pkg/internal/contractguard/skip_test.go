package contractguard

import "testing"

func TestShouldSkipRelativeDir_SkipsHiddenMetadataDirectories(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		".claude",
		"pkg/.cache",
		"ui/.storybook",
	} {
		if !ShouldSkipRelativeDir(rel) {
			t.Fatalf("expected %q to be skipped", rel)
		}
	}
}

func TestShouldSkipRelativeDir_KeepsVisibleDirectoriesUnlessExplicitlySkipped(t *testing.T) {
	t.Parallel()

	if ShouldSkipRelativeDir("pkg/api") {
		t.Fatal("pkg/api should stay visible without an explicit skip")
	}
	if !ShouldSkipRelativeDir("pkg/api/generated", "pkg/api/generated") {
		t.Fatal("pkg/api/generated should be skipped when the caller opts in")
	}
}
