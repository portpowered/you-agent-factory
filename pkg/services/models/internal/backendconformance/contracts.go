// Package backendconformance owns the pure release invariant that connects
// customer-reachable inference backends to an installable Models backend.
package backendconformance

import (
	"fmt"
	"sort"
	"strings"
)

// MinimumPinnedArtifactSizeBytes is the lower bound for a published backend
// archive. Placeholder-sized manifest values must not satisfy conformance.
const MinimumPinnedArtifactSizeBytes int64 = 1 << 20

const (
	TargetDarwinArm64  = "darwin-arm64"
	TargetLinuxAmd64   = "linux-amd64"
	TargetWindowsAmd64 = "windows-amd64"
)

// Reference identifies one backend name exposed to a customer.
type Reference struct {
	Identifier string
	Source     string
}

// PinnedArtifact contains the manifest facts needed by the offline guard.
type PinnedArtifact struct {
	BackendID string
	TargetID  string
	SizeBytes int64
}

// ReleaseBuiltCommand records concrete production release-build evidence for
// a command that is not installed from the pinned backend artifact matrix.
type ReleaseBuiltCommand struct {
	Command  string
	Evidence string
}

// Inputs is the detached, explicit input set for Validate. The validator does
// not discover files, inspect the environment, or perform network IO.
type Inputs struct {
	References           []Reference
	RegisteredBackends   []string
	PinnedArtifacts      []PinnedArtifact
	ReleaseBuiltCommands []ReleaseBuiltCommand
}

// ResolutionKind names the one supplying class assigned to a reference.
type ResolutionKind string

const (
	ResolutionPinnedArtifact ResolutionKind = "pinned artifact"
	ResolutionReleaseBuilt   ResolutionKind = "release-built command"
	ResolutionAmbiguous      ResolutionKind = "ambiguous"
	ResolutionUnresolved     ResolutionKind = "unresolved"
)

// Resolution is the deterministic class assigned by Classify.
type Resolution struct {
	Reference Reference
	Kind      ResolutionKind
}

// Failure is one actionable conformance diagnostic.
type Failure struct {
	Reference      Reference
	Classification ResolutionKind
	Detail         string
}

// ValidationError aggregates every failure in stable source/identifier order.
type ValidationError struct {
	Failures []Failure
}

func (err *ValidationError) Error() string {
	if err == nil || len(err.Failures) == 0 {
		return ""
	}

	failures := append([]Failure(nil), err.Failures...)
	sortFailures(failures)
	var builder strings.Builder
	builder.WriteString("backend conformance failed:")
	for _, failure := range failures {
		fmt.Fprintf(&builder, "\n- %s: identifier %q classification %s: %s",
			diagnosticSource(failure.Reference), failure.Reference.Identifier,
			failure.Classification, failure.Detail)
	}
	return builder.String()
}

func diagnosticSource(reference Reference) string {
	if strings.TrimSpace(reference.Source) == "" {
		return "source <unknown>"
	}
	return "source " + reference.Source
}

func sortFailures(failures []Failure) {
	sort.SliceStable(failures, func(left, right int) bool {
		leftSource := failures[left].Reference.Source
		rightSource := failures[right].Reference.Source
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		if failures[left].Reference.Identifier != failures[right].Reference.Identifier {
			return failures[left].Reference.Identifier < failures[right].Reference.Identifier
		}
		if failures[left].Classification != failures[right].Classification {
			return failures[left].Classification < failures[right].Classification
		}
		return failures[left].Detail < failures[right].Detail
	})
}
