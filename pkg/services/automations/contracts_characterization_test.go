package automations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// fakeRootService is a peer-shaped Automations Service that depends only on
// Automations root contracts. It proves the singular root seam is implementable
// without Automations implementation packages or cron/poller/watcher types.
type fakeRootService struct {
	ready bool
}

func (f *fakeRootService) Ready(context.Context, automations.ReadyRequest) (automations.ReadyResult, error) {
	if f == nil || !f.ready {
		return automations.ReadyResult{}, &automations.Error{
			Op:   "Ready",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	return automations.ReadyResult{Ready: true}, nil
}

func TestServiceRootSeam_FakeImplementsPlainContracts(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	result, err := svc.Ready(context.Background(), automations.ReadyRequest{})
	if err != nil {
		t.Fatalf("Ready() unexpected error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("Ready() result.Ready = false, want true")
	}
}

func TestServiceRootSeam_FakeTypedNotReady(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: false}
	_, err := svc.Ready(context.Background(), automations.ReadyRequest{})
	if err == nil {
		t.Fatal("Ready() error = nil, want typed not-ready error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("Ready() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeNotReady {
		t.Fatalf("Ready() error code = %q, want %q", typed.Code, automations.ErrorCodeNotReady)
	}
	if !errors.Is(err, automations.ErrNotReady) {
		t.Fatalf("Ready() error = %v, want errors.Is ErrNotReady", err)
	}
}
