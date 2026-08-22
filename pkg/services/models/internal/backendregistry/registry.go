// Package backendregistry owns the immutable backend facts shared by Models
// artifact handling and managed-runtime admission.
package backendregistry

import "strings"

// ArtifactFacts describes one backend that has a published LocalAI artifact.
// Artifact support is intentionally separate from managed-runtime aliases:
// an artifact may exist without being enabled for managed-runtime
// configuration.
type ArtifactFacts struct {
	ID               string
	SourceRepository string
	SourcePath       string
}

// Record is one canonical Models backend record. ManagedRuntimeAliases are
// the normalized configuration values currently enabled for supervision.
type Record struct {
	Artifact              ArtifactFacts
	ManagedRuntimeAliases []string
}

// Records returns detached canonical backend records in stable order. Each
// call creates fresh values so callers cannot mutate registry state.
func Records() []Record {
	return []Record{
		{
			Artifact: ArtifactFacts{
				ID:               "localai-llamacpp",
				SourceRepository: "https://github.com/ggerganov/llama.cpp",
				SourcePath:       "backend/cpp/llama-cpp",
			},
			ManagedRuntimeAliases: []string{"LLAMACPP"},
		},
		{
			Artifact: ArtifactFacts{
				ID:               "localai-whisper",
				SourceRepository: "https://github.com/ggml-org/whisper.cpp",
				SourcePath:       "backend/go/whisper",
			},
		},
		{
			Artifact: ArtifactFacts{
				ID:               "localai-vibevoice",
				SourceRepository: "https://github.com/mudler/vibevoice.cpp",
				SourcePath:       "backend/go/vibevoice-cpp",
			},
		},
	}
}

// LookupArtifact returns the canonical facts for one published artifact
// identifier. Artifact identifiers retain their exact manifest spelling.
func LookupArtifact(id string) (Record, bool) {
	for _, record := range Records() {
		if record.Artifact.ID == id {
			return record, true
		}
	}
	return Record{}, false
}

// IsManagedRuntimeBackend reports whether value is one of the currently
// enabled managed-runtime aliases. It performs only the established trim and
// uppercase normalization; it does not inspect configuration or perform IO.
func IsManagedRuntimeBackend(value string) bool {
	canonical := strings.ToUpper(strings.TrimSpace(value))
	for _, record := range Records() {
		for _, alias := range record.ManagedRuntimeAliases {
			if canonical == alias {
				return true
			}
		}
	}
	return false
}
