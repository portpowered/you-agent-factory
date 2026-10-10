package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type saveDeniedDefinitionFiles struct{ platformfilesystem.Local }

func (files saveDeniedDefinitionFiles) MkdirTemp(parent, pattern string) (string, error) {
	if strings.HasPrefix(pattern, ".blocked.staging-") {
		return "", os.ErrPermission
	}
	return files.Local.MkdirTemp(parent, pattern)
}

type definitionResultCommand struct{}

func (definitionResultCommand) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := strings.Join(request.Args, " ") + string(request.Stdin)
	marker := "definition-before"
	if strings.Contains(prompt, "definition-after") {
		marker = "definition-after"
	} else if !strings.Contains(prompt, marker) {
		return platformprocess.CommandResult{}, errors.New("selected definition instructions missing from provider request")
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(marker + " COMPLETE")}, nil
}

// Save/readback and activation are session-owned API contracts; subsequent
// invocation proves the accepted definition is actually used by the runtime.
// The parent owns one exact persistence-edge graph through all parallel cells.
func TestFactorySaveFailureRetainsActiveDefinition(t *testing.T) {
	t.Parallel()
	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:                       api.Start,
		ProviderCommandRunner:                  definitionResultCommand{},
		FactoryDefinitionPersistenceFileSystem: saveDeniedDefinitionFiles{},
	})
	support.CleanupProcess(t, process)
	hostDir := support.ScaffoldSingleStepFactory(t, "definition-save-host")
	support.ClearSeedInputs(t, hostDir)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	type selectedDefinition struct{ name, root string }
	selected := []selectedDefinition{}
	for _, name := range []string{"save-success", "save-denied", "save-missing"} {
		dir := support.ScaffoldSingleStepFactory(t, name)
		support.ClearSeedInputs(t, dir)
		support.UpdateFactoryConfig(t, dir, func(config map[string]any) {
			config["workTypes"].([]any)[0].(map[string]any)["handlingBehavior"] = []string{"DEFAULT"}
		})
		support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "definition-model"))
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\ndefinition-before {{ (index .Inputs 0).Payload }}\n")
		portable, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, env, filepath.Join(dir, "factory.json"))
		if err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(dir, "portable.json")
		if err := os.WriteFile(source, portable, 0o600); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		support.CreateAndActivateNamedFactoryAtRootWithProcess(t, process, env, dir, root, name, source)
		selected = append(selected, selectedDefinition{name, root})
	}
	inputs := support.FakeInputs(context.Background(), []string{"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--no-record", "--quiet"})
	inputs.Input.Env, inputs.Input.WorkingDirectory = env, hostDir
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	baseURL := api.WaitForURL(t)
	for _, cell := range selected {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			id := openDefinitionsNamedSession(t, baseURL, cell.root, cell.name)
			t.Cleanup(func() { closeDefinitionsFactorySession(t, baseURL, id) })
			endpoint := baseURL + "/factory-sessions/" + id + "/factory"
			before := support.GetJSON[factoryapi.Factory](t, endpoint)
			assertDefinitionInvocation(t, baseURL, id, "definition-before")
			if cell.name == "save-missing" {
				assertMissingDefinitionKeepsOwner(t, baseURL, cell.root)
			} else {
				assertDefinitionSaveOutcome(t, endpoint, before, cell.name == "save-denied")
			}
			after := support.GetJSON[factoryapi.Factory](t, endpoint)
			want := "definition-before"
			if cell.name == "save-success" {
				want = "definition-after"
			} else if !reflect.DeepEqual(after, before) {
				t.Fatal("failed definition request changed active readback")
			}
			assertDefinitionInvocation(t, baseURL, id, want)
			if cell.name == "save-success" {
				fresh := openDefinitionsNamedSession(t, baseURL, cell.root, "accepted-update")
				t.Cleanup(func() { closeDefinitionsFactorySession(t, baseURL, fresh) })
				assertDefinitionInvocation(t, baseURL, fresh, "definition-after")
			}
		})
	}
}

func assertDefinitionSaveOutcome(t *testing.T, endpoint string, before factoryapi.Factory, denied bool) {
	t.Helper()
	// Detach slices from readback so rejected input cannot change the expected
	// retained definition in memory. A new named target avoids stale version.
	raw, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var updated factoryapi.Factory
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	updated.Name = "accepted-update"
	if denied {
		updated.Name = "blocked"
	}
	body := "definition-after {{ (index .Inputs 0).Payload }}"
	(*updated.Workstations)[0].Body = &body
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: updated, Mode: &mode})
	if err != nil {
		t.Fatal(err)
	}
	response, _, status := definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	if denied {
		assertDefinitionsHTTPError(t, response, status, http.StatusInternalServerError, "INTERNAL_ERROR")
		return
	}
	if status != http.StatusOK {
		t.Fatalf("save = %d %s", status, response)
	}
	var saved factoryapi.Factory
	if err := json.Unmarshal(response, &saved); err != nil {
		t.Fatal(err)
	}
	readback := support.GetJSON[factoryapi.Factory](t, endpoint)
	if saved.Name != updated.Name || !reflect.DeepEqual(saved, readback) || (*readback.Workstations)[0].Body == nil || *(*readback.Workstations)[0].Body != body {
		t.Fatal("accepted save/readback lost updated instructions")
	}
}

func assertMissingDefinitionKeepsOwner(t *testing.T, baseURL, root string) {
	t.Helper()
	name := "missing-definition"
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: root, Target: &factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindNamed, Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", payload)
	assertDefinitionsHTTPError(t, body, status, http.StatusBadRequest, "BAD_REQUEST")
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil || !strings.Contains(failure.Message, "Factory source could not be loaded or validated") {
		t.Fatal("missing definition lost its source diagnostic")
	}
}

func assertDefinitionInvocation(t *testing.T, baseURL, id, marker string) {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: "definition request"}); err != nil {
		t.Fatal(err)
	}
	content := factoryapi.WorkContent{part}
	kind := factoryapi.InvocationInputSourceKindText
	payload, err := json.Marshal(factoryapi.InvocationRequest{SourceKind: &kind, Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions/"+id+"/invocations", payload)
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || response.Status != factoryapi.InvocationTerminalStatusCompleted || response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("definition invocation = %d %s", status, body)
	}
	text, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || text.Text != marker+" COMPLETE" {
		t.Fatalf("definition result = %q, %v; want %s", text.Text, err, marker)
	}
}
