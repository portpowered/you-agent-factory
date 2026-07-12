package providerexecution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestProviderExecutorExecuteMapsCanonicalSuccessMetadata(t *testing.T) {
	provider := &executionTestProvider{response: interfaces.InferenceResponse{
		Content: "done",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: string(interfaces.ModelProviderCursor), Kind: "session_id", ID: "sess-1",
		},
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{Provider: "cursor", ResponseMetadata: map[string]string{"content_bytes": "4"}},
			Command:  &interfaces.CommandDiagnostic{Stdin: "secret prompt", Env: map[string]string{"API_KEY": "secret"}},
		},
	}}

	result, err := NewProviderExecutor(provider).Execute(context.Background(), ExecutionInput{
		Request: interfaces.ProviderInferenceRequest{UserMessage: "hello"}, Attempt: 3,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.calls != 1 || result.Attempt != 3 || result.Response.Content != "done" {
		t.Fatalf("result = %#v, calls = %d", result, provider.calls)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "cursor" || result.ProviderSession.ID != "sess-1" {
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
	providerErr := workerprovider.NewProviderErrorWithSession(
		interfaces.WorkFailureTypeThrottled,
		"Provider capacity is temporarily unavailable.",
		errors.New("exit status 1"),
		&interfaces.ProviderSessionMetadata{Provider: string(interfaces.ModelProviderCursor), Kind: "session_id", ID: "sess-failed"},
	)
	provider := &executionTestProvider{err: providerErr}

	result, err := NewProviderExecutor(provider).Execute(context.Background(), ExecutionInput{})
	if !errors.Is(err, providerErr) || provider.calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, provider.calls)
	}
	if result.Attempt != 1 || result.FailureMetadata == nil || result.FailureMetadata.Type != interfaces.WorkFailureTypeThrottled {
		t.Fatalf("failure result = %#v", result)
	}
	if result.FailureDetail == nil || result.FailureDetail.Message != "Provider is temporarily unavailable due to usage or capacity limits." {
		t.Fatalf("failure detail = %#v", result.FailureDetail)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider session = %#v", result.ProviderSession)
	}
}

func TestProviderExecutorExecutePropagatesCancellationWithoutRetry(t *testing.T) {
	provider := newBlockingExecutionTestProvider()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result ExecutionResult
		err    error
	}, 1)
	go func() {
		result, err := NewProviderExecutor(provider).Execute(ctx, ExecutionInput{Attempt: 2})
		done <- struct {
			result ExecutionResult
			err    error
		}{result, err}
	}()
	<-provider.started
	cancel()
	completed := <-done
	if !errors.Is(completed.err, context.Canceled) || provider.callCount() != 1 {
		t.Fatalf("err = %v, calls = %d", completed.err, provider.callCount())
	}
	if completed.result.FailureDetail == nil || completed.result.FailureDetail.Reason != interfaces.WorkFailureTypeUnknown {
		t.Fatalf("failure detail = %#v", completed.result.FailureDetail)
	}
}

func TestProviderExecutorExecuteClassifiesDeadline(t *testing.T) {
	provider := &executionTestProvider{err: context.DeadlineExceeded}
	result, err := NewProviderExecutor(provider).Execute(context.Background(), ExecutionInput{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if result.FailureDetail == nil || result.FailureDetail.Reason != interfaces.WorkFailureTypeTimeout || result.FailureDetail.Message != "Provider request timed out." {
		t.Fatalf("failure detail = %#v", result.FailureDetail)
	}
}

func TestProviderExecutorExecuteBoundsAndRedactsFailureDiagnostics(t *testing.T) {
	secret := "token=super-secret " + strings.Repeat("x", 2048)
	providerErr := workerprovider.NewProviderErrorWithSession(
		interfaces.WorkFailureTypePermanentBadRequest,
		secret,
		errors.New(secret),
		nil,
	)
	providerErr.Diagnostics = &interfaces.WorkDiagnostics{
		Provider: &interfaces.ProviderDiagnostic{ResponseMetadata: map[string]string{"content_bytes": "20"}},
		Command:  &interfaces.CommandDiagnostic{Stdout: secret, Stderr: secret},
	}
	result, _ := NewProviderExecutor(&executionTestProvider{err: providerErr}).Execute(context.Background(), ExecutionInput{})
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
		reason  interfaces.WorkFailureType
		message string
	}{
		{interfaces.WorkFailureTypeAuthFailure, "Provider authentication failed."},
		{interfaces.WorkFailureTypePermanentBadRequest, "Provider rejected the request as invalid."},
		{interfaces.WorkFailureTypeThrottled, "Provider is temporarily unavailable due to usage or capacity limits."},
		{interfaces.WorkFailureTypeInternalServerError, "Provider encountered a temporary server error."},
		{interfaces.WorkFailureTypeTimeout, "Provider request timed out."},
		{interfaces.WorkFailureTypeMisconfigured, "Provider command could not be started."},
		{interfaces.WorkFailureTypeUnknown, "Provider execution failed."},
	}
	for _, tc := range tests {
		t.Run(string(tc.reason), func(t *testing.T) {
			providerErr := workerprovider.NewProviderError(tc.reason, sensitive, errors.New(sensitive))
			result, _ := NewProviderExecutor(&executionTestProvider{err: providerErr}).Execute(context.Background(), ExecutionInput{})
			if result.FailureDetail == nil || result.FailureDetail.Message != tc.message {
				t.Fatalf("failure detail = %#v, want message %q", result.FailureDetail, tc.message)
			}
			if strings.Contains(result.FailureDetail.Message, "sk-secret") || strings.Contains(result.FailureDetail.Message, "raw prompt") {
				t.Fatalf("failure detail exposed provider text: %#v", result.FailureDetail)
			}
		})
	}
}

type executionTestProvider struct {
	response interfaces.InferenceResponse
	err      error
	calls    int
}

func (p *executionTestProvider) Infer(context.Context, interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.calls++
	return p.response, p.err
}

type blockingExecutionTestProvider struct {
	started chan struct{}
	mu      sync.Mutex
	calls   int
}

func newBlockingExecutionTestProvider() *blockingExecutionTestProvider {
	return &blockingExecutionTestProvider{started: make(chan struct{})}
}

func (p *blockingExecutionTestProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		close(p.started)
	}
	p.mu.Unlock()
	<-ctx.Done()
	return interfaces.InferenceResponse{}, ctx.Err()
}

func (p *blockingExecutionTestProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}
