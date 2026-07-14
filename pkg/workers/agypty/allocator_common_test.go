package agypty

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPTYKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind PTYKind
		want string
	}{
		{PTYKindPOSIX, "posix"},
		{PTYKindConPTY, "conpty"},
		{PTYKindUnknown, "unknown"},
		{PTYKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Fatalf("PTYKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestPlatformSessionPTYKind(t *testing.T) {
	t.Parallel()

	var nilSession *platformSession
	if got := nilSession.PTYKind(); got != PTYKindUnknown {
		t.Fatalf("nil PTYKind() = %v, want %v", got, PTYKindUnknown)
	}

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindConPTY, closeOnlyPTY{})
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}
	if got := session.PTYKind(); got != PTYKindConPTY {
		t.Fatalf("PTYKind() = %v, want %v", got, PTYKindConPTY)
	}
}

func TestNewPlatformSession_RejectsNilPTY(t *testing.T) {
	t.Parallel()

	_, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindPOSIX, nil)
	if err == nil {
		t.Fatal("newPlatformSession() error = nil, want error")
	}
}

func TestValidateProcessLaunch_RejectsEmptyArgv(t *testing.T) {
	t.Parallel()

	err := validateProcessLaunch(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       nil,
	})
	if err == nil || !strings.Contains(err.Error(), "argv is required") {
		t.Fatalf("validateProcessLaunch() error = %v, want argv required error", err)
	}
}

func TestWrapPTYAllocationFailure(t *testing.T) {
	t.Parallel()

	if err := wrapPTYAllocationFailure(nil); err != nil {
		t.Fatalf("wrapPTYAllocationFailure(nil) = %v, want nil", err)
	}

	want := errors.New("openpty failed")
	err := wrapPTYAllocationFailure(want)
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("wrapPTYAllocationFailure() error = %v, want %v", err, ErrPTYAllocationFailed)
	}
}

func TestCheckAllocateContext_AllowsNilContext(t *testing.T) {
	t.Parallel()

	if err := checkAllocateContext(nil); err != nil {
		t.Fatalf("checkAllocateContext(nil) error = %v, want nil", err)
	}
}

func TestValidateArgv_RejectsEmptyExecutable(t *testing.T) {
	t.Parallel()

	if err := ValidateArgv([]string{"  "}); err == nil {
		t.Fatal("ValidateArgv() error = nil, want error")
	}
}

func TestRejectShellWrapper_AllowsNonShellPrograms(t *testing.T) {
	t.Parallel()

	if err := RejectShellWrapper([]string{"/bin/agy", "chat", "hello"}); err != nil {
		t.Fatalf("RejectShellWrapper() error = %v, want nil", err)
	}
}

func TestContainsTerminalEscapeOrControl_DetectsEscapeAndControlBytes(t *testing.T) {
	t.Parallel()

	if !ContainsTerminalEscapeOrControl("\x1b[31m") {
		t.Fatal("ContainsTerminalEscapeOrControl() = false, want true for ESC")
	}
	if !ContainsTerminalEscapeOrControl("ok\x07") {
		t.Fatal("ContainsTerminalEscapeOrControl() = false, want true for BEL")
	}
	if ContainsTerminalEscapeOrControl("plain\nanswer") {
		t.Fatal("ContainsTerminalEscapeOrControl() = true, want false for clean text")
	}
}

func TestMockAllocator_PropagatesAllocateError(t *testing.T) {
	t.Parallel()

	want := errors.New("allocate failed")
	allocator := &MockAllocator{Err: want}
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig())
	if !errors.Is(err, want) {
		t.Fatalf("Allocate() error = %v, want %v", err, want)
	}
}

func TestMockSession_PropagatesRunError(t *testing.T) {
	t.Parallel()

	want := errors.New("run failed")
	session := &MockSession{RunErr: want}
	_, err := session.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestSessionProcessNilGuards(t *testing.T) {
	t.Parallel()

	var proc *sessionProcess
	if err := proc.Terminate(); err != nil {
		t.Fatalf("Terminate(nil) error = %v, want nil", err)
	}
	proc.Close()
	if proc.PID() != 0 {
		t.Fatalf("PID(nil) = %d, want 0", proc.PID())
	}
}

func TestExitCodeFromWait(t *testing.T) {
	t.Parallel()

	if got := exitCodeFromWait(errors.New("wait failed"), nil); got != -1 {
		t.Fatalf("exitCodeFromWait(waitErr) = %d, want -1", got)
	}
	if got := exitCodeFromWait(nil, &sessionProcess{exitCode: 2}); got != 2 {
		t.Fatalf("exitCodeFromWait(proc.exitCode=2) = %d, want 2", got)
	}
	if got := exitCodeFromWait(nil, nil); got != 0 {
		t.Fatalf("exitCodeFromWait(nil,nil) = %d, want 0", got)
	}
}

func TestPlatformSessionRun_RejectsClosedSession(t *testing.T) {
	t.Parallel()

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{})
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := session.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want closed-session error")
	}
}

func TestExecuteSessionRun_RejectsMissingInputs(t *testing.T) {
	t.Parallel()

	cfg := DefaultSessionConfig()
	proc := &sessionProcess{}

	if _, err := executeSessionRun(context.Background(), cfg, nil, proc); err == nil {
		t.Fatal("executeSessionRun(nil reader) error = nil, want error")
	}

	reader := io.NopCloser(strings.NewReader(""))
	if _, err := executeSessionRun(context.Background(), cfg, reader, nil); err == nil {
		t.Fatal("executeSessionRun(nil proc) error = nil, want error")
	}
}

func TestClosePTYReader_AllowsNil(t *testing.T) {
	t.Parallel()

	closePTYReader(nil)
}
