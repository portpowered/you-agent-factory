package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

type runnerSpy struct {
	calls int
}

func (runner *runnerSpy) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.calls++
	return workers.RunnerExecutionResult{}, nil
}

func TestNewResolvesMatchingDetachedRegistrationsWithoutExecution(t *testing.T) {
	codex := &runnerSpy{}
	gemini := &runnerSpy{}
	registrations := []runners.Registration{
		registration(workers.RunnerIDCodex, "Codex", codex),
		registration(workers.RunnerIDGemini, "Gemini", gemini),
	}

	registry, err := New(registrations)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	codexBinding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
	})
	if err != nil {
		t.Fatalf("Resolve(codex) error = %v", err)
	}
	if codexBinding.Identity != workers.RunnerIDCodex ||
		codexBinding.Metadata.ID != workers.RunnerIDCodex ||
		codexBinding.Runner != codex {
		t.Fatalf("Resolve(codex) = %#v, want matching registration", codexBinding)
	}
	geminiBinding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDGemini,
	})
	if err != nil || geminiBinding.Runner != gemini {
		t.Fatalf("Resolve(gemini) = (%#v, %v), want matching registration", geminiBinding, err)
	}
	if codex.calls != 0 || gemini.calls != 0 {
		t.Fatalf("runner calls = (%d, %d), want zero", codex.calls, gemini.calls)
	}
}

func TestNewSnapshotsCallerOwnedRegistrationMetadata(t *testing.T) {
	input := registration(workers.RunnerIDCodex, "Codex", &runnerSpy{})
	registry, err := New([]runners.Registration{input})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	input.Metadata.Capabilities.Baseline[0] = "mutated"
	input.Metadata.Capabilities.Optional[0].Status = "mutated"
	first, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if first.Metadata.Capabilities.Baseline[0] !=
		workers.RunnerBaselineCapabilityPromptSubmission {
		t.Fatalf("resolved baseline = %#v, want construction snapshot", first.Metadata.Capabilities.Baseline)
	}
	if first.Metadata.Capabilities.Optional[0].Status !=
		workers.RunnerOptionalCapabilityStatusSupported {
		t.Fatalf("resolved optional = %#v, want construction snapshot", first.Metadata.Capabilities.Optional)
	}

	first.Metadata.Capabilities.Baseline[0] = "mutated-again"
	first.Metadata.Capabilities.Optional[0].Detail = "mutated"
	second, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
	})
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if second.Metadata.Capabilities.Baseline[0] !=
		workers.RunnerBaselineCapabilityPromptSubmission ||
		second.Metadata.Capabilities.Optional[0].Detail != "" {
		t.Fatalf("second Resolve() metadata = %#v, want detached result", second.Metadata)
	}
}

func TestNewRejectsInvalidRegistryAtomically(t *testing.T) {
	var typedNil *runnerSpy
	valid := registration(workers.RunnerIDCodex, "Codex", &runnerSpy{})
	cases := []struct {
		name          string
		registrations []runners.Registration
		want          error
	}{
		{
			name: "empty identity",
			registrations: []runners.Registration{{
				Metadata: valid.Metadata,
				Runner:   &runnerSpy{},
			}},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "noncanonical identity",
			registrations: []runners.Registration{registration(
				" Codex ",
				"Codex",
				&runnerSpy{},
			)},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "identity metadata conflict",
			registrations: []runners.Registration{{
				Identity: workers.RunnerIDCodex,
				Metadata: validMetadata(workers.RunnerIDGemini, "Gemini"),
				Runner:   &runnerSpy{},
			}},
			want: workers.ErrConflictingRunnerRegistration,
		},
		{
			name: "malformed metadata",
			registrations: []runners.Registration{{
				Identity: workers.RunnerIDCodex,
				Metadata: validMetadata(workers.RunnerIDCodex, " "),
				Runner:   &runnerSpy{},
			}},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "invalid capabilities",
			registrations: []runners.Registration{{
				Identity: workers.RunnerIDCodex,
				Metadata: workers.RunnerMetadata{
					ID:          workers.RunnerIDCodex,
					DisplayName: "Codex",
					Capabilities: workers.RunnerCapabilities{
						Baseline: []workers.RunnerBaselineCapability{"unknown"},
					},
				},
				Runner: &runnerSpy{},
			}},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "nil implementation",
			registrations: []runners.Registration{{
				Identity: workers.RunnerIDCodex,
				Metadata: valid.Metadata,
			}},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "typed nil implementation",
			registrations: []runners.Registration{{
				Identity: workers.RunnerIDCodex,
				Metadata: valid.Metadata,
				Runner:   typedNil,
			}},
			want: workers.ErrInvalidRunnerRegistration,
		},
		{
			name: "duplicate canonical identity",
			registrations: []runners.Registration{
				valid,
				registration(workers.RunnerIDCodex, "Codex duplicate", &runnerSpy{}),
			},
			want: workers.ErrDuplicateRunnerRegistration,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry, err := New(test.registrations)
			if !errors.Is(err, test.want) {
				t.Fatalf("New() error = %v, want %v", err, test.want)
			}
			if registry != nil {
				t.Fatalf("New() registry = %#v, want nil after atomic rejection", registry)
			}
		})
	}
}

func TestResolveRequiresExactCanonicalIdentity(t *testing.T) {
	codex := &runnerSpy{}
	gemini := &runnerSpy{}
	registry, err := New([]runners.Registration{
		registration(workers.RunnerIDCodex, "Codex", codex),
		registration(workers.RunnerIDGemini, "Gemini", gemini),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, identity := range []string{"", "CODEX", " codex ", "unknown"} {
		binding, resolveErr := registry.Resolve(runners.ResolutionRequest{
			Identity: identity,
		})
		want := workers.ErrUnknownRunnerSelection
		if strings.TrimSpace(identity) == "" {
			want = workers.ErrMissingRunnerSelection
		}
		if !errors.Is(resolveErr, want) ||
			binding.Identity != "" ||
			binding.Metadata.ID != "" ||
			binding.Runner != nil {
			t.Fatalf("Resolve(%q) = (%#v, %v), want empty %v", identity, binding, resolveErr, want)
		}
	}
	if codex.calls != 0 || gemini.calls != 0 {
		t.Fatalf("runner calls = (%d, %d), want zero", codex.calls, gemini.calls)
	}
}

func TestResolveRequiresSupportedCapabilitiesWithoutExecution(t *testing.T) {
	codex := &runnerSpy{}
	gemini := &runnerSpy{}
	registry, err := New([]runners.Registration{
		registration(workers.RunnerIDCodex, "Codex", codex),
		registration(workers.RunnerIDGemini, "Gemini", gemini),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
	})
	if err != nil {
		t.Fatalf("Resolve(supported) error = %v", err)
	}
	second, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
	})
	if err != nil {
		t.Fatalf("second Resolve(supported) error = %v", err)
	}
	if first.Runner != codex || second.Runner != codex ||
		first.Identity != second.Identity ||
		!reflect.DeepEqual(first.Metadata, second.Metadata) {
		t.Fatalf("repeated resolution = (%#v, %#v), want equivalent binding facts", first, second)
	}

	first.Metadata.Capabilities.Optional[0].Status = "mutated"
	if second.Metadata.Capabilities.Optional[0].Status !=
		workers.RunnerOptionalCapabilityStatusSupported {
		t.Fatalf("second metadata = %#v, want detached collections", second.Metadata)
	}
	if codex.calls != 0 || gemini.calls != 0 {
		t.Fatalf("runner calls = (%d, %d), want zero", codex.calls, gemini.calls)
	}
}

func TestResolveRejectsUnsupportedCapabilityWithSafeContext(t *testing.T) {
	codex := &runnerSpy{}
	gemini := &runnerSpy{}
	registry, err := New([]runners.Registration{
		registration(workers.RunnerIDCodex, "Codex", codex),
		registration(workers.RunnerIDGemini, "Gemini", gemini),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: workers.RunnerIDCodex,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if !errors.Is(err, workers.ErrUnsupportedRunnerCapability) {
		t.Fatalf("Resolve(unsupported) error = %v, want canonical capability error", err)
	}
	var capabilityErr *workers.UnsupportedRunnerCapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("Resolve(unsupported) error = %T, want structured capability error", err)
	}
	if capabilityErr.RunnerID != workers.RunnerIDCodex ||
		capabilityErr.Capability != workers.RunnerOptionalCapabilitySessionResume {
		t.Fatalf("capability error = %#v, want detached selection context", capabilityErr)
	}
	if binding.Identity != "" || binding.Metadata.ID != "" || binding.Runner != nil {
		t.Fatalf("Resolve(unsupported) binding = %#v, want unusable zero binding", binding)
	}
	if codex.calls != 0 || gemini.calls != 0 {
		t.Fatalf("runner calls = (%d, %d), want zero", codex.calls, gemini.calls)
	}
}

func registration(
	identity string,
	displayName string,
	runner workers.Runner,
) runners.Registration {
	return runners.Registration{
		Identity: identity,
		Metadata: validMetadata(identity, displayName),
		Runner:   runner,
	}
}

func validMetadata(identity string, displayName string) workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          identity,
		DisplayName: displayName,
		Capabilities: workers.RunnerCapabilities{
			Baseline: workers.V1BaselineCapabilities(),
			Optional: []workers.RunnerOptionalCapabilitySupport{{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			}},
		},
	}
}
