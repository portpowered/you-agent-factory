//go:build windows

package pty

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestWindowsHost_AllocateWithMockOpener(t *testing.T) {
	t.Parallel()
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	host := &WindowsHost{Open: func() (*conPTYAllocation, error) {
		return &conPTYAllocation{inPipe: inWrite, outPipe: outRead, ptyIn: inRead, ptyOut: outWrite}, nil
	}}
	native, err := host.Allocate(context.Background())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	t.Cleanup(func() { _ = native.Close() })
	if native.Kind() != KindConPTY {
		t.Fatalf("Kind() = %v", native.Kind())
	}
}

func TestWindowsHost_AllocateOpensConPTY(t *testing.T) {
	host := NewHost()
	native, err := host.Allocate(context.Background())
	if err != nil {
		t.Skipf("ConPTY unavailable: %v", err)
	}
	defer native.Close()
	allocation := native.(*conPTYAllocation)
	if allocation.Handle() == 0 || allocation.InputPipe() == nil || allocation.OutputPipe() == nil {
		t.Fatal("ConPTY allocation omitted native handles")
	}
}

func TestWindowsHost_PropagatesOpenFailure(t *testing.T) {
	want := errors.New("open failed")
	host := &WindowsHost{Open: func() (*conPTYAllocation, error) { return nil, want }}
	_, err := host.Allocate(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Allocate() error = %v, want %v", err, want)
	}
}

func TestWindowsHost_StartRejectsForeignPTY(t *testing.T) {
	host := NewHost()
	_, _, err := host.Start(ProcessLaunch{Argv: []string{"agy"}}, foreignPTY{})
	if err == nil {
		t.Fatal("Start() error = nil, want foreign PTY rejected")
	}
}

type foreignPTY struct{}

func (foreignPTY) Kind() Kind   { return KindUnknown }
func (foreignPTY) Close() error { return nil }

func TestWindowsHost_CloseIsIdempotent(t *testing.T) {
	native, err := NewHost().Allocate(context.Background())
	if err != nil {
		t.Skipf("ConPTY unavailable: %v", err)
	}
	if err := native.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := native.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
