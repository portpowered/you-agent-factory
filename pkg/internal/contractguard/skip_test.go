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

func TestShouldSkipDir_AppliesSharedPolicyFromWalkPaths(t *testing.T) {
	t.Parallel()

	root := `C:\repo`
	if !ShouldSkipDir(root, `C:\repo\.claude`) {
		t.Fatal(`expected hidden metadata directory ".claude" to be skipped`)
	}
	if !ShouldSkipDir(root, `C:\repo\pkg\api\generated`, "pkg/api/generated") {
		t.Fatal(`expected explicit generated directory exception to be skipped`)
	}
	if ShouldSkipDir(root, `C:\repo\pkg\api`) {
		t.Fatal(`expected visible package directory to remain scannable`)
	}
}
