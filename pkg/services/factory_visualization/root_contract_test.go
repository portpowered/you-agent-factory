package factory_visualization_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// fakeRootPeer is a peer-shaped Root implementer that uses only the published
// Factory Visualization root package. It stays inert until Activate/Start.
type fakeRootPeer struct {
	started bool
	waitErr error
}

func (f *fakeRootPeer) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("start Factory visualization: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.started {
		return errors.New("start Factory visualization: already started")
	}
	f.started = true
	return nil
}

func (f *fakeRootPeer) Stop(context.Context) error {
	f.started = false
	return nil
}

func (f *fakeRootPeer) Wait(context.Context) error {
	if !f.started {
		if f.waitErr != nil {
			return f.waitErr
		}
		return errors.New("wait for Factory visualization: not started")
	}
	return nil
}

func TestRootContractFakePeerInertNotActivated(t *testing.T) {
	t.Parallel()

	peer := &fakeRootPeer{}
	var root factoryvisualization.Root = peer

	err := root.Wait(context.Background())
	if err == nil {
		t.Fatal("Wait on inert Root: error = nil, want not-started failure")
	}
	if !strings.Contains(err.Error(), "not started") {
		t.Fatalf("Wait on inert Root: error = %v, want not-started vocabulary", err)
	}
	if peer.started {
		t.Fatal("inert Root peer must not mark itself started after Wait")
	}
}

func TestConcreteServiceImplementsRoot(t *testing.T) {
	t.Parallel()

	// Compile-time reachability: existing lifecycle Service remains the Root
	// implementer so activation stays on the singular peer-facing seam.
	var _ factoryvisualization.Root = (*factoryvisualization.Service)(nil)
}
