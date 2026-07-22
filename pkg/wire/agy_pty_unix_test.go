//go:build linux || darwin

package wire

import (
	"context"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
)

func TestPOSIXPTYSessionRun_CapturesCleanedOutput(t *testing.T) {
	result, session := runWiredPOSIXPTY(t, []string{"/bin/sh", "-c", "printf '\\033[31magy-pty\\033[0m'"}, agypty.DefaultSessionConfig())
	defer session.Close()
	if result.CleanedText != "agy-pty" {
		t.Fatalf("CleanedText = %q", result.CleanedText)
	}
}

func TestPOSIXPTYSessionRun_IdleTimeoutTerminatesProcess(t *testing.T) {
	cfg := agypty.DefaultSessionConfig()
	cfg.IdleTimeout, cfg.HardTimeout = 100*time.Millisecond, time.Hour
	result, session := runWiredPOSIXPTY(t, []string{"/bin/sleep", "120"}, cfg)
	defer session.Close()
	if !result.TimedOut {
		t.Fatal("TimedOut = false")
	}
}

func TestPOSIXPTYSessionRun_ClosesPTYAfterRun(t *testing.T) {
	_, session := runWiredPOSIXPTY(t, []string{"/bin/echo", "close"}, agypty.DefaultSessionConfig())
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func runWiredPOSIXPTY(t *testing.T, argv []string, cfg agypty.SessionConfig) (agypty.SessionResult, agypty.PTYSession) {
	t.Helper()
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{AgyPTYClock: platformclock.Real{}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := allocator.Allocate(context.Background(), agypty.ProcessLaunch{Executable: argv[0], Argv: argv}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result, session
}
