package testutil

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

// ReplayHarness runs a replay artifact through the production-style service
// path. It loads embedded config from the artifact, installs replay side
// effects through service replay mode, and uses the worker-pool dispatch path
// by default.
type ReplayHarness struct {
	t            *testing.T
	ArtifactPath string
	Artifact     *interfaces.ReplayArtifact
	Service      *ServiceTestHarness
}

// ReplayHarnessOption configures a ReplayHarness.
type ReplayHarnessOption func(*replayHarnessConfig)

type replayHarnessConfig struct {
	dir            string
	serviceOptions []ServiceTestHarnessOption
}

// WithReplayHarnessDir overrides the service directory used when constructing
// the replay harness. Use this when replay should validate against a copied
// factory checkout rather than a blank temp directory.
func WithReplayHarnessDir(dir string) ReplayHarnessOption {
	return func(cfg *replayHarnessConfig) {
		cfg.dir = dir
	}
}

// WithReplayHarnessServiceOptions appends service harness options. Use this
// only when a regression needs to exercise a specific service boundary.
func WithReplayHarnessServiceOptions(opts ...ServiceTestHarnessOption) ReplayHarnessOption {
	return func(cfg *replayHarnessConfig) {
		cfg.serviceOptions = append(cfg.serviceOptions, opts...)
	}
}

// LoadReplayArtifact loads and validates a replay artifact fixture for tests.
func LoadReplayArtifact(t *testing.T, artifactPath string) *interfaces.ReplayArtifact {
	t.Helper()

	artifact, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("LoadReplayArtifact: %v", err)
	}
	return artifact
}

// NewReplayHarness builds a replay service harness from an artifact path.
// To promote a customer recording into a regression fixture, commit the replay
// JSON under the relevant testdata directory and pass its path here.
func NewReplayHarness(t *testing.T, artifactPath string, opts ...ReplayHarnessOption) *ReplayHarness {
	t.Helper()

	h, err := BuildReplayHarness(t, artifactPath, opts...)
	if err != nil {
		t.Fatalf("NewReplayHarness: %v", err)
	}
	return h
}

// BuildReplayHarness is the error-returning form used by assertion helpers.
func BuildReplayHarness(t *testing.T, artifactPath string, opts ...ReplayHarnessOption) (*ReplayHarness, error) {
	t.Helper()

	artifact, err := replay.Load(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("load replay artifact: %w", err)
	}

	cfg := &replayHarnessConfig{
		dir: t.TempDir(),
		serviceOptions: []ServiceTestHarnessOption{
			WithFullWorkerPoolAndScriptWrap(),
			WithReplayPath(artifactPath),
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}
	serviceHarness := NewServiceTestHarness(t, cfg.dir, cfg.serviceOptions...)

	return &ReplayHarness{
		t:            t,
		ArtifactPath: artifactPath,
		Artifact:     artifact,
		Service:      serviceHarness,
	}, nil
}

// RunUntilComplete runs replay until terminal state, divergence, or timeout.
func (h *ReplayHarness) RunUntilComplete(timeout time.Duration) error {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := h.Service.RunInBackground(ctx)
	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-h.Service.WaitToComplete():
		cancel()
		err := <-errCh
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-ctx.Done():

		cancel()
		<-errCh
		return fmt.Errorf("replay did not complete within %s: %w", timeout, ctx.Err())
	}
}

// AssertReplaySucceeds runs a replay artifact and fails the test if replay
// diverges, times out, or returns another runtime error.
func AssertReplaySucceeds(t *testing.T, artifactPath string, timeout time.Duration, opts ...ReplayHarnessOption) *ReplayHarness {
	t.Helper()

	h := NewReplayHarness(t, artifactPath, opts...)
	if err := h.RunUntilComplete(timeout); err != nil {
		t.Fatalf("AssertReplaySucceeds: %v", err)
	}
	return h
}

// AssertReplayDiverges runs a replay artifact and returns the structured
// divergence report. It fails the test if replay succeeds or fails for an
// unrelated reason.
func AssertReplayDiverges(t *testing.T, artifactPath string, timeout time.Duration, opts ...ReplayHarnessOption) replay.DivergenceReport {
	t.Helper()

	h := NewReplayHarness(t, artifactPath, opts...)
	err := h.RunUntilComplete(timeout)
	if err == nil {
		t.Fatal("AssertReplayDiverges: replay succeeded, expected divergence")
	}
	var divergence *replay.DivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("AssertReplayDiverges: error = %v, want replay divergence", err)
	}
	return divergence.Report
}
