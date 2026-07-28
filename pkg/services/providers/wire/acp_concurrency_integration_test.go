package wire_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProviderServiceRunsConcurrentACPAttemptsWithIsolatedSessions(t *testing.T) {
	var starts atomic.Int32
	service, err := providerswire.New(func(name string, args ...string) *exec.Cmd {
		if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestProvidersACPAgentProcess$")
		}
		return exec.Command(name, args...)
	}, integrationExecutableLocator{})
	if err != nil {
		t.Fatalf("construct Providers service: %v", err)
	}

	type result struct {
		response providers.ExecuteResponse
		err      error
	}
	results := make(chan result, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index := 1; index <= 2; index++ {
		index := index
		go func() {
			ready.Done()
			ready.Wait()
			response, executeErr := service.Execute(context.Background(), providers.ExecuteRequest{
				ProviderID: "cursor-acp", Instructions: "concurrent ACP attempt",
				Prompt:           []providers.ContentPart{{Kind: providers.ContentKindText, Text: fmt.Sprintf("attempt %d", index)}},
				WorkingDirectory: t.TempDir(),
				Environment: []providers.EnvironmentEntry{
					{Name: providersACPModeEnvironment, Value: "isolate"},
					{Name: "YOU_TEST_ACP_SESSION_ID", Value: fmt.Sprintf("isolated-session-%d", index)},
				},
			})
			results <- result{response: response, err: executeErr}
		}()
	}

	seen := map[string]bool{}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent ACP execution: %v", got.err)
		}
		if got.response.Session == nil {
			t.Fatal("concurrent ACP execution omitted its Provider Session")
		}
		seen[got.response.Session.ID] = true
	}
	if starts.Load() != 2 || !seen["isolated-session-1"] || !seen["isolated-session-2"] || len(seen) != 2 {
		t.Fatalf("ACP starts=%d sessions=%v, want two isolated attempts", starts.Load(), seen)
	}
}
