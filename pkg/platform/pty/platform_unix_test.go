//go:build linux || darwin

package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPOSIXHost_AllocateWithMockOpener(t *testing.T) {
	t.Parallel()
	masterRead, masterWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	host := &POSIXHost{Open: func() (*os.File, *os.File, error) { return masterRead, masterWrite, nil }}
	native, err := host.Allocate(context.Background())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	defer native.Close()
	if native.Kind() != KindPOSIX {
		t.Fatalf("Kind() = %v", native.Kind())
	}
}

func TestPOSIXHost_AllocateOpensPTY(t *testing.T) {
	native, err := NewHost().Allocate(context.Background())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	defer native.Close()
	allocation := native.(*posixPTYAllocation)
	names := strings.ToLower(allocation.Master().Name() + " " + allocation.Slave().Name())
	if !strings.Contains(names, "pt") {
		t.Fatalf("PTY names = %q", names)
	}
}

func TestPOSIXHost_PropagatesOpenFailure(t *testing.T) {
	want := errors.New("open failed")
	host := &POSIXHost{Open: func() (*os.File, *os.File, error) { return nil, nil, want }}
	_, err := host.Allocate(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Allocate() error = %v, want %v", err, want)
	}
}

func TestPOSIXHost_StartCapturesRawOutput(t *testing.T) {
	raw := runPOSIXHost(t, []string{"/bin/echo", "agy-pty"})
	if !strings.Contains(string(raw), "agy-pty") {
		t.Fatalf("raw output = %q", raw)
	}
}

func TestPOSIXHost_TerminatesAttachedProcess(t *testing.T) {
	host := NewHost()
	native, err := host.Allocate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	proc, reader, err := host.Start(ProcessLaunch{Executable: "/bin/sleep", Argv: []string{"/bin/sleep", "120"}}, native)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer proc.Close()
	if err := proc.Terminate(); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	_ = proc.Wait()
}

func TestPOSIXHost_ClosesPTYAfterProcess(t *testing.T) {
	_ = runPOSIXHost(t, []string{"/bin/echo", "close"})
}

func runPOSIXHost(t *testing.T, argv []string) []byte {
	t.Helper()
	host := NewHost()
	native, err := host.Allocate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	proc, reader, err := host.Start(ProcessLaunch{Executable: argv[0], Argv: argv}, native)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	raw, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if waitErr := proc.Wait(); waitErr != nil {
		t.Fatalf("Wait() error = %v", waitErr)
	}
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	return raw
}
