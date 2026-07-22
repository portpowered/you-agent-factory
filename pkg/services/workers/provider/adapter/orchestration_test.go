package adapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

func TestRegistryLookupDoesNotDecodeNativeChunks(t *testing.T) {
	t.Parallel()

	fake := &kernelAdapter{}
	registry, err := adapter.NewRegistry(fake)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	selected, err := registry.Lookup(fake.Identity())
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if selected != fake {
		t.Fatalf("Lookup() = %T %p, want %p", selected, selected, fake)
	}
	if fake.decoder != nil {
		t.Fatal("registry lookup must not construct decoders")
	}
}

func TestExecuteKeepsNativeSyntaxInsideSelectedAdapter(t *testing.T) {
	t.Parallel()

	fake := &kernelAdapter{}
	registry, err := adapter.NewRegistry(fake)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner := &kernelRunner{observations: []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: []byte("quasar:hel")},
		{Stream: adapter.OutputStreamStderr, Chunk: []byte("ignored native warning")},
		{Stream: adapter.OutputStreamStdout, Chunk: []byte("lo")},
	}}
	result, err := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: "opaque-fixture",
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Model: "fixture-model", UserMessage: "private prompt",
		}},
		Decoder: adapter.DecoderContext{RunID: "run-1", DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.request.Command != "opaque-provider" || runner.request.Stdin == nil {
		t.Fatalf("runner request = %#v", runner.request)
	}
	if result.Outcome != adapter.CommandOutcomeCompleted || result.Response.Content != "hello" || result.Failure != nil {
		t.Fatalf("execution result = %#v", result)
	}
	if !result.Capabilities.NativeStreaming || len(result.Drafts) != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("neutral outputs = %#v", result)
	}
	if result.Drafts[0].Provenance.NativeEventType != "quasar" || fake.decoder.flushReason != adapter.FlushReasonCompleted {
		t.Fatalf("draft = %#v, flush reason = %q", result.Drafts[0], fake.decoder.flushReason)
	}
	if err := factorysessions.ValidateResponseEventDraft(result.Drafts[0]); err != nil {
		t.Fatalf("ValidateDraft() error = %v", err)
	}
}

func TestExecuteFlushesBeforeReturningEveryCommandOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prepare    func() (context.Context, *kernelRunner)
		outcome    adapter.CommandOutcome
		flush      adapter.FlushReason
		failureTyp workerexecution.WorkFailureType
	}{
		{name: "process failure", prepare: func() (context.Context, *kernelRunner) {
			return context.Background(), &kernelRunner{result: workerprocess.CommandResult{ExitCode: 7}}
		}, outcome: adapter.CommandOutcomeProcessFailed, flush: adapter.FlushReasonTerminated, failureTyp: workerexecution.WorkFailureTypeInternalServerError},
		{name: "canceled", prepare: func() (context.Context, *kernelRunner) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, &kernelRunner{err: context.Canceled}
		}, outcome: adapter.CommandOutcomeCanceled, flush: adapter.FlushReasonCanceled, failureTyp: workerexecution.WorkFailureTypeTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &kernelAdapter{}
			registry, err := adapter.NewRegistry(fake)
			if err != nil {
				t.Fatalf("NewRegistry() error = %v", err)
			}
			ctx, runner := tc.prepare()
			result, executeErr := adapter.Execute(ctx, registry, runner, adapter.ExecuteInput{Provider: fake.Identity()})
			if executeErr != nil {
				t.Fatalf("Execute() error = %v", executeErr)
			}
			if result.Outcome != tc.outcome || fake.decoder.flushReason != tc.flush {
				t.Fatalf("outcome = %q, flush = %q", result.Outcome, fake.decoder.flushReason)
			}
			if !fake.parseAfterFlush || result.Failure == nil || result.Failure.Type != tc.failureTyp {
				t.Fatalf("parse after flush = %t, failure = %#v", fake.parseAfterFlush, result.Failure)
			}
		})
	}
}

func TestExecuteRunsProviderOwnedFallbackOnceAndPublishesCapabilityUpdate(t *testing.T) {
	t.Parallel()
	fallback := &kernelAdapter{}
	selected := &kernelAdapter{
		fallback: fallback, shouldFallback: true,
		fallbackDiagnostic: adapter.Diagnostic{Code: "degraded", Message: "fixture selected final mode"},
	}
	registry, err := adapter.NewRegistry(selected)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner := &kernelRunner{}
	result, err := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{Provider: selected.Identity()})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.calls != 2 || result.Response.Content != "hello" || len(result.CapabilityUpdates) != 1 {
		t.Fatalf("fallback result = %#v, runner calls = %d", result, runner.calls)
	}
	update := result.CapabilityUpdates[0]
	if update.Provider != selected.Identity() || update.Diagnostic.Code != "degraded" || len(result.Diagnostics) != 1 {
		t.Fatalf("capability update = %#v, diagnostics = %#v", update, result.Diagnostics)
	}
}

func TestExecuteRejectsInvalidFallbackPlans(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		selected  *kernelAdapter
		wantError string
	}{
		{
			name:      "planner error",
			selected:  &kernelAdapter{fallbackErr: errors.New("unsafe fallback policy")},
			wantError: "plan provider adapter fallback",
		},
		{
			name:      "nil adapter",
			selected:  &kernelAdapter{shouldFallback: true},
			wantError: "nil adapter",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			registry, err := adapter.NewRegistry(tc.selected)
			if err != nil {
				t.Fatalf("NewRegistry() error = %v", err)
			}
			_, executeErr := adapter.Execute(context.Background(), registry, &kernelRunner{}, adapter.ExecuteInput{Provider: tc.selected.Identity()})
			if executeErr == nil || !strings.Contains(executeErr.Error(), tc.wantError) {
				t.Fatalf("Execute() error = %v, want %q", executeErr, tc.wantError)
			}
		})
	}
}

type kernelRunner struct {
	observations []adapter.Observation
	result       workerprocess.CommandResult
	err          error
	request      workerprocess.CommandRequest
	calls        int
}

func (r *kernelRunner) Run(_ context.Context, request workerprocess.CommandRequest, observe func(adapter.Observation) error) (workerprocess.CommandResult, error) {
	r.calls++
	r.request = request
	for _, observation := range r.observations {
		if err := observe(observation); err != nil {
			return r.result, err
		}
	}
	if r.result.Stdout == nil {
		r.result.Stdout = []byte("hello")
	}
	return r.result, r.err
}

type kernelAdapter struct {
	decoder            *kernelDecoder
	parseAfterFlush    bool
	fallback           adapter.Adapter
	shouldFallback     bool
	fallbackErr        error
	fallbackDiagnostic adapter.Diagnostic
}

func (*kernelAdapter) Identity() adapter.Identity { return "opaque-fixture" }

func (*kernelAdapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	return adapter.CommandBuildResult{Request: workerprocess.CommandRequest{
		Command: "opaque-provider", Stdin: []byte(input.Request.UserMessage),
	}}, nil
}

func (a *kernelAdapter) NewDecoder(context.Context, adapter.DecoderContext) (adapter.Decoder, error) {
	a.decoder = &kernelDecoder{}
	return a.decoder, nil
}

func (a *kernelAdapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	a.parseAfterFlush = a.decoder != nil && a.decoder.flushed
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{Content: string(input.CommandResult.Stdout)}}, nil
}

func (*kernelAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{NativeStreaming: true, MessageSnapshots: true}}, nil
}

func (*kernelAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.FlushReason == adapter.FlushReasonCompleted {
		return adapter.FailureResult{}
	}
	failureType := workerexecution.WorkFailureTypeInternalServerError
	if input.FlushReason == adapter.FlushReasonCanceled {
		failureType = workerexecution.WorkFailureTypeTimeout
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: workerexecution.WorkFailureFamilyRetryable, Type: failureType,
		Message: "opaque provider did not complete", Retry: adapter.RetryGuidance{Retryable: true},
	}}
}

func (a *kernelAdapter) PlanFallback(context.Context, adapter.FallbackContext) (adapter.FallbackPlan, bool, error) {
	return adapter.FallbackPlan{Adapter: a.fallback, Diagnostic: a.fallbackDiagnostic}, a.shouldFallback, a.fallbackErr
}

type kernelDecoder struct {
	buffer      []byte
	flushed     bool
	flushReason adapter.FlushReason
}

func (d *kernelDecoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: "native_warning", Message: "provider warning omitted"}}}, nil
	}
	d.buffer = append(d.buffer, observation.Chunk...)
	return adapter.DecodeResult{}, nil
}

func (d *kernelDecoder) Flush(_ context.Context, input adapter.FlushContext) (adapter.DecodeResult, error) {
	d.flushed, d.flushReason = true, input.Reason
	if len(d.buffer) == 0 {
		return adapter.DecodeResult{}, nil
	}
	nativeType, content, found := bytes.Cut(d.buffer, []byte(":"))
	if !found {
		return adapter.DecodeResult{}, errors.New("opaque record was malformed")
	}
	payload, err := json.Marshal(factorysessions.ResponseEventMessage{Role: "assistant", ContentBlocks: []factorysessions.ResponseEventContentBlock{{Kind: factorysessions.ResponseEventContentBlockText, Text: string(content)}}})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return adapter.DecodeResult{Drafts: []factorysessions.ResponseEventDraft{{
		RunID: "run-1", DispatchID: "dispatch-1", Kind: factorysessions.ResponseEventKindMessage, Phase: factorysessions.ResponseEventPhaseCompleted,
		Provenance: factorysessions.ResponseEventProvenance{Provider: "opaque-fixture", NativeEventType: string(nativeType), Delivery: factorysessions.ResponseEventDeliveryNativeStream, Representation: factorysessions.ResponseEventRepresentationSnapshot, Fidelity: factorysessions.ResponseEventFidelityNormalized},
		Payload:    payload,
	}}}, nil
}

var _ adapter.Adapter = (*kernelAdapter)(nil)
var _ adapter.FallbackPlanner = (*kernelAdapter)(nil)
var _ adapter.Decoder = (*kernelDecoder)(nil)
var _ adapter.StreamingCommandRunner = (*kernelRunner)(nil)
