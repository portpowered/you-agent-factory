package wire

import "testing"

func TestAppendManagedBackendEnvironmentPreservesExplicitValuesAndReplacesKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"PATH=C:\\runtime",
		"VIBEVOICECPP_LIBRARY=C:\\stale\\library.dll",
		"MODEL=tts",
	}
	got := appendManagedBackendEnvironment(base, []string{
		"vibevoicecpp_library=C:\\managed\\library.dll",
		"MODEL_ROOT=C:\\models",
	})
	if len(got) != len(base)+1 {
		t.Fatalf("merged environment length = %d, want %d: %#v", len(got), len(base)+1, got)
	}
	if got[0] != base[0] || got[2] != base[2] {
		t.Fatalf("merged environment changed unrelated values: %#v", got)
	}
	if got[1] != "vibevoicecpp_library=C:\\managed\\library.dll" {
		t.Fatalf("merged environment did not replace case-insensitive library key: %#v", got)
	}
	if got[3] != "MODEL_ROOT=C:\\models" {
		t.Fatalf("merged environment omitted new value: %#v", got)
	}
}
