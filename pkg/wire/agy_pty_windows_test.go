//go:build windows

package wire

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestConPTYSessionRun_CompletesChildProcess(t *testing.T) {
	result, session := runWiredConPTY(t, []string{`C:\Windows\System32\cmd.exe`, "/c", "echo", "agy-pty"}, workers.DefaultPTYSessionConfig())
	defer session.Close()
	if !strings.Contains(result.CleanedText, "agy-pty") {
		t.Fatalf("CleanedText = %q", result.CleanedText)
	}
}

func TestConPTYSessionRun_HardTimeoutTerminatesProcess(t *testing.T) {
	cfg := workers.DefaultPTYSessionConfig()
	cfg.HardTimeout, cfg.IdleTimeout = 100*time.Millisecond, time.Hour
	result, session := runWiredConPTY(t, []string{`C:\Windows\System32\ping.exe`, "-n", "120", "127.0.0.1"}, cfg)
	defer session.Close()
	if !result.TimedOut {
		t.Fatal("TimedOut = false")
	}
}

func TestConPTYSessionRun_ClosesPTYAfterRun(t *testing.T) {
	_, session := runWiredConPTY(t, []string{`C:\Windows\System32\cmd.exe`, "/c", "echo", "close"}, workers.DefaultPTYSessionConfig())
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func runWiredConPTY(t *testing.T, argv []string, cfg workers.PTYSessionConfig) (workers.PTYSessionResult, workers.PTYSession) {
	t.Helper()
	if _, err := os.Stat(argv[0]); err != nil {
		t.Skipf("executable unavailable: %v", err)
	}
	allocator, err := provideAgyPTYAllocator(serviceedges.Edges{AgyPTYClock: platformclock.Real{}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := allocator.Allocate(context.Background(), workers.PTYProcessLaunch{Executable: argv[0], Argv: argv}, cfg)
	if err != nil {
		t.Skipf("ConPTY unavailable: %v", err)
	}
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result, session
}
