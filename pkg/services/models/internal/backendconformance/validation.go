package backendconformance

import (
	"fmt"
	"strings"
)

// Classify assigns one supplying class to reference. It is pure and does not
// validate the manifest details belonging to a pinned artifact.
func Classify(reference Reference, inputs Inputs) (Resolution, error) {
	registered := countRegistered(reference.Identifier, inputs.RegisteredBackends)
	releaseBuilt := countReleaseBuilt(reference.Identifier, inputs.ReleaseBuiltCommands)

	switch {
	case registered == 1 && releaseBuilt == 0:
		return Resolution{Reference: reference, Kind: ResolutionPinnedArtifact}, nil
	case registered == 0 && releaseBuilt == 1:
		return Resolution{Reference: reference, Kind: ResolutionReleaseBuilt}, nil
	case registered == 0 && releaseBuilt == 0:
		return Resolution{Reference: reference, Kind: ResolutionUnresolved}, validationFailure(reference, ResolutionUnresolved,
			"no registered pinned backend or concrete release-built command supplies the identifier")
	default:
		return Resolution{Reference: reference, Kind: ResolutionAmbiguous}, validationFailure(reference, ResolutionAmbiguous,
			fmt.Sprintf("matches %d registered pinned backend records and %d release-built command records", registered, releaseBuilt))
	}
}

// Validate checks every customer-reachable reference and every manifest fact
// needed to prove a pinned backend is installable on the closed target matrix.
func Validate(inputs Inputs) error {
	failures := make([]Failure, 0)
	for _, reference := range inputs.References {
		if strings.TrimSpace(reference.Identifier) == "" {
			failures = append(failures, Failure{
				Reference: reference, Classification: ResolutionUnresolved,
				Detail: "the customer-reachable identifier is empty",
			})
			continue
		}

		resolution, err := Classify(reference, inputs)
		if err != nil {
			failures = append(failures, failureFromError(reference, err))
			continue
		}
		if resolution.Kind == ResolutionPinnedArtifact {
			failures = append(failures, validatePinned(reference, inputs.PinnedArtifacts)...)
		} else {
			failures = append(failures, validateReleaseBuilt(reference, inputs.ReleaseBuiltCommands)...)
		}
	}

	if len(failures) == 0 {
		return nil
	}
	sortFailures(failures)
	return &ValidationError{Failures: failures}
}

func countRegistered(identifier string, registered []string) int {
	count := 0
	for _, candidate := range registered {
		if candidate == identifier {
			count++
		}
	}
	return count
}

func countReleaseBuilt(identifier string, commands []ReleaseBuiltCommand) int {
	count := 0
	for _, command := range commands {
		if command.Command == identifier {
			count++
		}
	}
	return count
}

func validatePinned(reference Reference, artifacts []PinnedArtifact) []Failure {
	matching := make([]PinnedArtifact, 0)
	for _, artifact := range artifacts {
		if artifact.BackendID == reference.Identifier {
			matching = append(matching, artifact)
		}
	}
	if len(matching) == 0 {
		return []Failure{pinnedFailure(reference, "manifest has no artifact entry for the registered backend")}
	}

	byTarget := make(map[string][]PinnedArtifact, len(matching))
	failures := make([]Failure, 0)
	for _, artifact := range matching {
		if !isRequiredTarget(artifact.TargetID) {
			failures = append(failures, pinnedFailure(reference,
				fmt.Sprintf("manifest target %q is outside the closed darwin-arm64, linux-amd64, windows-amd64 matrix", artifact.TargetID)))
			continue
		}
		byTarget[artifact.TargetID] = append(byTarget[artifact.TargetID], artifact)
	}

	for _, target := range requiredTargets() {
		entries := byTarget[target]
		switch len(entries) {
		case 0:
			failures = append(failures, pinnedFailure(reference,
				fmt.Sprintf("manifest is missing the required target %q", target)))
		case 1:
			if entries[0].SizeBytes <= MinimumPinnedArtifactSizeBytes {
				failures = append(failures, pinnedFailure(reference,
					fmt.Sprintf("target %q sizeBytes %d must be strictly greater than %d bytes (1 MiB)", target, entries[0].SizeBytes, MinimumPinnedArtifactSizeBytes)))
			}
		default:
			failures = append(failures, pinnedFailure(reference,
				fmt.Sprintf("manifest has %d entries for required target %q; exactly one is required", len(entries), target)))
		}
	}
	return failures
}

func validateReleaseBuilt(reference Reference, commands []ReleaseBuiltCommand) []Failure {
	for _, command := range commands {
		if command.Command == reference.Identifier && strings.TrimSpace(command.Evidence) != "" {
			return nil
		}
	}
	return []Failure{{
		Reference: reference, Classification: ResolutionReleaseBuilt,
		Detail: "release-built classification has no concrete Makefile or production release-target evidence",
	}}
}

func isRequiredTarget(target string) bool {
	for _, required := range requiredTargets() {
		if target == required {
			return true
		}
	}
	return false
}

func pinnedFailure(reference Reference, detail string) Failure {
	return Failure{Reference: reference, Classification: ResolutionPinnedArtifact, Detail: detail}
}

func validationFailure(reference Reference, classification ResolutionKind, detail string) error {
	return &ValidationError{Failures: []Failure{{
		Reference: reference, Classification: classification, Detail: detail,
	}}}
}

func failureFromError(reference Reference, err error) Failure {
	validation, ok := err.(*ValidationError)
	if !ok || len(validation.Failures) == 0 {
		return Failure{Reference: reference, Classification: ResolutionUnresolved, Detail: err.Error()}
	}
	failure := validation.Failures[0]
	failure.Reference = reference
	return failure
}

func requiredTargets() []string {
	return []string{TargetDarwinArm64, TargetLinuxAmd64, TargetWindowsAmd64}
}
