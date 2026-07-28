package wire_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProviderServiceUsesFreshACPProcessForTheNextAttemptAfterFailure(t *testing.T) {
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

	request := providers.ExecuteRequest{
		ProviderID: "cursor-acp", Instructions: "fresh ACP attempts",
		Prompt:           []providers.ContentPart{{Kind: providers.ContentKindText, Text: "execute attempt"}},
		WorkingDirectory: t.TempDir(),
		Environment:      []providers.EnvironmentEntry{{Name: providersACPModeEnvironment, Value: "fail"}},
	}
	if _, err := service.Execute(context.Background(), request); err == nil || !strings.Contains(err.Error(), "functional ACP prompt failure") {
		t.Fatalf("first ACP attempt error = %v, want prompt failure", err)
	}

	request.Environment[0].Value = "success"
	response, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("second ACP attempt error = %v", err)
	}
	if response.Session == nil || response.Session.ID != "providers-integration-session" || !strings.Contains(response.Content, "COMPLETE") {
		t.Fatalf("second ACP attempt response = %#v", response)
	}
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want one fresh process per attempt", starts.Load())
	}
}
