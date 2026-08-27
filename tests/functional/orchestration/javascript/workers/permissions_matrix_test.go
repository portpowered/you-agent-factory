package workers_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const stableDisallowedPermissionDiagnostic = `policy denied: Factory "named-factory" child "skip-child" requested permission "SKIP_PERMISSIONS" not listed in allowedPermissions`

const disallowedPermissionWorkflow = `return (async function () {
  return await agent.run({
    prompt: "permission denial child",
    label: "skip-child",
    modelProvider: "codex",
    permissions: "SKIP_PERMISSIONS"
  });
})();`

func runJavaScriptPermissionMatrixCharacterization(t *testing.T, fixture *javascriptSharedProcessFixture) {
	tests := []struct {
		name        string
		factoryName string
		prompt      string
		permissions string
		wantArgs    []string
	}{
		{
			name:        "permissions-omitted",
			factoryName: sharedJavaScriptPermissionOmittedFactory,
			prompt:      "shared permissions omitted",
			permissions: "omitted",
			wantArgs:    []string{"exec", "--json", "-"},
		},
		{
			name:        "permissions-default",
			factoryName: sharedJavaScriptPermissionDefaultFactory,
			prompt:      "shared permissions default",
			permissions: "DEFAULT",
			wantArgs:    []string{"exec", "--json", "-"},
		},
		{
			name:        "permissions-skip",
			factoryName: sharedJavaScriptPermissionSkipFactory,
			prompt:      "shared permissions skip",
			permissions: "SKIP_PERMISSIONS",
			wantArgs:    []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runner := support.NewRecordingCommandRunner("permission matrix child output")
			if err := fixture.router.register(test.prompt, runner); err != nil {
				t.Fatalf("register permission route: %v", err)
			}
			t.Cleanup(func() {
				if err := fixture.router.unregister(test.prompt); err != nil {
					t.Errorf("unregister permission route: %v", err)
				}
			})
			beforeSessions := fixture.persistentSessionIDs(t)
			inputs, err := fixture.executeRemote(t, test.factoryName, "shared "+test.name)
			if err != nil {
				t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
			}
			assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
			if !strings.Contains(inputs.Stdout(), "permission matrix child output") {
				t.Fatalf("permission matrix output = %q, want provider output", inputs.Stdout())
			}
			requests := fixture.router.requestRecords()
			if len(requests) == 0 {
				t.Fatal("permission matrix provider request count = 0, want one")
			}
			got := requests[len(requests)-1]
			if got.Command != "codex" || !reflect.DeepEqual(got.Args, test.wantArgs) {
				t.Fatalf("provider command = %q %#v, want codex %#v", got.Command, got.Args, test.wantArgs)
			}
			afterSessions := fixture.persistentSessionIDs(t)
			newSessions := differenceJavaScriptSessionIDs(beforeSessions, afterSessions)
			if len(newSessions) != 1 {
				t.Fatalf("permission matrix sessions before=%v after=%v new=%v, want one owning session", beforeSessions, afterSessions, newSessions)
			}
			assertJavaScriptSharedCompletedDispatch(t, fixture, newSessions[0], "codex", "", "permission-matrix-child")
			fixture.trackSession(t, newSessions[0])
		})
	}
}

func runJavaScriptDisallowedPermission(t *testing.T, fixture *javascriptSharedProcessFixture) {
	beforeCalls := fixture.router.callCount()
	beforeSessions := fixture.persistentSessionIDs(t)
	inputs, err := fixture.executeRemote(t, fixture.disallowedFactory, "shared permission denial")
	if err == nil {
		t.Fatalf("Process.Execute() error = nil; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}
	assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode disallowed permission invocation response: %v; stdout=%q", decodeErr, inputs.Stdout())
	}
	if response.SessionId == nil || strings.TrimSpace(*response.SessionId) == "" {
		t.Fatalf("disallowed permission response = %#v, want failed session identity", response)
	}
	session := readJavaScriptSharedDurableSession(t, fixture.baseURL, *response.SessionId)
	if session.FailureDetail == nil {
		t.Fatalf("disallowed permission durable session = %#v, want failure detail", session)
	}
	output := strings.Join([]string{inputs.Stdout(), inputs.Stderr(), err.Error(), session.FailureDetail.Message}, "\n")
	if !strings.Contains(output, stableDisallowedPermissionDiagnostic) {
		t.Fatalf("public denial diagnostic = %q, want %q", output, stableDisallowedPermissionDiagnostic)
	}
	if fixture.router.callCount() != beforeCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged %d before denied child dispatch", fixture.router.callCount(), beforeCalls)
	}
	afterSessions := fixture.persistentSessionIDs(t)
	newSessions := differenceJavaScriptSessionIDs(beforeSessions, afterSessions)
	if len(newSessions) != 1 || newSessions[0] != *response.SessionId {
		t.Fatalf("disallowed permission sessions before=%v after=%v new=%v, want one owning failed session", beforeSessions, afterSessions, newSessions)
	}
	assertJavaScriptSharedNoDispatch(t, fixture, *response.SessionId)
	fixture.trackSession(t, *response.SessionId)
}

func permissionMatrixFactoryConfig(source string) map[string]any {
	config := map[string]any{}
	config["name"] = "javascript-permission-matrix"
	config["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{
			"name": "prompt", "required": false,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
		}},
	}
	config["orchestrator"] = map[string]any{
		"kind": "JAVASCRIPT",
		"javascript": map[string]any{
			"inlineSource": map[string]any{
				"encoding": "utf-8",
				"inline":   source,
			},
			"argsSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
				"additionalProperties": false,
			},
		},
	}
	return config
}

func disallowedPermissionFactoryConfig() map[string]any {
	config := permissionMatrixFactoryConfig(disallowedPermissionWorkflow)
	config["name"] = "named-factory"
	orchestrator := config["orchestrator"].(map[string]any)
	javascript := orchestrator["javascript"].(map[string]any)
	javascript["sourceRef"] = "workflows/review.js"
	delete(javascript, "inlineSource")
	javascript["defaultPolicy"] = map[string]any{
		"allowedPermissions": []any{"DEFAULT"},
	}
	return config
}

func permissionMatrixWorkflow(permissions string) string {
	return permissionMatrixWorkflowWithPrompt(permissions, "capture the current Codex command")
}

func permissionMatrixWorkflowWithPrompt(permissions, prompt string) string {
	field := ""
	if permissions != "omitted" {
		field = `, permissions: "` + permissions + `"`
	}
	return `return (async function () {
  return await agent.run({
    prompt: "` + prompt + `",
    label: "permission-matrix-child",
    modelProvider: "codex"` + field + `
  });
})();`
}
