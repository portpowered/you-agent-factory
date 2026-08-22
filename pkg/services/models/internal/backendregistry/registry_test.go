package backendregistry

import "testing"

func TestRecordsKeepArtifactSupportSeparateFromManagedRuntimeAliases(t *testing.T) {
	t.Parallel()

	records := Records()
	if len(records) != 3 {
		t.Fatalf("Records length = %d, want three artifact records", len(records))
	}

	want := map[string]struct {
		repository string
		path       string
		aliases    []string
	}{
		"localai-llamacpp": {
			repository: "https://github.com/ggerganov/llama.cpp",
			path:       "backend/cpp/llama-cpp",
			aliases:    []string{"LLAMACPP"},
		},
		"localai-whisper": {
			repository: "https://github.com/ggml-org/whisper.cpp",
			path:       "backend/go/whisper",
		},
		"localai-vibevoice": {
			repository: "https://github.com/mudler/vibevoice.cpp",
			path:       "backend/go/vibevoice-cpp",
		},
	}
	for _, record := range records {
		expected, ok := want[record.Artifact.ID]
		if !ok {
			t.Fatalf("unexpected artifact record %q", record.Artifact.ID)
		}
		if record.Artifact.SourceRepository != expected.repository || record.Artifact.SourcePath != expected.path {
			t.Fatalf("artifact facts for %q = %#v, want repository/path %q/%q", record.Artifact.ID, record.Artifact, expected.repository, expected.path)
		}
		if !sameStrings(record.ManagedRuntimeAliases, expected.aliases) {
			t.Fatalf("managed aliases for %q = %#v, want %#v", record.Artifact.ID, record.ManagedRuntimeAliases, expected.aliases)
		}
		if IsManagedRuntimeBackend(record.Artifact.ID) {
			t.Fatalf("artifact identifier %q must not be accepted as managed-runtime alias", record.Artifact.ID)
		}
	}
	if !IsManagedRuntimeBackend("  llamaCpp \t") {
		t.Fatal("normalized LLAMACPP alias = false, want true")
	}
}

func TestRecordsReturnDetachedViews(t *testing.T) {
	t.Parallel()

	first := Records()
	first[0].ManagedRuntimeAliases[0] = "MUTATED"
	second := Records()
	if second[0].ManagedRuntimeAliases[0] != "LLAMACPP" {
		t.Fatalf("registry alias after detached-view mutation = %q, want LLAMACPP", second[0].ManagedRuntimeAliases[0])
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
