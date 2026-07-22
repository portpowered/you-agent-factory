package agypty

import (
	"context"
	"errors"
	"io"
	"testing"

	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
)

type failingHost struct {
	pty platformpty.Allocation
	err error
}

func (h failingHost) Allocate(context.Context) (platformpty.Allocation, error) { return h.pty, h.err }
func (failingHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, errors.New("unexpected Start")
}

func TestAllocator_PreservesUnsupportedHostFailure(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{err: platformpty.ErrUnsupportedPlatform}, testPTYClock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Allocate() error = %v", err)
	}
}

func TestAllocator_WrapsNativeAllocationFailure(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{err: errors.New("native failure")}, testPTYClock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v", err)
	}
}

func TestAllocator_RejectsNilNativeAllocation(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{}, testPTYClock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v", err)
	}
}
