package replay_contracts

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

type replayContractHarness struct {
	t            *testing.T
	ArtifactPath string
	Artifact     *interfaces.ReplayArtifact
	Service      *testutil.ServiceTestHarness
}

func newReplayContractHarness(
	t *testing.T,
	artifactPath string,
	dir string,
	opts ...testutil.ServiceTestHarnessOption,
) *replayContractHarness {
	t.Helper()

	if dir == "" {
		dir = t.TempDir()
	}
	serviceOptions := []testutil.ServiceTestHarnessOption{
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithReplayPath(artifactPath),
	}
	serviceOptions = append(serviceOptions, opts...)

	return &replayContractHarness{
		t:            t,
		ArtifactPath: artifactPath,
		Artifact:     testutil.LoadReplayArtifact(t, artifactPath),
		Service:      testutil.NewServiceTestHarness(t, dir, serviceOptions...),
	}
}

func (h *replayContractHarness) RunUntilComplete(timeout time.Duration) error {
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

func assertReplaySucceedsWithCustomizedHarness(
	t *testing.T,
	artifactPath string,
	timeout time.Duration,
	dir string,
	opts ...testutil.ServiceTestHarnessOption,
) *replayContractHarness {
	t.Helper()

	h := newReplayContractHarness(t, artifactPath, dir, opts...)
	if err := h.RunUntilComplete(timeout); err != nil {
		t.Fatalf("assertReplaySucceedsWithCustomizedHarness: %v", err)
	}
	return h
}
