package functionalscenarios

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestProjectCanonicalRepositoryInventories(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	projection, err := Project(
		readFile(t, filepath.Join(root, "contracts", "cli", "commands.json")),
		readFile(t, filepath.Join(root, "api", "openapi.yaml")),
		readFile(t, filepath.Join(root, "contracts", "mcp", "tools.json")),
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}

	byID := make(map[string]Component, len(projection.Components))
	for _, component := range projection.Components {
		byID[component.StableID] = component
	}

	wantComponents := map[string]Component{
		"cli/you.session": {
			StableID: "cli/you.session", Interface: InterfaceCLI, Identity: "you.session",
			Name: "you session", Classification: ClassificationGrouping,
		},
		"cli/you.session.list": {
			StableID: "cli/you.session.list", Interface: InterfaceCLI, Identity: "you.session.list",
			Name: "you session list", Classification: ClassificationRunnable,
		},
		"rest/getFactorySession": {
			StableID: "rest/getFactorySession", Interface: InterfaceREST, Identity: "getFactorySession",
			Name: "getFactorySession", Classification: ClassificationOperation,
		},
		"mcp/mcp.tool.you.factory_session.get": {
			StableID: "mcp/mcp.tool.you.factory_session.get", Interface: InterfaceMCP,
			Identity: "mcp.tool.you.factory_session.get", Name: "you.factory_session.get", Classification: ClassificationTool,
		},
		"sse/getEventsBySessionId": {
			StableID: "sse/getEventsBySessionId", Interface: InterfaceSSE, Identity: "getEventsBySessionId",
			Name: "getEventsBySessionId", Classification: ClassificationEventStream,
		},
		"sse/getFactoryResponseEventsBySessionId": {
			StableID: "sse/getFactoryResponseEventsBySessionId", Interface: InterfaceSSE,
			Identity: "getFactoryResponseEventsBySessionId", Name: "getFactoryResponseEventsBySessionId", Classification: ClassificationEventStream,
		},
	}
	for stableID, want := range wantComponents {
		if got, ok := byID[stableID]; !ok || got != want {
			t.Fatalf("component %q = %+v, present %v, want %+v", stableID, got, ok, want)
		}
	}
}

func TestProjectIsSortedAndCanonicalSerializationIsRepeatable(t *testing.T) {
	t.Parallel()

	cli := []byte(`{"commands":{"second":{"id":"you.z","path":"you z","runnable":true},"first":{"id":"you.a","path":"you a","runnable":false}}}`)
	mcp := []byte(`{"tools":{"second":{"id":"mcp.tool.z","name":"z"},"first":{"id":"mcp.tool.a","name":"a"}}}`)
	openAPI := []byte(`openapi: 3.0.3
info: {title: test, version: 1.0.0}
paths:
  /z:
    get:
      operationId: zOperation
      responses:
        '200': {description: ok}
  /a:
    get:
      operationId: aEvents
      responses:
        '200':
          description: ok
          content:
            text/event-stream: {schema: {type: string}}
`)
	first, err := Project(cli, openAPI, mcp)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	second, err := Project(cli, openAPI, mcp)
	if err != nil {
		t.Fatalf("second Project() error = %v", err)
	}
	firstJSON, err := MarshalCanonicalJSON(first)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}
	secondJSON, err := MarshalCanonicalJSON(second)
	if err != nil {
		t.Fatalf("second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical bytes differ:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	stableIDs := make([]string, 0, len(first.Components))
	for _, component := range first.Components {
		stableIDs = append(stableIDs, component.StableID)
	}
	if !slices.IsSorted(stableIDs) {
		t.Fatalf("stable IDs are not sorted: %v", stableIDs)
	}
}

func TestProjectRejectsMissingAndDuplicateCanonicalIdentities(t *testing.T) {
	t.Parallel()

	validCLI := []byte(`{"commands":{"one":{"id":"you.one","path":"you one","runnable":true}}}`)
	validMCP := []byte(`{"tools":{"one":{"id":"mcp.tool.one","name":"one"}}}`)
	validOpenAPI := []byte(`openapi: 3.0.3
info: {title: test, version: 1.0.0}
paths:
  /one:
    get:
      operationId: getOne
      responses:
        '200': {description: ok}
`)
	tests := []struct {
		name, want        string
		cli, openAPI, mcp []byte
	}{
		{
			name: "missing cli identity", want: `cli interface: missing canonical identity for command key "one"`,
			cli: []byte(`{"commands":{"one":{"path":"you one","runnable":true}}}`), openAPI: validOpenAPI, mcp: validMCP,
		},
		{
			name: "duplicate cli identity", want: `cli interface: duplicate canonical identity "you.same"`,
			cli: []byte(`{"commands":{"one":{"id":"you.same","path":"you one"},"two":{"id":"you.same","path":"you two"}}}`), openAPI: validOpenAPI, mcp: validMCP,
		},
		{
			name: "missing rest identity", want: "rest interface: missing operationId for GET /one",
			cli: validCLI, openAPI: []byte(`openapi: 3.0.3
info: {title: test, version: 1.0.0}
paths:
  /one:
    get:
      responses:
        '200': {description: ok}
`), mcp: validMCP,
		},
		{
			name: "missing mcp identity", want: `mcp interface: missing canonical identity for tool key "one"`,
			cli: validCLI, openAPI: validOpenAPI, mcp: []byte(`{"tools":{"one":{"name":"one"}}}`),
		},
		{
			name: "duplicate mcp identity", want: `mcp interface: duplicate canonical identity "mcp.tool.same"`,
			cli: validCLI, openAPI: validOpenAPI, mcp: []byte(`{"tools":{"one":{"id":"mcp.tool.same","name":"one"},"two":{"id":"mcp.tool.same","name":"two"}}}`),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Project(test.cli, test.openAPI, test.mcp)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Project() error = %v, want diagnostic containing %q", err, test.want)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not resolve test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
