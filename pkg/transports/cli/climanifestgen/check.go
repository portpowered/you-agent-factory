package climanifestgen

import "github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"

// Drift describes byte-level differences between generated artifacts and the
// current generator output.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
	// CommandIDs maps a generated artifact path to the stable IDs affected by drift.
	CommandIDs map[string][]string
}

// Empty reports whether generated artifacts match the generator output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// AnnotateDrift adds CLI command identity context to policy-free artifact
// drift computed by the command-selected Platform store.
func AnnotateDrift(base generatedartifacts.Drift) Drift {
	drift := Drift{
		Stale: append([]string(nil), base.Stale...), Missing: append([]string(nil), base.Missing...),
		Unexpected: append([]string(nil), base.Unexpected...), CommandIDs: map[string][]string{},
	}
	artifactIDs := map[string][]string{
		RunSubmitFamilyJSONPath:       RunSubmitFamilyCommandIDs,
		RunSubmitFamilyCommandIDsPath: RunSubmitFamilyCommandIDs,
		MCPFamilyJSONPath:             MCPFamilyCommandIDs,
		MCPFamilyCommandIDsPath:       MCPFamilyCommandIDs,
	}
	for _, paths := range [][]string{drift.Missing, drift.Stale} {
		for _, path := range paths {
			if ids := artifactIDs[path]; len(ids) > 0 {
				drift.CommandIDs[path] = append([]string(nil), ids...)
			}
		}
	}
	return drift
}
