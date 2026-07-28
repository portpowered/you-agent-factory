package definition_activation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestDefinitionActivationGatewaySaveAndNamedSwap exercises editable save and
// named upsert/activate paths through production Definitions↔Sessions wiring so
// activation gateway delegation remains covered by the functional suite.
func TestDefinitionActivationGatewaySaveAndNamedSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("slow definition activation gateway wiring")
	}

	rootDir := t.TempDir()
	seedDefinitionActivationFactory(t, rootDir, "alpha", "alpha-task")

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	current := getDefinitionActivationCurrentFactory(t, baseURL)
	firstSaved := saveDefinitionActivationCurrentFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("alpha", "story", current.Version),
	)
	if firstSaved.WorkTypes == nil || len(*firstSaved.WorkTypes) != 1 || (*firstSaved.WorkTypes)[0].Name != "story" {
		t.Fatalf("first saved work types = %#v, want story", firstSaved.WorkTypes)
	}

	secondSaved := saveDefinitionActivationCurrentFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("alpha", "article", firstSaved.Version),
	)
	if secondSaved.WorkTypes == nil || len(*secondSaved.WorkTypes) != 1 || (*secondSaved.WorkTypes)[0].Name != "article" {
		t.Fatalf("second saved work types = %#v, want article", secondSaved.WorkTypes)
	}

	activated := upsertDefinitionActivationNamedFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("beta", "beta-task", nil),
	)
	if activated.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("activated factory name = %q, want beta", activated.Name)
	}

	current = getDefinitionActivationCurrentFactory(t, baseURL)
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory after beta activation = %q, want beta", current.Name)
	}

	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server", baseURL,
		"factory", "replace-current",
	})
	inputs.Input.WorkingDirectory = filepath.Join(rootDir, "beta")
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(factory replace-current) error = %v; stderr=%q", err, inputs.Stderr())
	}

	postReplace := getDefinitionActivationCurrentFactory(t, baseURL)
	if postReplace.Version == nil || current.Version == nil {
		t.Fatal("expected version metadata after replace-current")
	}
	if postReplace.Version.Logical.Int64() <= current.Version.Logical.Int64() {
		t.Fatalf(
			"post-replace logical version = %d, want > %d",
			postReplace.Version.Logical.Int64(),
			current.Version.Logical.Int64(),
		)
	}
}

// TestDefinitionActivationGatewayUpsertReplaceAndSwitch covers named upsert
// replace and switching the session current factory between persisted names.
func TestDefinitionActivationGatewayUpsertReplaceAndSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("slow definition activation gateway wiring")
	}

	rootDir := t.TempDir()
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(
		sourcePath,
		[]byte(definitionActivationFactoryBody("alpha", "alpha-task", nil)),
		0o600,
	); err != nil {
		t.Fatalf("write alpha source: %v", err)
	}
	support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, "alpha", sourcePath)
	betaDir := support.CreateNamedFactoryAtRoot(t, sourceDir, rootDir, "beta", sourcePath)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	activatedBeta := upsertDefinitionActivationNamedFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("beta", "beta-task", nil),
	)
	if activatedBeta.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("activated beta name = %q, want beta", activatedBeta.Name)
	}

	replacedBeta := upsertDefinitionActivationNamedFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("beta", "story", nil),
	)
	if replacedBeta.WorkTypes == nil || len(*replacedBeta.WorkTypes) != 1 || (*replacedBeta.WorkTypes)[0].Name != "story" {
		t.Fatalf("replaced beta work types = %#v, want story", replacedBeta.WorkTypes)
	}

	activatedAlpha := upsertDefinitionActivationNamedFactory(
		t,
		baseURL,
		definitionActivationFactoryBody("alpha", "alpha-task", nil),
	)
	if activatedAlpha.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("activated alpha name = %q, want alpha", activatedAlpha.Name)
	}

	current := getDefinitionActivationCurrentFactory(t, baseURL)
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory = %q, want alpha", current.Name)
	}
	if _, err := os.Stat(filepath.Join(betaDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("beta factory.json missing after switch: %v", err)
	}
}

// TestDefinitionActivationGatewayDefaultRootSave exercises replace-current saves
// against the default root factory before a durable current pointer exists.
func TestDefinitionActivationGatewayDefaultRootSave(t *testing.T) {
	if testing.Short() {
		t.Skip("slow definition activation gateway wiring")
	}

	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		[]byte(definitionActivationFactoryBody("root-runtime", "root-task", nil)),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	current := getDefinitionActivationCurrentFactory(t, baseURL)
	if current.Name != "UNDEFINED" {
		t.Fatalf("default current factory name = %q, want UNDEFINED", current.Name)
	}

	saved := saveDefinitionActivationCurrentFactory(
		t,
		baseURL,
		definitionActivationDefaultFactorySaveBody("root-runtime", "story", current.Version),
	)
	if saved.WorkTypes == nil || len(*saved.WorkTypes) != 1 || (*saved.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved default factory work types = %#v, want story", saved.WorkTypes)
	}
}

func seedDefinitionActivationFactory(t *testing.T, rootDir, name, workType string) {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(
		sourcePath,
		[]byte(definitionActivationFactoryBody(name, workType, nil)),
		0o600,
	); err != nil {
		t.Fatalf("write factory source %s: %v", name, err)
	}
	support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func getDefinitionActivationCurrentFactory(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		serverURL+"/factory-sessions/~default/factory",
		nil,
	)
	if err != nil {
		t.Fatalf("build GET current factory request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", response.StatusCode)
	}
	var current factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	return current
}

func saveDefinitionActivationCurrentFactory(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		serverURL+"/factory-sessions/~default/factory",
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		t.Fatalf("build PUT current factory request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/~default/factory: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("PUT /factory-sessions/~default/factory status = %d, want 200", response.StatusCode)
	}
	var saved factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&saved); err != nil {
		t.Fatalf("decode saved current factory response: %v", err)
	}
	return saved
}

func upsertDefinitionActivationNamedFactory(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(body), &factory); err != nil {
		t.Fatalf("decode named factory body: %v", err)
	}
	factory.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(1<<62 - 1),
		Physical: time.Now().UTC().Add(time.Hour),
	}
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{
		Factory: factory,
		Mode:    &mode,
	})
	if err != nil {
		t.Fatalf("encode named factory activation request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		serverURL+"/factory-sessions/~default/factory",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build PUT named factory request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT named factory activation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("PUT named factory activation status = %d, want 200", response.StatusCode)
	}
	var activated factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&activated); err != nil {
		t.Fatalf("decode named factory activation response: %v", err)
	}
	return activated
}

func definitionActivationFactoryBody(
	name, workType string,
	version *factoryapi.HybridLogicalTimestamp,
) string {
	document := map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"inputs": []map[string]string{
				{"workType": workType, "state": "init"},
			},
			"outputs": []map[string]string{
				{"workType": workType, "state": "done"},
			},
		}},
	}
	if version != nil {
		document["version"] = map[string]any{
			"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
			"physical": version.Physical.UTC().Format(time.RFC3339Nano),
		}
	}
	payload, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(payload)
}

func definitionActivationDefaultFactorySaveBody(
	id, workType string,
	version *factoryapi.HybridLogicalTimestamp,
) string {
	document := map[string]any{
		"name": "UNDEFINED",
		"id":   id,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"inputs": []map[string]string{
				{"workType": workType, "state": "init"},
			},
			"outputs": []map[string]string{
				{"workType": workType, "state": "done"},
			},
		}},
	}
	if version != nil {
		document["version"] = map[string]any{
			"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
			"physical": version.Physical.UTC().Format(time.RFC3339Nano),
		}
	}
	payload, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(payload)
}
