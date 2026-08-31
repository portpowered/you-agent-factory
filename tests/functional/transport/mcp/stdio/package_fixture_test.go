package stdio_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// mcpStdioRootFixture owns immutable application wiring for the package. Each
// Process.Execute call remains serialized because the MCP stdio boundary owns
// its streams and invocation lifecycle, while the root itself is reusable.
type mcpStdioRootFixture struct {
	buildMu   sync.Mutex
	executeMu sync.Mutex
	process   support.ApplicationProcess
	buildErr  error
	closeOnce sync.Once
	closeErr  error
}

var sharedMCPStdioRoot mcpStdioRootFixture

func (fixture *mcpStdioRootFixture) ensure(t testing.TB) support.ApplicationProcess {
	t.Helper()

	fixture.buildMu.Lock()
	defer fixture.buildMu.Unlock()
	if fixture.process == nil && fixture.buildErr == nil {
		fixture.process, fixture.buildErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
		if fixture.buildErr == nil {
			mcpStdioTopology.recordRootBuild()
		}
	}
	if fixture.buildErr != nil {
		t.Fatalf("BuildProcess() for shared MCP stdio root: %v", fixture.buildErr)
	}
	return fixture.process
}

func (fixture *mcpStdioRootFixture) execute(
	process support.ApplicationProcess,
	input root.Input,
) error {
	fixture.executeMu.Lock()
	defer fixture.executeMu.Unlock()
	mcpStdioTopology.recordInvocationStarted()
	defer mcpStdioTopology.recordInvocationReturned()
	return process.Execute(input)
}

func closeSharedMCPStdioRoot() error {
	sharedMCPStdioRoot.buildMu.Lock()
	process := sharedMCPStdioRoot.process
	sharedMCPStdioRoot.buildMu.Unlock()
	if process == nil {
		return nil
	}

	sharedMCPStdioRoot.closeOnce.Do(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), mcpStdioStopTimeout)
		defer cancel()
		sharedMCPStdioRoot.closeErr = process.Close(closeContext)
		mcpStdioTopology.recordRootClose()
	})
	return sharedMCPStdioRoot.closeErr
}

func executeSharedMCPStdioProcess(
	process support.ApplicationProcess,
	input root.Input,
) error {
	if process == nil {
		return fmt.Errorf("execute MCP stdio process requires an application process")
	}
	return sharedMCPStdioRoot.execute(process, input)
}
