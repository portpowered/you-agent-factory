package service

import (
	"context"
	"errors"
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

	codexBinding, ok := registry.Resolve(workers.RunnerIDCodex)
	if !ok {
		t.Fatal("Resolve(codex) found = false")
	}
	if codexBinding.Identity != workers.RunnerIDCodex ||
		codexBinding.Metadata.ID != workers.RunnerIDCodex ||
		codexBinding.Runner != codex {
		t.Fatalf("Resolve(codex) = %#v, want matching registration", codexBinding)
	}
	geminiBinding, ok := registry.Resolve(workers.RunnerIDGemini)
	if !ok || geminiBinding.Runner != gemini {
		t.Fatalf("Resolve(gemini) = (%#v, %t), want matching registration", geminiBinding, ok)
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
	first, ok := registry.Resolve(workers.RunnerIDCodex)
	if !ok {
		t.Fatal("Resolve() found = false")
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
	second, _ := registry.Resolve(workers.RunnerIDCodex)
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
	registry, err := New([]runners.Registration{
		registration(workers.RunnerIDCodex, "Codex", &runnerSpy{}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, identity := range []string{"", "CODEX", " codex ", "unknown"} {
		if binding, ok := registry.Resolve(identity); ok ||
			binding.Identity != "" ||
			binding.Metadata.ID != "" ||
			binding.Runner != nil {
			t.Fatalf("Resolve(%q) = (%#v, %t), want empty miss", identity, binding, ok)
		}
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
