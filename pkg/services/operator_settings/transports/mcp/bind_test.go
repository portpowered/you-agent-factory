package operatorsettingsmcp_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
)

const testConfigPath = "/home/operator/.you-agent-factory/config.json"

func TestBind_FakeRootInvokedThroughCanonicalLoadDocumentTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	scopeID := "local-00000000-0000-4000-8000-000000000010"
	fake := fakeSettingsRoot{
		invoked: &invoked,
		loadDocument: func(request operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			if request.Path != testConfigPath {
				t.Fatalf("path = %q, want %q", request.Path, testConfigPath)
			}
			if !request.RequireExisting {
				t.Fatal("RequireExisting = false, want true")
			}
			return operatorsettings.LoadDocumentResult{
				Document: operatorsettings.Document{BackendScopeID: scopeID},
				Path:     request.Path,
				Found:    true,
			}, nil
		},
	}
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: fake})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolLoadDocument,
		json.RawMessage(`{"path":"`+testConfigPath+`","requireExisting":true}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_document) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake settings root was not invoked")
	}
	var response mcpoperatorsettings.ToolResponse[operatorsettings.LoadDocumentResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("CallTool(load_document) = %s, want success", raw)
	}
	if response.Result.Document.BackendScopeID != scopeID {
		t.Fatalf("BackendScopeID = %q, want %q", response.Result.Document.BackendScopeID, scopeID)
	}
}

func TestCallTool_UnknownToolReturnsStableError(t *testing.T) {
	t.Parallel()

	_, err := mcpoperatorsettings.CallTool(
		context.Background(),
		fakeSettingsRoot{},
		"you.operator_settings.unknown_tool",
		json.RawMessage(`{}`),
	)
	if err == nil {
		t.Fatal("CallTool(unknown tool) error = nil, want unsupported-tool error")
	}
	if got := err.Error(); got != `unsupported tool "you.operator_settings.unknown_tool"` {
		t.Fatalf("CallTool(unknown tool) error = %q, want %q", got, `unsupported tool "you.operator_settings.unknown_tool"`)
	}
}

func TestBind_ToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpoperatorsettings.BindToolOperation(fakeSettingsRoot{invoked: &invoked})
	_, err := operation(nil, mcpoperatorsettings.ToolLoadDocument, json.RawMessage(`{"path":"`+testConfigPath+`"}`))
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
	if invoked {
		t.Fatal("fake settings root was invoked for nil context")
	}
}

func TestPackageBoundary_DoesNotImportOperatorSettingsInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal"
	packagePath := "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
}

type fakeSettingsRoot struct {
	operatorsettings.Service
	invoked              *bool
	loadDocument         func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	applyDocumentUpdate  func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error)
	resolveEffective     func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error)
}

func (fake fakeSettingsRoot) markInvoked() {
	if fake.invoked != nil {
		*fake.invoked = true
	}
}

func (fake fakeSettingsRoot) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	fake.markInvoked()
	if fake.loadDocument == nil {
		panic("unexpected LoadDocument on fake settings root")
	}
	return fake.loadDocument(request)
}

func (fake fakeSettingsRoot) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	fake.markInvoked()
	if fake.applyDocumentUpdate == nil {
		panic("unexpected ApplyDocumentUpdate on fake settings root")
	}
	return fake.applyDocumentUpdate(request)
}

func (fake fakeSettingsRoot) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	fake.markInvoked()
	if fake.resolveEffective == nil {
		panic("unexpected ResolveEffective on fake settings root")
	}
	return fake.resolveEffective(request)
}

func assertPackageDirectImportsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	}
}
