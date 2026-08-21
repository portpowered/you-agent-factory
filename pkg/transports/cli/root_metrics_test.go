package cli

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestProductionMetricsCommandResolvesGlobalJSONAndSessionScope(t *testing.T) {
	var gotRequest factoryvisualization.RuntimeMetricsQueryRequest
	factory := withTestInjectedPlatformRoles(CommandFactory{
		runtimeMetricsQuery: func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			gotRequest = request
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
	root.SetArgs([]string{"metrics", "--group-by", "provider", "--session", "session-a", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	if gotRequest.SessionID != "session-a" {
		t.Fatalf("query session ID = %q, want session-a", gotRequest.SessionID)
	}
	var document struct {
		Scope struct {
			Kind             string  `json:"kind"`
			FactorySessionID *string `json:"factory_session_id"`
		} `json:"scope"`
		GroupBy string `json:"group_by"`
		Groups  []struct {
			Key string `json:"key"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode metrics JSON: %v\n%s", err, output.String())
	}
	if document.Scope.Kind != "factory_session" || document.Scope.FactorySessionID == nil || *document.Scope.FactorySessionID != "session-a" {
		t.Fatalf("JSON scope = %#v, want session-a", document.Scope)
	}
	if document.GroupBy != "provider" || len(document.Groups) != 1 || document.Groups[0].Key != "provider-a" {
		t.Fatalf("JSON grouping = %q with groups %#v, want provider/provider-a", document.GroupBy, document.Groups)
	}
}
