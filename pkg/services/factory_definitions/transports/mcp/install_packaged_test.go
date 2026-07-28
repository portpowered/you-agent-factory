package factorydefinition_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
)

func installPackagedToolInput(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal install packaged input: %v", err)
	}
	return raw
}

func TestBind_InstallPackagedToolInvokesInjectedInstallRole(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	var got factorydefinitions.InstallPackagedFactoryRequest
	install := factorydefinitions.InstallPackagedFactoryOperation(func(
		_ context.Context,
		request factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		got = request
		return factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: destDir + "/goal",
			},
			Outcome: factorydefinitions.PackagedFactoryInstallCreated,
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		}, nil
	})
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Install: install})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		installPackagedToolInput(t, map[string]any{
			"package": "@you/goal",
			"dir":     destDir,
			"format":  "json",
			"replace": true,
		}),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}
	if got.Name != "@you/goal" {
		t.Fatalf("install request name = %q, want @you/goal", got.Name)
	}
	if got.RootDir != destDir {
		t.Fatalf("install request rootDir = %q, want %q", got.RootDir, destDir)
	}
	if got.Format != factorydefinitions.PackagedFactoryFormatJSON {
		t.Fatalf("install request format = %q, want JSON", got.Format)
	}
	if !got.Replace {
		t.Fatal("install request replace = false, want true")
	}

	var response struct {
		Result *factorydefinitionmcp.InstallPackagedResult `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope     `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("tool response result = nil, want install payload")
	}
	if response.Result.Name != "@you/goal" || response.Result.FactoryDir != destDir+"/goal" {
		t.Fatalf("install result = %#v, want goal install facts", response.Result)
	}
	if response.Result.Outcome != string(factorydefinitions.PackagedFactoryInstallCreated) {
		t.Fatalf("install outcome = %q, want created", response.Result.Outcome)
	}
	if response.Result.Format != string(factorydefinitions.PackagedFactoryFormatJSON) {
		t.Fatalf("install format = %q, want JSON", response.Result.Format)
	}
}

func TestBind_InstallPackagedToolUsesDefinitionsRootWhenInstallRoleUnset(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	root := &mcpCapturingInstallRootFake{
		result: factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: destDir + "/goal",
			},
			Outcome: factorydefinitions.PackagedFactoryInstallCreated,
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		},
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		installPackagedToolInput(t, map[string]any{
			"package": "@you/goal",
			"dir":     destDir,
		}),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}
	if !root.invoked {
		t.Fatal("InstallPackagedFactory was not invoked through the injected Definitions root")
	}
	if root.request.Name != "@you/goal" {
		t.Fatalf("install request name = %q, want @you/goal", root.request.Name)
	}

	var response struct {
		Result *factorydefinitionmcp.InstallPackagedResult `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope     `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || response.Result.Name != "@you/goal" {
		t.Fatalf("install result = %#v, want goal install payload", response.Result)
	}
}

func TestBind_InstallPackagedToolMissingPackageReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	install := factorydefinitions.InstallPackagedFactoryOperation(func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		t.Fatal("install should not run when package identity is missing")
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	})
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Install: install})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		installPackagedToolInput(t, map[string]any{"dir": destDir}),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if envelope.Message != "package name is required" {
		t.Fatalf("error.message = %q, want package name is required", envelope.Message)
	}
}

func TestBind_InstallPackagedToolUnknownPackageReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	installErr := &factorydefinitions.UnknownPackagedFactoryError{
		Name:      "@you/missing",
		Available: []string{"@you/goal"},
	}
	install := factorydefinitions.InstallPackagedFactoryOperation(func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, installErr
	})
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Install: install})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		installPackagedToolInput(t, map[string]any{
			"package": "@you/missing",
			"dir":     destDir,
		}),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "factory_definition.packaged.unknown_identity", false)
	if !strings.Contains(envelope.Message, "@you/missing") {
		t.Fatalf("error.message = %q, want missing package identity context", envelope.Message)
	}
	if envelope.Details["package"] != "@you/missing" {
		t.Fatalf("error.details.package = %#v, want @you/missing", envelope.Details["package"])
	}
	available, ok := envelope.Details["available"].([]any)
	if !ok || len(available) != 1 || available[0] != "@you/goal" {
		t.Fatalf("error.details.available = %#v, want [@you/goal]", envelope.Details["available"])
	}
}

func TestBind_InstallPackagedToolMalformedJSONDoesNotInvokeInstallRole(t *testing.T) {
	t.Parallel()

	install := factorydefinitions.InstallPackagedFactoryOperation(func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		t.Fatal("install should not run before request decode succeeds")
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	})
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Install: install})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		json.RawMessage(`{"package":`),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode install packaged input") {
		t.Fatalf("error.message = %q, want decode install packaged input context", envelope.Message)
	}
}

func TestBind_InstallPackagedToolDistributeFailureReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	install := factorydefinitions.InstallPackagedFactoryOperation(func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
	})
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Install: install})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolInstallPackaged,
		installPackagedToolInput(t, map[string]any{
			"package": "@you/goal",
			"dir":     destDir,
		}),
	)
	if err != nil {
		t.Fatalf("CallTool(install_packaged) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "factory_definition.packaged.distribute_failed", false)
	if envelope.Message != "factory distribute failed" {
		t.Fatalf("error.message = %q, want factory distribute failed", envelope.Message)
	}
}

type mcpCapturingInstallRootFake struct {
	mcpDefinitionsRootFake

	invoked bool
	request factorydefinitions.InstallPackagedFactoryRequest
	result  factorydefinitions.InstallPackagedFactoryResult
	err     error
}

func (fake *mcpCapturingInstallRootFake) InstallPackagedFactory(
	_ context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	fake.invoked = true
	fake.request = request
	if fake.err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, fake.err
	}
	if fake.result.Definition.Name != "" || fake.result.Outcome != "" {
		return fake.result, nil
	}
	return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
}
