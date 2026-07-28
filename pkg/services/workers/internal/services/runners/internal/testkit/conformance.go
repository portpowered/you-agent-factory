// Package testkit provides reusable black-box conformance checks for common
// Workers Runner implementations without provider adapters or external effects.
package testkit

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const executionFailureMessage = "fixture execution failure"

// Subject supplies one Runner and explicit requests for the common behavioral
// contract. Later specialized runner packets can reuse Run with their own
// deterministic implementation.
type Subject struct {
	Runner             workers.Runner
	ValidRequest       workers.RunnerExecutionRequest
	InvalidRequest     workers.RunnerExecutionRequest
	UnsupportedRequest workers.RunnerExecutionRequest
	FailureRequest     workers.RunnerExecutionRequest
	ExpectedResult     workers.RunnerExecutionResult
	CapturedRequest    func() (workers.RunnerExecutionRequest, bool)
	AssertCaptured     func(*testing.T)
	// SkipUnsupportedCapability omits the unsupported-capability subtest for
	// Runners that delegate optional-capability policy to outer registry edges.
	SkipUnsupportedCapability bool
}

// Run proves success, caller-mutation isolation, normalized failures, and
// cancellation through only the common Workers Runner contract.
func Run(t *testing.T, subject Subject) {
	t.Helper()
	t.Run("success is detached", func(t *testing.T) {
		request := workers.CloneProviderInferenceRequest(subject.ValidRequest)
		result, err := subject.Runner.Execute(t.Context(), request)
		if err != nil {
			t.Fatalf("Execute(valid) error = %v", err)
		}
		if !reflect.DeepEqual(result, subject.ExpectedResult) {
			t.Fatalf("Execute(valid) = %#v, want %#v", result, subject.ExpectedResult)
		}

		mutateRequest(&request)
		if subject.AssertCaptured != nil {
			subject.AssertCaptured(t)
		} else {
			captured, ok := subject.CapturedRequest()
			if !ok || !reflect.DeepEqual(captured, subject.ValidRequest) {
				t.Fatalf("captured request = %#v, want detached %#v", captured, subject.ValidRequest)
			}
		}

		mutateResult(&result)
		later, err := subject.Runner.Execute(t.Context(), subject.ValidRequest)
		if err != nil {
			t.Fatalf("second Execute(valid) error = %v", err)
		}
		if !reflect.DeepEqual(later, subject.ExpectedResult) {
			t.Fatalf("second result = %#v, want detached %#v", later, subject.ExpectedResult)
		}
	})

	t.Run("invalid request is normalized", func(t *testing.T) {
		_, err := subject.Runner.Execute(t.Context(), subject.InvalidRequest)
		assertProviderFailure(t, err, workers.WorkFailureTypePermanentBadRequest)
	})

	t.Run("unsupported capability is normalized", func(t *testing.T) {
		if subject.SkipUnsupportedCapability {
			t.Skip("optional capability policy is enforced outside this Runner")
		}
		_, err := subject.Runner.Execute(t.Context(), subject.UnsupportedRequest)
		if !errors.Is(err, workers.ErrUnsupportedRunnerCapability) {
			t.Fatalf("Execute(unsupported) error = %v, want capability error", err)
		}
	})

	t.Run("cancellation is preserved", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := subject.Runner.Execute(ctx, subject.ValidRequest)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute(canceled) error = %v, want context.Canceled", err)
		}
	})

	t.Run("execution failure is normalized", func(t *testing.T) {
		_, err := subject.Runner.Execute(t.Context(), subject.FailureRequest)
		assertProviderFailure(t, err, workers.WorkFailureTypeInternalServerError)
	})
}

func assertProviderFailure(
	t *testing.T,
	err error,
	want workers.WorkFailureType,
) {
	t.Helper()
	var failure *workers.ProviderError
	if !errors.As(err, &failure) || failure.Type != want {
		t.Fatalf("Execute() error = %#v, want ProviderError type %q", err, want)
	}
}

func mutateRequest(request *workers.RunnerExecutionRequest) {
	request.InputTokens[0].(map[string]any)["nested"].([]any)[0] = "mutated"
	request.Dispatch.InputTokens[0].(map[string]any)["nested"].([]any)[0] = "mutated"
	request.EnvVars["FIXTURE"] = "mutated"
	request.ModelBindings[0].Content[0].Text = "mutated"
	request.ModelBindings[0].Content[0].Metadata["nested"].([]any)[0] = "mutated"
	request.RequiredOptionalCapabilities[0] = workers.RunnerOptionalCapabilitySessionResume
}

func mutateResult(result *workers.RunnerExecutionResult) {
	result.Content = "mutated"
	if result.ProviderSession != nil {
		result.ProviderSession.ID = "mutated"
	}
	if result.Diagnostics == nil {
		return
	}
	if result.Diagnostics.Metadata != nil {
		result.Diagnostics.Metadata["fixture"] = "mutated"
	}
	if result.Diagnostics.Provider != nil &&
		result.Diagnostics.Provider.ResponseMetadata != nil {
		result.Diagnostics.Provider.ResponseMetadata["status"] = "mutated"
	}
	if result.Diagnostics.Command != nil {
		result.Diagnostics.Command.Args = append(
			result.Diagnostics.Command.Args,
			"mutated",
		)
	}
}

// InMemoryRunner is a deterministic common-contract implementation used by
// the foundational conformance proof.
type InMemoryRunner struct {
	captured workers.RunnerExecutionRequest
	hasCall  bool
	result   workers.RunnerExecutionResult
}

var _ workers.Runner = (*InMemoryRunner)(nil)

// NewInMemorySubject returns the external-effect-free foundation subject.
func NewInMemorySubject() Subject {
	result := workers.RunnerExecutionResult{
		Content: "fixture success",
		ProviderSession: &workers.ProviderSessionMetadata{
			Provider: "memory",
			Kind:     "fixture",
			ID:       "session-1",
		},
		Diagnostics: &workers.WorkDiagnostics{
			Provider: &workers.ProviderDiagnostic{
				Provider:         "memory",
				ResponseMetadata: map[string]string{"status": "ok"},
			},
			Metadata: map[string]string{"fixture": "deterministic"},
		},
	}
	valid := workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-conformance",
			InputTokens: []any{map[string]any{
				"nested": []any{"dispatch-original"},
			}},
		},
		RunnerID: "memory",
		InputTokens: []any{map[string]any{
			"nested": []any{"original"},
		}},
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot: "prompt",
			Content: []work.WorkContentPart{{
				Type:     work.WorkContentPartTypeText,
				Text:     "original",
				Metadata: map[string]any{"nested": []any{"metadata-original"}},
			}},
		}},
		UserMessage: "run fixture",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
		EnvVars: map[string]string{"FIXTURE": "original"},
	}
	runner := &InMemoryRunner{result: cloneResult(result)}
	invalid := workers.CloneProviderInferenceRequest(valid)
	invalid.UserMessage = ""
	unsupported := workers.CloneProviderInferenceRequest(valid)
	unsupported.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
		workers.RunnerOptionalCapabilitySessionResume,
	}
	failure := workers.CloneProviderInferenceRequest(valid)
	failure.UserMessage = executionFailureMessage
	return Subject{
		Runner:             runner,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     result,
		CapturedRequest:    runner.CapturedRequest,
	}
}

// Execute validates the fixture request, snapshots it, and returns detached
// common-contract outcomes.
func (runner *InMemoryRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	if request.RunnerID == "" || request.UserMessage == "" {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"runner request is invalid",
			nil,
		)
	}
	for _, capability := range request.RequiredOptionalCapabilities {
		if capability != workers.RunnerOptionalCapabilityImageInput {
			return workers.RunnerExecutionResult{}, &workers.UnsupportedRunnerCapabilityError{
				RunnerID:   request.RunnerID,
				Capability: capability,
			}
		}
	}
	if request.UserMessage == executionFailureMessage {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypeInternalServerError,
			"runner execution failed",
			errors.New("deterministic fixture failure"),
		)
	}
	runner.captured = workers.CloneProviderInferenceRequest(request)
	runner.hasCall = true
	return cloneResult(runner.result), nil
}

// CapturedRequest returns a detached copy of the most recent successful input.
func (runner *InMemoryRunner) CapturedRequest() (
	workers.RunnerExecutionRequest,
	bool,
) {
	if runner == nil || !runner.hasCall {
		return workers.RunnerExecutionRequest{}, false
	}
	return workers.CloneProviderInferenceRequest(runner.captured), true
}

func cloneResult(result workers.RunnerExecutionResult) workers.RunnerExecutionResult {
	clone := result
	clone.ProviderSession = workers.CloneProviderSessionMetadata(result.ProviderSession)
	clone.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return clone
}
