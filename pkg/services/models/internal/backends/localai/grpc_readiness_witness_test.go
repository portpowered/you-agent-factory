package localai

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const readinessWitnessBudget = 30 * time.Second

func TestPinnedGRPCHostReadinessRemainsInLoadModelUntilCallerCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for controlled LocalAI peer: %v", err)
	}
	backend := &blockedLoadModelBackend{
		health:    make(chan time.Time, 1),
		loadModel: make(chan time.Time, 1),
		cancelled: make(chan time.Time, 1),
	}
	server := grpcgo.NewServer()
	server.RegisterService(&localAIBackendServiceDesc, backend)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(time.Second):
			t.Errorf("controlled LocalAI gRPC server did not stop")
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	modelPath := filepath.Join(t.TempDir(), "fixture.gguf")
	negotiator := NewPinnedGRPCHostProtocolNegotiator(platformgrpc.NetworkDialer{})
	resultCh := make(chan struct {
		result modelseffects.HostProtocolNegotiationResult
		err    error
	}, 1)
	go func() {
		result, negotiateErr := negotiator.Negotiate(
			ctx,
			listener.Addr().String(),
			modelseffects.HostProtocolNegotiationRequest{Configuration: modelseffects.ResolvedHostConfiguration{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         "localai-llamacpp",
				ModelName:       models.BuiltInModelNameEmbed,
				ModelPath:       modelPath,
			}},
		)
		resultCh <- struct {
			result modelseffects.HostProtocolNegotiationResult
			err    error
		}{result: result, err: negotiateErr}
	}()

	healthAt := awaitWitnessSignal(t, backend.health, resultCh, "Health")
	loadModelAt := awaitWitnessSignal(t, backend.loadModel, resultCh, "LoadModel")

	// This timer is the controlled boundary gate: elapsed wall time is the
	// behavior under test, so no event-driven substitute can establish that the
	// LoadModel RPC remained active beyond the readiness budget.
	budgetTimer := time.NewTimer(readinessWitnessBudget)
	select {
	case outcome := <-resultCh:
		budgetTimer.Stop()
		t.Fatalf("Negotiate returned before the 30-second readiness budget while LoadModel is blocked: result=%#v error=%v", outcome.result, outcome.err)
	case <-budgetTimer.C:
	}
	if elapsed := time.Since(loadModelAt); elapsed < readinessWitnessBudget {
		t.Fatalf("blocked LoadModel age at readiness-budget observation = %s, want at least %s", elapsed, readinessWitnessBudget)
	}
	stillBlockedTimer := time.NewTimer(time.Second)
	select {
	case outcome := <-resultCh:
		stillBlockedTimer.Stop()
		t.Fatalf("Negotiate returned during the measured readiness overrun: result=%#v error=%v", outcome.result, outcome.err)
	case <-stillBlockedTimer.C:
	}
	callerCanceledAt := time.Now()
	cancel()
	var outcome struct {
		result modelseffects.HostProtocolNegotiationResult
		err    error
	}
	select {
	case outcome = <-resultCh:
	case <-time.After(3 * time.Second):
		t.Fatal("LoadModel gRPC call did not return after caller cancellation")
	}
	serverCanceledAt := awaitWitnessSignal(t, backend.cancelled, resultCh, "LoadModel cancellation")
	if outcome.result.Ready || !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("Negotiate after caller cancellation = result %#v, error %v; want not-ready/context.Canceled", outcome.result, outcome.err)
	}
	if !healthAt.Before(loadModelAt) || !loadModelAt.Before(callerCanceledAt) || serverCanceledAt.Before(callerCanceledAt) {
		t.Fatalf("witness phase order is invalid: health=%s load_model=%s caller_cancel=%s server_cancel=%s", healthAt, loadModelAt, callerCanceledAt, serverCanceledAt)
	}
	observedOverrun := callerCanceledAt.Sub(loadModelAt) - readinessWitnessBudget
	t.Logf("LOCALAI-READINESS-WITNESS endpoint=%s health=success_at:%s load_model=blocked_at:%s rpc_active_after_budget=true overrun_after_load_start=%s caller_cancel=observed_at:%s negotiator_return=typed_context_canceled server_cancel=observed_at:%s ready=false", listener.Addr(), healthAt.UTC().Format(time.RFC3339Nano), loadModelAt.UTC().Format(time.RFC3339Nano), observedOverrun, callerCanceledAt.UTC().Format(time.RFC3339Nano), serverCanceledAt.UTC().Format(time.RFC3339Nano))
}

func awaitWitnessSignal[T any](
	t *testing.T,
	signal <-chan T,
	result <-chan struct {
		result modelseffects.HostProtocolNegotiationResult
		err    error
	},
	phase string,
) T {
	t.Helper()
	select {
	case observed := <-signal:
		return observed
	case outcome := <-result:
		t.Fatalf("Negotiate returned before %s was observed: result=%#v error=%v", phase, outcome.result, outcome.err)
		var zero T
		return zero
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s from controlled LocalAI peer", phase)
		var zero T
		return zero
	}
}

type blockedLoadModelBackend struct {
	health    chan time.Time
	loadModel chan time.Time
	cancelled chan time.Time
}

func (backend *blockedLoadModelBackend) Health(context.Context, *HealthMessage) (*Reply, error) {
	backend.health <- time.Now()
	return &Reply{}, nil
}

func (backend *blockedLoadModelBackend) LoadModel(ctx context.Context, _ *ModelOptions) (*Result, error) {
	backend.loadModel <- time.Now()
	<-ctx.Done()
	backend.cancelled <- time.Now()
	return nil, status.Error(codes.Canceled, "controlled LoadModel cancellation")
}

func (*blockedLoadModelBackend) Predict(context.Context, *PredictOptions) (*Reply, error) {
	return nil, status.Error(codes.Unimplemented, "not used by readiness witness")
}
