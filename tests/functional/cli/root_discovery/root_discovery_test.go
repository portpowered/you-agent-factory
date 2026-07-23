package root_discovery_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestBareRootPrintsConciseHelpWithoutProductEffects(t *testing.T) {
	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactoryDefinitionLoadingFileSystem:  countingFactoryFileSystem{calls: &effects},
		FactoryDefinitionScaffoldFileSystem: countingFactoryFileSystem{calls: &effects},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if effects.Load() != 0 {
		t.Fatalf("BuildProcess() external effect calls = %d, want 0", effects.Load())
	}

	redirected := executeBareRoot(t, process, false)
	terminal := executeBareRoot(t, process, true)
	if redirected != terminal {
		t.Fatalf("terminal and redirected help differ:\nterminal:\n%s\nredirected:\n%s", terminal, redirected)
	}
	if effects.Load() != 0 {
		t.Fatalf("bare root external effect calls = %d, want 0", effects.Load())
	}
}

func executeBareRoot(t *testing.T, process interface{ Execute(root.Input) error }, stdoutIsTTY bool) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := false
	home := t.TempDir()
	err := process.Execute(root.Input{
		Args:             []string{"you"},
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: t.TempDir(),
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf("Process.Execute(bare root) error = %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Process.Execute(bare root) stderr = %q, want empty", stderr.String())
	}
	for _, expected := range []string{
		"Run and manage CPN-based workflow factories",
		"Available Commands:",
		"run",
		"server",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("Process.Execute(bare root) stdout omitted %q:\n%s", expected, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "How to use:") {
		t.Fatalf("Process.Execute(bare root) emitted long-form help:\n%s", stdout.String())
	}
	return stdout.String()
}

type countingFactoryFileSystem struct {
	calls *atomic.Int32
}

func (fileSystem countingFactoryFileSystem) Stat(string) (fs.FileInfo, error) {
	fileSystem.calls.Add(1)
	return nil, fs.ErrNotExist
}

func (fileSystem countingFactoryFileSystem) ReadFile(string) ([]byte, error) {
	fileSystem.calls.Add(1)
	return nil, fs.ErrNotExist
}

func (fileSystem countingFactoryFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return nil
}

func (fileSystem countingFactoryFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return nil
}

type countingProvider struct {
	calls *atomic.Int32
}

var _ providercontract.Provider = countingProvider{}

func (provider countingProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	provider.calls.Add(1)
	return workers.InferenceResponse{}, nil
}
