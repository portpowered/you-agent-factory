package binding_test

import (
	"context"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/binding"
)

func TestRequireRejectsNilAndTypedNilRoots(t *testing.T) {
	t.Parallel()

	for name, root := range map[string]binding.Root{
		"nil":       nil,
		"typed nil": (*typedNilRoot)(nil),
	} {
		name, root := name, root
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got, err := binding.New(root).Require(); got != nil || err == nil {
				t.Fatalf("Require() = (%v, %v), want (nil, error)", got, err)
			}
		})
	}
}

func TestRequireAcceptsNonPointerRoot(t *testing.T) {
	t.Parallel()

	root, err := binding.New(valueRoot{}).Require()
	if err != nil {
		t.Fatalf("Require() error = %v, want nil", err)
	}
	if root == nil {
		t.Fatal("Require() root = nil, want bound root")
	}
}

func TestOperationsRejectUnavailableRoot(t *testing.T) {
	t.Parallel()

	handler := binding.New(nil)
	operations := []struct {
		name string
		call func() error
	}{
		{
			name: "Activate",
			call: func() error {
				_, err := handler.Activate(context.Background(), factoryvisualization.ActivateRequest{})
				return err
			},
		},
		{
			name: "Join",
			call: func() error {
				_, err := handler.Join(context.Background(), factoryvisualization.JoinRequest{})
				return err
			},
		},
		{
			name: "StopDrain",
			call: func() error {
				_, err := handler.StopDrain(context.Background(), factoryvisualization.StopDrainRequest{})
				return err
			},
		},
		{
			name: "Observe",
			call: func() error {
				_, err := handler.Observe(context.Background(), factoryvisualization.ObserveRequest{})
				return err
			},
		},
		{
			name: "OpenPresentation",
			call: func() error {
				_, err := handler.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{})
				return err
			},
		},
		{
			name: "PresentProgress",
			call: func() error {
				_, err := handler.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{})
				return err
			},
		},
		{
			name: "FinalizePresentation",
			call: func() error {
				_, err := handler.FinalizePresentation(context.Background(), factoryvisualization.FinalizePresentationRequest{})
				return err
			},
		},
		{
			name: "ClosePresentation",
			call: func() error {
				_, err := handler.ClosePresentation(context.Background(), factoryvisualization.ClosePresentationRequest{})
				return err
			},
		},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			if err := operation.call(); err == nil {
				t.Fatal("operation error = nil, want unavailable-root error")
			}
		})
	}
}

type typedNilRoot struct {
	factoryvisualization.Root
}

var _ factoryvisualization.Root = (*typedNilRoot)(nil)

type valueRoot struct {
	factoryvisualization.Root
}

var _ factoryvisualization.Root = valueRoot{}
