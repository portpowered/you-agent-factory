//go:build backendconformance

package backendconformance

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyRequiresExactlyOneSupplyingClass(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		inputs     Inputs
		wantKind   ResolutionKind
		wantDetail string
	}{
		{
			name:     "registered pinned backend",
			inputs:   Inputs{RegisteredBackends: []string{"localai-test"}},
			wantKind: ResolutionPinnedArtifact,
		},
		{
			name: "release-built command",
			inputs: Inputs{ReleaseBuiltCommands: []ReleaseBuiltCommand{{
				Command: "you", Evidence: "build-all -> build -> bin/you",
			}}},
			wantKind: ResolutionReleaseBuilt,
		},
		{
			name:   "unresolved",
			inputs: Inputs{}, wantKind: ResolutionUnresolved,
			wantDetail: "no registered pinned backend",
		},
		{
			name: "ambiguous registry and release build",
			inputs: Inputs{
				RegisteredBackends: []string{"localai-test"},
				ReleaseBuiltCommands: []ReleaseBuiltCommand{{
					Command: "localai-test", Evidence: "release target",
				}},
			},
			wantKind:   ResolutionAmbiguous,
			wantDetail: "matches 1 registered pinned backend records and 1 release-built command records",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reference := Reference{Identifier: referenceIdentifier(tc.name), Source: "fixture factory"}
			resolution, err := Classify(reference, tc.inputs)
			if tc.wantKind == ResolutionPinnedArtifact || tc.wantKind == ResolutionReleaseBuilt {
				if err != nil {
					t.Fatalf("Classify() error = %v", err)
				}
				if resolution.Kind != tc.wantKind {
					t.Fatalf("Classify() kind = %q, want %q", resolution.Kind, tc.wantKind)
				}
				return
			}

			if err == nil {
				t.Fatalf("Classify() error = nil, want %s", tc.wantKind)
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || len(validation.Failures) != 1 {
				t.Fatalf("Classify() error = %#v, want one ValidationError", err)
			}
			if validation.Failures[0].Classification != tc.wantKind {
				t.Fatalf("classification = %q, want %q", validation.Failures[0].Classification, tc.wantKind)
			}
			if tc.wantDetail != "" && !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("error = %q, want detail containing %q", err, tc.wantDetail)
			}
		})
	}
}

func TestValidateAcceptsCompletePinnedBackendAndReleaseBuiltCommand(t *testing.T) {
	t.Parallel()

	inputs := Inputs{
		References: []Reference{
			{Identifier: "localai-test", Source: "generated/factories/positive/factory.json"},
			{Identifier: "you", Source: "generated/factories/release/factory.json"},
		},
		RegisteredBackends: []string{"localai-test"},
		PinnedArtifacts:    completePinnedArtifacts("localai-test"),
		ReleaseBuiltCommands: []ReleaseBuiltCommand{{
			Command: "you", Evidence: "build-all -> build -> bin/you",
		}},
	}

	if err := Validate(inputs); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsIncompletePinnedArtifactMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		mutate     func([]PinnedArtifact) []PinnedArtifact
		wantDetail string
	}{
		{
			name: "missing target",
			mutate: func(artifacts []PinnedArtifact) []PinnedArtifact {
				return artifacts[:2]
			},
			wantDetail: `missing the required target "windows-amd64"`,
		},
		{
			name: "size exactly one MiB",
			mutate: func(artifacts []PinnedArtifact) []PinnedArtifact {
				artifacts[0].SizeBytes = MinimumPinnedArtifactSizeBytes
				return artifacts
			},
			wantDetail: "must be strictly greater than 1048576 bytes",
		},
		{
			name: "size below one MiB",
			mutate: func(artifacts []PinnedArtifact) []PinnedArtifact {
				artifacts[1].SizeBytes = MinimumPinnedArtifactSizeBytes - 1
				return artifacts
			},
			wantDetail: "target \"linux-amd64\" sizeBytes 1048575",
		},
		{
			name: "duplicate target",
			mutate: func(artifacts []PinnedArtifact) []PinnedArtifact {
				return append(artifacts, artifacts[0])
			},
			wantDetail: `exactly one is required`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Inputs{
				References:         []Reference{{Identifier: "localai-test", Source: "manifest fixture"}},
				RegisteredBackends: []string{"localai-test"},
				PinnedArtifacts:    tc.mutate(completePinnedArtifacts("localai-test")),
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("Validate() error = %v, want detail containing %q", err, tc.wantDetail)
			}
		})
	}
}

func TestVariantCharacterizationRetainsClosedTargetBaselineAndUniquenessFailures(t *testing.T) {
	t.Parallel()

	const backendID = "localai-llamacpp"
	cuda := PinnedArtifact{
		BackendID: backendID, TargetID: "windows-amd64-cuda",
		SizeBytes: MinimumPinnedArtifactSizeBytes + 4,
	}
	validBaselines := completePinnedArtifacts(backendID)

	tests := []struct {
		name         string
		artifacts    []PinnedArtifact
		wantDetails  []string
		wantFailures int
	}{
		{
			name:         "approved CUDA variant with all baselines is outside current closed matrix",
			artifacts:    append(append([]PinnedArtifact(nil), validBaselines...), cuda),
			wantDetails:  []string{"outside the closed darwin-arm64, linux-amd64, windows-amd64 matrix"},
			wantFailures: 1,
		},
		{
			name:      "CUDA variant cannot replace the Windows CPU baseline",
			artifacts: append(append([]PinnedArtifact(nil), validBaselines[:2]...), cuda),
			wantDetails: []string{
				"outside the closed darwin-arm64, linux-amd64, windows-amd64 matrix",
				`missing the required target "windows-amd64"`,
			},
			wantFailures: 2,
		},
		{
			name:         "duplicate Windows CPU tuple remains invalid",
			artifacts:    append(append([]PinnedArtifact(nil), validBaselines...), validBaselines[2]),
			wantDetails:  []string{`exactly one is required`},
			wantFailures: 1,
		},
		{
			name:         "duplicate Windows CUDA tuple remains invalid",
			artifacts:    append(append(append([]PinnedArtifact(nil), validBaselines...), cuda), cuda),
			wantDetails:  []string{"outside the closed darwin-arm64, linux-amd64, windows-amd64 matrix"},
			wantFailures: 2,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Inputs{
				References:         []Reference{{Identifier: backendID, Source: "candidate manifest"}},
				RegisteredBackends: []string{backendID},
				PinnedArtifacts:    tc.artifacts,
			})
			if err == nil {
				t.Fatal("Validate() error = nil, want the current baseline/closed-target rejection")
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || len(validation.Failures) != tc.wantFailures {
				t.Fatalf("Validate() error = %#v, want %d deterministic failures", err, tc.wantFailures)
			}
			message := err.Error()
			for _, expected := range tc.wantDetails {
				if !strings.Contains(message, expected) {
					t.Fatalf("Validate() error = %q, want detail containing %q", message, expected)
				}
			}
		})
	}
}

func TestValidateNamesFactoryAndCommandForOmnivoiceShapedReference(t *testing.T) {
	t.Parallel()

	const command = "omnivoice-llamacpp"
	err := Validate(Inputs{
		References: []Reference{{
			Identifier: command,
			Source:     "generated/factories/tts/factory.json (worker tts-executor)",
		}},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want unresolved reference")
	}
	message := err.Error()
	for _, expected := range []string{"generated/factories/tts/factory.json", command, "unresolved"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("Validate() error = %q, want %q", message, expected)
		}
	}
}

func TestValidateRequiresConcreteReleaseBuildEvidence(t *testing.T) {
	t.Parallel()

	err := Validate(Inputs{
		References: []Reference{{Identifier: "repo-command", Source: "release fixture"}},
		ReleaseBuiltCommands: []ReleaseBuiltCommand{{
			Command: "repo-command",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "concrete Makefile") {
		t.Fatalf("Validate() error = %v, want missing release-build evidence", err)
	}
}

func TestValidationErrorIsDeterministicAcrossReferenceOrder(t *testing.T) {
	t.Parallel()

	first := Validate(Inputs{References: []Reference{
		{Identifier: "z-command", Source: "z-source"},
		{Identifier: "a-command", Source: "a-source"},
	}})
	second := Validate(Inputs{References: []Reference{
		{Identifier: "a-command", Source: "a-source"},
		{Identifier: "z-command", Source: "z-source"},
	}})
	if first == nil || second == nil || first.Error() != second.Error() {
		t.Fatalf("validation errors differ:\nfirst=%v\nsecond=%v", first, second)
	}
}

func completePinnedArtifacts(backendID string) []PinnedArtifact {
	return []PinnedArtifact{
		{BackendID: backendID, TargetID: TargetDarwinArm64, SizeBytes: MinimumPinnedArtifactSizeBytes + 1},
		{BackendID: backendID, TargetID: TargetLinuxAmd64, SizeBytes: MinimumPinnedArtifactSizeBytes + 2},
		{BackendID: backendID, TargetID: TargetWindowsAmd64, SizeBytes: MinimumPinnedArtifactSizeBytes + 3},
	}
}

func referenceIdentifier(name string) string {
	switch name {
	case "registered pinned backend":
		return "localai-test"
	case "release-built command":
		return "you"
	case "ambiguous registry and release build":
		return "localai-test"
	default:
		return name
	}
}
