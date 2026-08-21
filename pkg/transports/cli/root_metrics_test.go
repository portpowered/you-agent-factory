package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func TestProductionMetricsCommandUsesInjectedRuntimeMetricsQuery(t *testing.T) {
	called := false
	factory := withTestInjectedPlatformRoles(CommandFactory{
		runtimeMetricsQuery: func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			called = true
			if !strings.HasSuffix(request.MetricsRoot, ".you-agent-factory\\metrics") && !strings.HasSuffix(request.MetricsRoot, ".you-agent-factory/metrics") {
				t.Fatalf("metrics root = %q, want the default metrics directory", request.MetricsRoot)
			}
			return factoryvisualization.RuntimeMetricsQueryResult{
				Providers: []factoryvisualization.RuntimeMetricsBreakdown{{Key: "provider-a"}},
			}, nil
		},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"metrics", "--group-by", "provider"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	if !called {
		t.Fatal("production metrics command did not use the injected query")
	}
	if !strings.Contains(output.String(), "Breakdown by provider: 1 rows") || !strings.Contains(output.String(), "provider-a:") {
		t.Fatalf("output = %q, want provider breakdown", output.String())
	}
}
