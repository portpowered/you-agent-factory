package root

import (
	"context"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestBuildProcessComposesAutomationsHostedSourcesInertly(t *testing.T) {
	t.Parallel()

	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if apiStarts != 0 {
		t.Fatalf("BuildProcess() started API lifecycle %d times, want zero", apiStarts)
	}
	if process == nil {
		t.Fatal("BuildProcess() returned nil process")
	}
	if automations.HostedLinearDefaultRequestTimeout != 30*time.Second {
		t.Fatalf(
			"HostedLinearDefaultRequestTimeout = %s, want %s",
			automations.HostedLinearDefaultRequestTimeout,
			30*time.Second,
		)
	}
}
