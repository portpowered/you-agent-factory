package wire

import (
	"context"
	"reflect"
	"testing"
	"time"

	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
)

func TestAdaptPTYAllocatorProjectsForeignOwnerShape(t *testing.T) {
	t.Parallel()
	candidate := foreignPTYFixture()
	allocator := requirePTYAllocator(t, candidate)
	session := allocateForeignPTY(t, allocator)
	assertForeignPTYLaunch(t, candidate)
	assertForeignPTYConfig(t, candidate)
	assertForeignPTYResult(t, session)
	closeForeignPTY(t, candidate, session)
}

func foreignPTYFixture() *foreignPTYAllocator {
	return &foreignPTYAllocator{
		session: &foreignPTYSession{
			result: foreignPTYSessionResult{
				ExitCode:    7,
				RawBytes:    []byte("raw"),
				CleanedText: "clean",
				TimedOut:    true,
				CapacityHit: true,
			},
		},
	}
}

func requirePTYAllocator(t *testing.T, candidate *foreignPTYAllocator) workersinternal.PTYAllocator {
	t.Helper()
	allocator := adaptPTYAllocator(candidate)
	if allocator == nil {
		t.Fatal("adaptPTYAllocator() = nil, want structural adapter")
	}
	return allocator
}

func allocateForeignPTY(t *testing.T, allocator workersinternal.PTYAllocator) workersinternal.PTYSession {
	t.Helper()
	session, err := allocator.Allocate(nil, workersinternal.PTYProcessLaunch{
		Executable: "agy",
		Argv:       []string{"--headless"},
		WorkDir:    "factory",
		Env:        []string{"MODE=test"},
	}, workersinternal.PTYSessionConfig{
		MaxCaptureBytes: 12,
		IdleTimeout:     time.Second,
		HardTimeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	return session
}

func assertForeignPTYLaunch(t *testing.T, candidate *foreignPTYAllocator) {
	t.Helper()
	want := foreignPTYProcessLaunch{
		Executable: "agy", Argv: []string{"--headless"}, WorkDir: "factory", Env: []string{"MODE=test"},
	}
	if !reflect.DeepEqual(candidate.launch, want) {
		t.Fatalf("foreign launch = %#v, want %#v", candidate.launch, want)
	}
}

func assertForeignPTYConfig(t *testing.T, candidate *foreignPTYAllocator) {
	t.Helper()
	want := foreignPTYSessionConfig{MaxCaptureBytes: 12, IdleTimeout: time.Second, HardTimeout: 2 * time.Second}
	if !reflect.DeepEqual(candidate.config, want) {
		t.Fatalf("foreign config = %#v, want %#v", candidate.config, want)
	}
}

func assertForeignPTYResult(t *testing.T, session workersinternal.PTYSession) {
	t.Helper()
	result, err := session.Run(nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := workersinternal.PTYSessionResult{
		ExitCode: 7, RawBytes: []byte("raw"), CleanedText: "clean", TimedOut: true, CapacityHit: true,
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("session result = %#v, want %#v", result, want)
	}
}

func closeForeignPTY(t *testing.T, candidate *foreignPTYAllocator, session workersinternal.PTYSession) {
	t.Helper()
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !candidate.session.closed {
		t.Fatal("foreign session was not closed")
	}
}

type foreignPTYAllocator struct {
	launch  foreignPTYProcessLaunch
	config  foreignPTYSessionConfig
	session *foreignPTYSession
}

func (allocator *foreignPTYAllocator) Allocate(
	_ context.Context,
	launch foreignPTYProcessLaunch,
	config foreignPTYSessionConfig,
) (*foreignPTYSession, error) {
	allocator.launch = launch
	allocator.config = config
	return allocator.session, nil
}

type foreignPTYProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

type foreignPTYSessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

type foreignPTYSession struct {
	result foreignPTYSessionResult
	closed bool
}

func (session *foreignPTYSession) Run(context.Context) (foreignPTYSessionResult, error) {
	return session.result, nil
}

func (session *foreignPTYSession) Close() error {
	session.closed = true
	return nil
}

type foreignPTYSessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}
