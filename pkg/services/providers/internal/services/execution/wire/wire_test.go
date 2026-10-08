package wire

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

func TestNewACPRegistrationRoutesOnlyPrivateContinuationInput(t *testing.T) {
	t.Parallel()

	fake := &continuationACPServiceFake{}
	registration := NewACPRegistration("cursor", fake)
	request := providers.ExecuteRequest{Provider: "cursor", AttemptID: "attempt-1"}
	if _, err := registration.Attempt(context.Background(), request); err != nil {
		t.Fatalf("Attempt() error = %v", err)
	}
	if !reflect.DeepEqual(fake.executeRequest, request) {
		t.Fatalf("ACP Execute request = %#v, want %#v", fake.executeRequest, request)
	}

	reference := providers.SessionRef{Provider: "cursor", Kind: providers.SessionIDKind, ID: "session-1"}
	if _, err := registration.Continue(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: request,
		ResumeSession:  &reference,
	}); err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if fake.continueReference != reference || !reflect.DeepEqual(fake.continueRequest, request) {
		t.Fatalf("ACP Continue = (%#v, %#v), want (%#v, %#v)", fake.continueRequest, fake.continueReference, request, reference)
	}
	_, err := registration.Continue(context.Background(), execution.ContinuationRequest{ExecuteRequest: request})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Continue(missing private reference) error = %#v, want invalid request", err)
	}
}

type continuationACPServiceFake struct {
	acp.Service
	executeRequest    providers.ExecuteRequest
	continueRequest   providers.ExecuteRequest
	continueReference providers.SessionRef
}

func (fake *continuationACPServiceFake) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeRequest = request
	return providers.ExecuteResult{}, nil
}

func (fake *continuationACPServiceFake) Continue(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (providers.ExecuteResult, error) {
	fake.continueRequest = request
	fake.continueReference = reference
	return providers.ExecuteResult{}, nil
}

var _ acp.ContinuationService = (*continuationACPServiceFake)(nil)

func TestNativeEffectsUseSuppliedClockAndRunner(t *testing.T) {
	t.Parallel()
	for _, provider := range []providers.ID{providers.IDCodex, providers.IDClaude, providers.IDAntigravity} {
		t.Run(string(provider), func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0), 37*time.Millisecond)
			runner := &clockRecordingProviderRunner{clock: clock}
			adapted := runner
			request := execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{Provider: provider, UserMessage: "supplied effect"}}
			var output []byte
			observe := func(chunk []byte) error { output = append(output, chunk...); return nil }
			var duration int64
			var err error
			switch provider {
			case providers.IDCodex:
				result, effectErr := NewCodexEffect((adapted).commandEffect(), clock, nil, nil).Execute(t.Context(), request, observe)
				duration, err = result.DurationMillis, effectErr
			case providers.IDClaude:
				result, effectErr := NewClaudeEffect((adapted).commandEffect(), clock).Execute(t.Context(), request, observe)
				duration, err = result.DurationMillis, effectErr
			case providers.IDAntigravity:
				result, effectErr := NewAgyCommandEffect((adapted).commandEffect(), clock, clock).Execute(t.Context(), request, observe)
				duration, err = result.DurationMillis, effectErr
			}
			if err != nil || duration != 37 || string(output) != "ok" {
				t.Fatalf("effect = (%dms, %q, %v), want (37ms, ok, nil)", duration, output, err)
			}
			wantCommand := string(provider)
			if provider == providers.IDAntigravity {
				wantCommand = "agy"
			}
			if runner.calls != 1 || runner.request.Command != wantCommand {
				t.Fatalf("runner = (%d calls, %q), want one %q command", runner.calls, runner.request.Command, wantCommand)
			}
		})
	}
}

type clockRecordingProviderRunner struct {
	clock   *platformclock.Deterministic
	calls   int
	request providerservice.CommandRequest
}

func (r *clockRecordingProviderRunner) Run(_ context.Context, request providerservice.CommandRequest) (providerservice.CommandResult, error) {
	r.calls++
	r.request = request
	r.clock.SetTick(1)
	return providerservice.CommandResult{Stdout: []byte("ok")}, nil
}

// RunStreaming supplies the buffered fixture's completed stdout chunk.
func (r *clockRecordingProviderRunner) RunStreaming(ctx context.Context, request providerservice.CommandRequest, observe providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
	result, err := r.Run(ctx, request)
	if len(result.Stdout) > 0 && observe != nil {
		if observeErr := observe(providerservice.OutputStreamStdout, result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
}

func (r *clockRecordingProviderRunner) commandEffect() providerservice.CommandRunner {
	return providerservice.CommandRunner{Run: r.Run, RunStreaming: r.RunStreaming}
}
