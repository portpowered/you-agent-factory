package execution_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFSCP02DetachedCanonicalProcessBoundary proves the process-owned detached
// capability is reachable through the same root.BuildProcess/Process.Execute
// construction used by customers. It records the excluded assembly boundary
// for valid live work while proving field-scoped canonical validation.
func TestFSCP02DetachedCanonicalProcessBoundary(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	factoryDir := support.ScaffoldSingleStepFactory(t, "fscp02-detached-live")
	home := t.TempDir()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})
	inputs.Input.Env = isolatedEnvironment(home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}

	capability := process.DetachedOperations()
	if capability == nil {
		t.Fatal("root process returned no detached capability")
	}
	detached, ok := capability.DetachedOperations().(factorysessions.DetachedService)
	if !ok || detached == nil {
		t.Fatalf("detached capability type = %T, want factorysessions.DetachedService", capability.DetachedOperations())
	}

	_, err = detached.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: factoryDir,
	})
	if err == nil || err.Error() != "Factory Sessions gateway is required" {
		t.Fatalf("detached canonical Start(live) error = %v, want excluded assembly boundary", err)
	}
	t.Logf("FSCP-02 F1/F2/F3/F4/F5/F6/F7/F8/F9/F10/F11/F12/F14 INCONCLUSIVE: root-built detached live start reached excluded assembly boundary: %v", err)

	_, err = detached.Start(t.Context(), factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationMode("invalid")})
	var requestErr *factorysessions.DetachedRequestError
	if !errors.As(err, &requestErr) || requestErr.Field != "mode" {
		t.Fatalf("detached canonical invalid Start() error = %v, want mode-scoped DetachedRequestError", err)
	}
	_, err = detached.Get(t.Context(), factorysessions.SessionGetRequest{})
	requestErr = nil
	if !errors.As(err, &requestErr) || requestErr.Field != "sessionId" {
		t.Fatalf("detached canonical invalid Get() error = %v, want sessionId-scoped DetachedRequestError", err)
	}
	t.Log("FSCP-02 F13 PASS: detached invalid requests returned canonical field-scoped errors before the excluded live gateway")
}

func isolatedEnvironment(home string) []string {
	state := filepath.Join(home, "state")
	cache := filepath.Join(home, "cache")
	return append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"APPDATA="+filepath.Join(home, "appdata"),
		"LOCALAPPDATA="+filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_DATA_HOME="+state,
		"XDG_CACHE_HOME="+cache,
	)
}
