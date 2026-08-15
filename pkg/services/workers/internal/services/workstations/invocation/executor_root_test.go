package invocation_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
)

func TestProviderExecutorExecuteMapsCanonicalSuccessMetadata(t *testing.T) {
	provider := &executionTestProvider{response: workerexecution.InferenceResponse{
		Content: "done",
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: string(modelprovider.ProviderCodex), Kind: "session_id", ID: "sess-1",
		},
		Diagnostics: &workerexecution.WorkDiagnostics{
			Provider: &workerexecution.ProviderDiagnostic{Provider: "cursor", ResponseMetadata: map[string]string{"content_bytes": "4"}},
			Command:  &workerexecution.CommandDiagnostic{Stdin: "secret prompt", Env: map[string]string{"API_KEY": "secret"}},
		},
	}}

	result, err := workerinvocation.NewProviderExecutor(provider).Execute(context.Background(), workerexecution.InvocationInput{
		Request: workerexecution.ProviderInferenceRequest{UserMessage: "hello"}, Attempt: 3,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.calls != 1 || result.Attempt != 3 || result.Response.Content != "done" {
		t.Fatalf("result = %#v, calls = %d", result, provider.calls)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "codex" || result.ProviderSession.ID != "sess-1" {
		t.Fatalf("provider session = %#v", result.ProviderSession)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil || result.Diagnostics.Provider.ResponseMetadata["content_bytes"] != "4" {
		t.Fatalf("safe diagnostics = %#v", result.Diagnostics)
	}
	if result.Response.Diagnostics == nil || result.Response.Diagnostics.Command == nil {
		t.Fatalf("provider response diagnostics were unexpectedly rewritten: %#v", result.Response.Diagnostics)
	}
}

func TestProviderExecutorExecuteMapsCanonicalProviderFailure(t *testing.T) {
	providerErr := workerexecution.NewProviderErrorWithSession(
		workerexecution.WorkFailureTypeThrottled,
		"Provider capacity is temporarily unavailable.",
		errors.New("exit status 1"),
		&workerexecution.ProviderSessionMetadata{Provider: string(modelprovider.ProviderCodex), Kind: "session_id", ID: "sess-failed"},
	)
	provider := &executionTestProvider{err: providerErr}

	result, err := workerinvocation.NewProviderExecutor(provider).Execute(context.Background(), workerexecution.InvocationInput{})
	if !errors.Is(err, providerErr) || provider.calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, provider.calls)
	}
	if result.Attempt != 1 || result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("failure result = %#v", result)
	}
	if result.FailureDetail == nil || result.FailureDetail.Message != "Provider is temporarily unavailable due to usage or capacity limits." {
		t.Fatalf("failure detail = %#v", result.FailureDetail)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "codex" {
		t.Fatalf("provider session = %#v", result.ProviderSession)
	}
}

func TestProviderExecutorExecutePropagatesCancellationWithoutRetry(t *testing.T) {
	provider := newBlockingExecutionTestProvider()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result workerexecution.InvocationResult
		err    error
	}, 1)
	go func() {
		result, err := workerinvocation.NewProviderExecutor(provider).Execute(ctx, workerexecution.InvocationInput{Attempt: 2})
		done <- struct {
			result workerexecution.InvocationResult
			err    error
		}{result, err}
	}()
	<-provider.started
	cancel()
	completed := <-done
	if !errors.Is(completed.err, context.Canceled) || provider.callCount() != 1 {
		t.Fatalf("err = %v, calls = %d", completed.err, provider.callCount())
	}
	if completed.result.FailureDetail == nil || completed.result.FailureDetail.Reason != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("failure detail = %#v", completed.result.FailureDetail)
	}
}

func TestProviderExecutorExecuteClassifiesDeadline(t *testing.T) {
	provider := &executionTestProvider{err: context.DeadlineExceeded}
	result, err := workerinvocation.NewProviderExecutor(provider).Execute(context.Background(), workerexecution.InvocationInput{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if result.FailureDetail == nil || result.FailureDetail.Reason != workerexecution.WorkFailureTypeTimeout || result.FailureDetail.Message != "Provider request timed out." {
		t.Fatalf("failure detail = %#v", result.FailureDetail)
	}
}

func TestProviderExecutorExecuteBoundsAndRedactsFailureDiagnostics(t *testing.T) {
	secret := "token=super-secret " + strings.Repeat("x", 2048)
	providerErr := workerexecution.NewProviderErrorWithSession(
		workerexecution.WorkFailureTypePermanentBadRequest,
		secret,
		errors.New(secret),
		nil,
	)
	providerErr.Diagnostics = &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{ResponseMetadata: map[string]string{"content_bytes": "20"}},
		Command:  &workerexecution.CommandDiagnostic{Stdout: secret, Stderr: secret},
	}
	result, _ := workerinvocation.NewProviderExecutor(&executionTestProvider{err: providerErr}).Execute(context.Background(), workerexecution.InvocationInput{})
	if result.FailureDetail == nil || result.FailureDetail.Message != "Provider rejected the request as invalid." {
		t.Fatalf("failure detail = %#v", result.FailureDetail)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil || result.Diagnostics.Provider.ResponseMetadata["content_bytes"] != "20" {
		t.Fatalf("safe diagnostics = %#v", result.Diagnostics)
	}
}

func TestProviderExecutorExecuteUsesReasonAllowlistForAllPersistedFailures(t *testing.T) {
	sensitive := "API key sk-secret secret=hidden raw prompt and provider output"
	tests := []struct {
		reason  workerexecution.WorkFailureType
		message string
	}{
		{workerexecution.WorkFailureTypeAuthFailure, "Provider authentication failed."},
		{workerexecution.WorkFailureTypePermanentBadRequest, "Provider rejected the request as invalid."},
		{workerexecution.WorkFailureTypeThrottled, "Provider is temporarily unavailable due to usage or capacity limits."},
		{workerexecution.WorkFailureTypeInternalServerError, "Provider encountered a temporary server error."},
		{workerexecution.WorkFailureTypeTimeout, "Provider request timed out."},
		{workerexecution.WorkFailureTypeMisconfigured, "Provider command could not be started."},
		{workerexecution.WorkFailureTypeUnknown, "Provider execution failed."},
	}
	for _, tc := range tests {
		t.Run(string(tc.reason), func(t *testing.T) {
			providerErr := workerexecution.NewProviderError(tc.reason, sensitive, errors.New(sensitive))
			result, _ := workerinvocation.NewProviderExecutor(&executionTestProvider{err: providerErr}).Execute(context.Background(), workerexecution.InvocationInput{})
			if result.FailureDetail == nil || result.FailureDetail.Message != tc.message {
				t.Fatalf("failure detail = %#v, want message %q", result.FailureDetail, tc.message)
			}
			if strings.Contains(result.FailureDetail.Message, "sk-secret") || strings.Contains(result.FailureDetail.Message, "raw prompt") {
				t.Fatalf("failure detail exposed provider text: %#v", result.FailureDetail)
			}
		})
	}
}

func TestProviderExecutorContinuationUsesProvidersReferenceWithoutFreshExecute(t *testing.T) {
	provider := &continuationExecutionTestProvider{}
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: "thread", ID: "opaque-session"}
	result, err := workerinvocation.NewProviderExecutor(provider).Execute(
		context.Background(),
		workerexecution.InvocationInput{Request: workerexecution.ProviderInferenceRequest{
			ModelProvider: string(providers.IDCodex),
			Dispatch:      work.WorkDispatch{DispatchID: "attempt-continue"},
			ResumeSession: &reference,
		}},
	)
	if err != nil {
		t.Fatalf("Execute continuation: %v", err)
	}
	if provider.continueCalls != 1 || provider.executeCalls != 0 {
		t.Fatalf("provider calls = continue:%d execute:%d, want continuation only", provider.continueCalls, provider.executeCalls)
	}
	if provider.reference != reference {
		t.Fatalf("continued reference = %#v, want %#v", provider.reference, reference)
	}
	if result.Response.Content != "continued" {
		t.Fatalf("continuation content = %q, want continued", result.Response.Content)
	}
}

type executionTestProvider struct {
	testutil.ProviderServiceAdapter
	response workerexecution.InferenceResponse
	err      error
	calls    int
}

func (p *executionTestProvider) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.calls++
	return p.response, p.err
}

func (p *executionTestProvider) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if strings.TrimSpace(request.Identity) == "" {
		request.Identity = string(modelprovider.ProviderCodex)
	}
	return p.ProviderServiceAdapter.ResolveIdentity(ctx, request)
}

func (p *executionTestProvider) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if strings.TrimSpace(request.ID.String()) == "" {
		request.ID = providers.IDCodex
	}
	return p.ProviderServiceAdapter.ValidatePrerequisites(ctx, request)
}

func (p *executionTestProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter := p.ProviderServiceAdapter
	adapter.InferFunc = p.Infer
	return adapter.Execute(ctx, request)
}

func (p *executionTestProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	adapter := p.ProviderServiceAdapter
	adapter.InferFunc = p.Infer
	return adapter.Continue(ctx, request)
}

type blockingExecutionTestProvider struct {
	testutil.ProviderServiceAdapter
	started chan struct{}
	mu      sync.Mutex
	calls   int
}

type continuationExecutionTestProvider struct {
	testutil.ProviderServiceAdapter
	executeCalls  int
	continueCalls int
	reference     providers.SessionRef
}

func (provider *continuationExecutionTestProvider) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	provider.executeCalls++
	return providers.ExecuteResult{Content: "ordinary"}, nil
}

func (provider *continuationExecutionTestProvider) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	provider.continueCalls++
	provider.reference = request.Reference
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    providers.ExecuteResult{Content: "continued"},
	}, nil
}

func newBlockingExecutionTestProvider() *blockingExecutionTestProvider {
	return &blockingExecutionTestProvider{started: make(chan struct{})}
}

func (p *blockingExecutionTestProvider) Infer(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		close(p.started)
	}
	p.mu.Unlock()
	<-ctx.Done()
	return workerexecution.InferenceResponse{}, ctx.Err()
}

func (p *blockingExecutionTestProvider) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if strings.TrimSpace(request.Identity) == "" {
		request.Identity = string(modelprovider.ProviderCodex)
	}
	return p.ProviderServiceAdapter.ResolveIdentity(ctx, request)
}

func (p *blockingExecutionTestProvider) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if strings.TrimSpace(request.ID.String()) == "" {
		request.ID = providers.IDCodex
	}
	return p.ProviderServiceAdapter.ValidatePrerequisites(ctx, request)
}

func (p *blockingExecutionTestProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter := p.ProviderServiceAdapter
	adapter.InferFunc = p.Infer
	return adapter.Execute(ctx, request)
}

func (p *blockingExecutionTestProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	adapter := p.ProviderServiceAdapter
	adapter.InferFunc = p.Infer
	return adapter.Continue(ctx, request)
}

func (p *blockingExecutionTestProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestNewExecutorAbsentProviderYieldsNoExecutor lets composition treat "this
// process has no provider" as an absent invocation edge rather than one that
// fails at dispatch time. The provider-invocation route depends on exactly
// this: a nil executor there leaves the route unbound, which is how a runtime
// declares it hosts no such Worker.
func TestNewExecutorAbsentProviderYieldsNoExecutor(t *testing.T) {
	if executor := workerinvocation.NewExecutor(nil); executor != nil {
		t.Fatalf("NewExecutor(nil) = %#v, want nil", executor)
	}
	if executor := workerinvocation.NewExecutor(&executionTestProvider{}); executor == nil {
		t.Fatal("NewExecutor(provider) = nil, want a constructed invocation adapter")
	}
}

func TestRunnerExecutorPreservesResultAndNormalizesFailures(t *testing.T) {
	providerSession := &workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: "thread", ID: "runner-session"}
	diagnostics := &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{Provider: "codex", ResponseMetadata: map[string]string{"source": "runner"}},
	}
	runner := &runnerExecutorTestDouble{result: workerexecution.RunnerExecutionResult{
		Content:         "runner output",
		Outcome:         workerexecution.OutcomeAccepted,
		ProviderSession: providerSession,
		Diagnostics:     diagnostics,
	}}

	result, err := workerinvocation.NewRunnerExecutor(runner).Execute(
		context.Background(),
		workerexecution.InvocationInput{Attempt: 0, Request: workerexecution.ProviderInferenceRequest{UserMessage: "hello"}},
	)
	if err != nil {
		t.Fatalf("runner Execute() = %v", err)
	}
	if result.Attempt != 1 || result.Response.Content != "runner output" || result.Response.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("runner result = %#v, want normalized attempt and output", result)
	}
	if result.ProviderSession == providerSession || result.Response.ProviderSession == providerSession {
		t.Fatal("runner result shares ProviderSession pointer with the runner")
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil || result.Diagnostics.Provider.ResponseMetadata["source"] != "runner" {
		t.Fatalf("runner diagnostics = %#v, want preserved safe metadata", result.Diagnostics)
	}
	if runner.request.UserMessage != "hello" {
		t.Fatalf("runner request = %#v, want forwarded request", runner.request)
	}

	failure := errors.New("runner failed")
	failed, runErr := workerinvocation.NewRunnerExecutor(&runnerExecutorTestDouble{err: failure}).Execute(
		context.Background(), workerexecution.InvocationInput{Attempt: 4},
	)
	if !errors.Is(runErr, failure) || failed.Attempt != 4 || failed.FailureDetail == nil || failed.FailureDetail.Reason != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("runner failure = result %#v, err %v; want attempt 4 and unknown failure", failed, runErr)
	}
	if workerinvocation.NewRunnerExecutor(nil) != nil {
		t.Fatal("NewRunnerExecutor(nil) returned an executor")
	}
}

func TestProviderExecutorMapsStructuredRequestAndProviderDiagnostics(t *testing.T) {
	service := &invocationProviderServiceBase{
		identity: providers.IDCodex,
		result: providers.ExecuteResult{
			Content:    "provider output",
			SessionRef: &providers.SessionRef{Provider: providers.IDCodex, Kind: "thread", ID: "provider-session"},
			Diagnostics: &providers.ExecuteDiagnostics{
				Metadata: map[string]string{"request_id": "req-1"},
				Command: &providers.ExecuteCommandDiagnostics{
					Command: "codex", Args: []string{"exec"}, Env: map[string]string{"TOKEN": "redacted"},
					ExitCode: 0, DurationMS: 17, WorkingDir: "C:\\factory",
				},
				Panic: &providers.ExecutePanicDiagnostics{Message: "bounded panic", Stack: "bounded stack"},
			},
		},
	}
	originalMetadata := map[string]any{"nested": []any{"before"}}
	request := workerexecution.ProviderInferenceRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: "factory-1", RuntimeID: "runtime-1", GenerationID: "generation-1",
			DispatchID: "dispatch-correlation", AttemptID: "attempt-correlation", RequestID: "request-1", TraceID: "trace-1",
		},
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-fallback", TransitionID: "transition-1", WorkerType: "worker-type",
			WorkstationName: "workstation", ProjectID: "project-1", InputBindings: map[string][]string{"slot": {"value"}},
			Execution: work.ExecutionMetadata{RequestID: "request-fallback", TraceID: "trace-fallback", ReplayKey: "replay-1", WorkIDs: []string{"work-1"}},
		},
		WorkerType: "worker", WorkstationType: "workstation", RunnerID: "codex", ProjectID: "project-1",
		Model: "model-1", ModelOperation: "TTS", ModelProvider: "codex", UserMessage: "hello",
		ModelBindings: []workerexecution.ResolvedModelOperationBinding{{
			Slot: "prompt", Source: workerexecution.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"audio":"preserve"}`), Metadata: originalMetadata}},
		}},
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilitySessionResume,
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		},
		Args: []string{"--structured"}, EnvVars: map[string]string{"KEY": "value"},
		ProcessEnvironment: []string{"KEY=value"}, InputTokens: []any{"token"},
		OutputSchema: "schema", ToolExecutionMode: workerexecution.RunnerToolExecutionModeRequired,
	}
	result, err := workerinvocation.NewProviderExecutor(service).Execute(
		context.Background(), workerexecution.InvocationInput{Request: request, Attempt: 2},
	)
	if err != nil {
		t.Fatalf("provider Execute() = %v", err)
	}
	if result.Response.Content != "provider output" || result.ProviderSession == nil || result.ProviderSession.ID != "provider-session" {
		t.Fatalf("provider result = %#v, want content and exact session metadata", result)
	}
	captured := service.request
	if captured.Provider != providers.IDCodex || captured.AttemptID != "attempt-correlation" || captured.Correlation.ReplayKey != "replay-1" || captured.Correlation.WorkIDs[0] != "work-1" {
		t.Fatalf("captured request = %#v, want canonical provider request correlation", captured)
	}
	if len(captured.ModelBindings) != 1 || captured.ModelBindings[0].Source != string(workerexecution.ModelOperationBindingSourceInput) || captured.ModelBindings[0].Content[0].JSON[0] != '{' {
		t.Fatalf("captured model bindings = %#v, want structured binding", captured.ModelBindings)
	}
	if captured.RequiredCapabilities[0] != string(workerexecution.RunnerOptionalCapabilitySessionResume) || captured.ToolExecutionMode != string(workerexecution.RunnerToolExecutionModeRequired) {
		t.Fatalf("captured capability mapping = %#v, want provider-neutral names", captured)
	}
	originalMetadata["nested"].([]any)[0] = "after"
	if captured.ModelBindings[0].Content[0].Metadata["nested"].([]any)[0] != "before" {
		t.Fatal("provider request model binding aliases caller metadata")
	}
	if result.Response.Diagnostics == nil || result.Response.Diagnostics.Command == nil || result.Response.Diagnostics.Panic == nil {
		t.Fatalf("provider diagnostics = %#v, want command and panic facts", result.Response.Diagnostics)
	}
}

func TestProviderExecutorClassifiesProviderFailuresAndExactContinuationOutcomes(t *testing.T) {
	failureKinds := []struct {
		kind providers.ExecuteFailureKind
		want workerexecution.WorkFailureType
	}{
		{providers.ExecuteFailureKindCanceled, workerexecution.WorkFailureTypeUnknown},
		{providers.ExecuteFailureKindTimeout, workerexecution.WorkFailureTypeTimeout},
		{providers.ExecuteFailureKindAuthentication, workerexecution.WorkFailureTypeAuthFailure},
		{providers.ExecuteFailureKindInvalidRequest, workerexecution.WorkFailureTypePermanentBadRequest},
		{providers.ExecuteFailureKindCapabilityMismatch, workerexecution.WorkFailureTypePermanentBadRequest},
		{providers.ExecuteFailureKindThrottled, workerexecution.WorkFailureTypeThrottled},
		{providers.ExecuteFailureKindMisconfigured, workerexecution.WorkFailureTypeMisconfigured},
		{providers.ExecuteFailureKindDependency, workerexecution.WorkFailureTypeUnknown},
	}
	for _, testCase := range failureKinds {
		t.Run(string(testCase.kind), func(t *testing.T) {
			ref := &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "failed-session"}
			service := &invocationProviderServiceBase{
				identity: providers.IDCodex,
				executeErr: providers.ExecuteFailure{
					Kind: testCase.kind, Message: "provider detail", SessionRef: ref,
					Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{"safe": "yes"}},
				},
			}
			result, err := workerinvocation.NewProviderExecutor(service).Execute(context.Background(), workerexecution.InvocationInput{Attempt: 3, Request: workerexecution.ProviderInferenceRequest{ModelProvider: "codex"}})
			var providerErr *workerexecution.ProviderError
			if err == nil || !errors.As(err, &providerErr) || result.FailureDetail == nil || result.FailureDetail.Reason != testCase.want {
				t.Fatalf("result = %#v, err = %v, want %q provider failure", result, err, testCase.want)
			}
			if result.ProviderSession == nil || result.ProviderSession.ID != "failed-session" || result.Diagnostics == nil || result.Diagnostics.Provider == nil {
				t.Fatalf("normalized failure = %#v, want session and safe diagnostics", result)
			}
		})
	}

	unsupported := &invocationProviderServiceBase{identity: providers.IDCodex}
	unsupportedResult, unsupportedErr := workerinvocation.NewProviderExecutor(unsupported).Execute(context.Background(), workerexecution.InvocationInput{
		Request: workerexecution.ProviderInferenceRequest{
			Continuation: &workerexecution.ProviderContinuationRef{Provider: "codex", Kind: "thread", ProviderSessionID: "opaque-session"},
		},
	})
	if unsupportedErr == nil || unsupportedResult.FailureDetail == nil || unsupportedResult.FailureDetail.Reason != workerexecution.WorkFailureTypePermanentBadRequest || unsupported.executeCalls != 0 {
		t.Fatalf("unsupported continuation = result %#v, err %v, want typed failure without Execute", unsupportedResult, unsupportedErr)
	}

	invalid, invalidErr := workerinvocation.NewProviderExecutor(&invocationProviderServiceBase{identity: providers.IDCodex}).Execute(context.Background(), workerexecution.InvocationInput{
		Request: workerexecution.ProviderInferenceRequest{Continuation: &workerexecution.ProviderContinuationRef{Provider: "codex"}},
	})
	if invalidErr == nil || invalid.FailureDetail == nil || invalid.FailureDetail.Reason != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("invalid continuation = result %#v, err %v, want typed invalid failure", invalid, invalidErr)
	}
}

func TestProviderExecutorRejectsCanceledContextAndMissingProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := workerinvocation.NewProviderExecutor(&invocationProviderServiceBase{identity: providers.IDCodex}).Execute(ctx, workerexecution.InvocationInput{Attempt: 7})
	if !errors.Is(err, context.Canceled) || result.Attempt != 7 || result.FailureDetail == nil {
		t.Fatalf("canceled context = result %#v, err %v, want canceled attempt", result, err)
	}
	missing, missingErr := workerinvocation.NewProviderExecutor(nil).Execute(context.Background(), workerexecution.InvocationInput{})
	if missingErr == nil || missing.FailureDetail == nil || missing.FailureDetail.Reason != workerexecution.WorkFailureTypeMisconfigured {
		t.Fatalf("missing provider = result %#v, err %v, want misconfigured failure", missing, missingErr)
	}
}

type runnerExecutorTestDouble struct {
	result  workerexecution.RunnerExecutionResult
	err     error
	request workerexecution.RunnerExecutionRequest
}

func (runner *runnerExecutorTestDouble) Execute(_ context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	runner.request = request
	return runner.result, runner.err
}

type invocationProviderServiceBase struct {
	identity     providers.ID
	result       providers.ExecuteResult
	executeErr   error
	request      providers.ExecuteRequest
	executeCalls int
}

func (service *invocationProviderServiceBase) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (service *invocationProviderServiceBase) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: service.canonicalIdentity()}}, nil
}

func (service *invocationProviderServiceBase) ResolveIdentity(_ context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	if strings.TrimSpace(request.Identity) == "" {
		return providers.ResolveIdentityResult{}, providers.ErrInvalidID
	}
	return providers.ResolveIdentityResult{ID: service.canonicalIdentity()}, nil
}

func (service *invocationProviderServiceBase) ResolveSelection(context.Context, providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return providers.ResolveSelectionResult{Provider: service.canonicalIdentity()}, nil
}

func (service *invocationProviderServiceBase) ValidatePrerequisites(context.Context, providers.ValidatePrerequisitesRequest) error {
	return nil
}

func (service *invocationProviderServiceBase) Execute(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	service.executeCalls++
	service.request = request.Clone()
	return service.result.Clone(), service.executeErr
}

func (service *invocationProviderServiceBase) canonicalIdentity() providers.ID {
	if service.identity == "" {
		return providers.IDCodex
	}
	return service.identity
}
