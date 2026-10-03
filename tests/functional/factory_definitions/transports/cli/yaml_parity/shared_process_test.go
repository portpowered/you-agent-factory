package yaml_parity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var yamlParityCLIProcess support.ApplicationProcess
var yamlParityCommands = &sourceCommandObserver{calls: make(map[string]int)}

// Each invocation owns its working directory, so rejection observations remain
// attributable while independent source cases run concurrently.
type sourceCommandObserver struct {
	mu    sync.Mutex
	calls map[string]int
}

func (r *sourceCommandObserver) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls[filepath.Clean(req.WorkDir)]++
	r.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider dispatch for rejected authored source: %s", req.WorkDir)
}

func (r *sourceCommandObserver) callCount(directory string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[filepath.Clean(directory)]
}

func TestMain(m *testing.M) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{ProviderCommandRunner: yamlParityCommands})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build YAML parity CLI process: %v\n", err)
		os.Exit(1)
	}
	yamlParityCLIProcess = process
	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close YAML parity CLI process: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}
