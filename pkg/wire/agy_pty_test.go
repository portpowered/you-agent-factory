package wire

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type recordingAgyPTYHost struct{ allocated bool }
type recordingAgyPTY struct{}

func (*recordingAgyPTY) Close() error           { return nil }
func (*recordingAgyPTY) Kind() platformpty.Kind { return platformpty.KindPOSIX }
func (h *recordingAgyPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	h.allocated = true
	return &recordingAgyPTY{}, nil
}
func (*recordingAgyPTYHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, nil
}

func TestEdgesDoNotExposeComposedAgyPTYAllocator(t *testing.T) {
	t.Parallel()

	if _, exposed := reflect.TypeOf(serviceedges.Edges{}).FieldByName("AgyPTYAllocator"); exposed {
		t.Fatal("Edges exposes composed AgyPTYAllocator; only Host and Clock effects may be replaced")
	}
}

func TestProvideAgyPTYAllocatorSelectsInertPlatformAdapter(t *testing.T) {
	t.Parallel()

	got, err := provideAgyPTYAllocator(serviceedges.Edges{
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	})
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	if got == nil {
		t.Fatal("provideAgyPTYAllocator() = nil, want inert platform adapter")
	}
}

func TestProvideAgyPTYAllocatorPreservesInjectedNativeHost(t *testing.T) {
	t.Parallel()

	host := &recordingAgyPTYHost{}
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{
		AgyPTYHost:  host,
		AgyPTYClock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := allocator.Allocate(context.Background(), workers.PTYProcessLaunch{
		Executable: "agy", Argv: []string{"agy"},
	}, workers.DefaultPTYSessionConfig())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	defer session.Close()
	if !host.allocated {
		t.Fatal("injected native host was not used")
	}
}
