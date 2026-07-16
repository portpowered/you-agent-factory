package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory/packages/classifier"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamedClassifierRouting_ClassifierLabelsDispatchOnlyMatchingTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier routing smoke")
	}

	for _, tc := range []struct {
		label             string
		wantWorkstation   string
		wantPrimaryResult string
	}{
		{label: "small", wantWorkstation: "run-small", wantPrimaryResult: "small result"},
		{label: "medium", wantWorkstation: "run-medium", wantPrimaryResult: "medium result"},
		{label: "large", wantWorkstation: "run-large", wantPrimaryResult: "large result"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			server := startNamedClassifierRoutingServer(t, tc.label)
			response := postNamedClassifierRoutingInvocation(t, server, "classifier routing "+tc.label)
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED", response.Status)
			}
			if got := invocationPrimaryResultText(t, response); got != tc.wantPrimaryResult {
				t.Fatalf("primaryResult = %q, want %q", got, tc.wantPrimaryResult)
			}

			assertNamedClassifierDispatchRoute(t, server, tc.wantWorkstation)
		})
	}
}

func TestNamedClassifierRouting_UnknownLabelFailsWithoutTargetDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier routing smoke")
	}

	server := startNamedClassifierRoutingServer(t, "unknown")
	response := postNamedClassifierRoutingInvocation(t, server, "classifier routing unknown")
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED", response.Status)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil", response.PrimaryResult)
	}
	if response.WorkState == nil || *response.WorkState != "task:failed" {
		t.Fatalf("workState = %#v, want task:failed", response.WorkState)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	if countClassifierDispatches(snapshot.DispatchHistory, classifier.ClassifierWorkstation) != 1 {
		t.Fatalf("classifier dispatches = %#v, want exactly one", snapshot.DispatchHistory)
	}
	for _, workstation := range []string{"run-small", "run-medium", "run-large"} {
		if countClassifierDispatches(snapshot.DispatchHistory, workstation) != 0 {
			t.Fatalf("unexpected target dispatch to %q: %#v", workstation, snapshot.DispatchHistory)
		}
	}
}

func TestNamedClassifierRouting_CustomizedRouteUsesConfiguredTargetPreset(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier customization smoke")
	}

	server := startNamedClassifierRoutingServerWithCustomization(t, "small", customizeMediumClassifierRoute)
	response := postNamedClassifierRoutingInvocation(t, server, "custom classifier routing")
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != "medium result" {
		t.Fatalf("primaryResult = %q, want medium result", got)
	}
	assertNamedClassifierDispatchRoute(t, server, "run-medium")
}

func TestNamedClassifierRouting_RealCLIUsesNamedFactoryForEachTier(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier CLI routing smoke")
	}

	binaryPath := buildYouCLIBinary(t)
	for _, tc := range []struct {
		label             string
		wantPrimaryResult string
	}{
		{label: "small", wantPrimaryResult: "small result"},
		{label: "medium", wantPrimaryResult: "medium result"},
		{label: "large", wantPrimaryResult: "large result"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			response, err := runNamedClassifierRoutingCLIJSON(t, binaryPath, tc.label)
			if err != nil {
				t.Fatalf("you run --named %s: %v", classifier.PackagedFactoryName, err)
			}
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED", response.Status)
			}
			if got := invocationPrimaryResultText(t, response); got != tc.wantPrimaryResult {
				t.Fatalf("primaryResult = %q, want %q", got, tc.wantPrimaryResult)
			}
		})
	}
}

func TestNamedClassifierRouting_RealCLIModelOverridesPreserveEachTier(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier CLI override routing smoke")
	}

	binaryPath := buildYouCLIBinary(t)
	for _, tc := range []struct {
		label             string
		wantPrimaryResult string
	}{
		{label: "small", wantPrimaryResult: "small result"},
		{label: "medium", wantPrimaryResult: "medium result"},
		{label: "large", wantPrimaryResult: "large result"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			response, err := runNamedClassifierRoutingCLIJSON(t, binaryPath, tc.label,
				"--default-worker-model-provider", "CLAUDE",
				"--default-worker-model", "claude-sonnet-4-20250514",
			)
			if err != nil {
				t.Fatalf("you run --named %s with model overrides: %v", classifier.PackagedFactoryName, err)
			}
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED", response.Status)
			}
			if got := invocationPrimaryResultText(t, response); got != tc.wantPrimaryResult {
				t.Fatalf("primaryResult = %q, want %q", got, tc.wantPrimaryResult)
			}
		})
	}
}

func TestNamedClassifierRouting_RealCLIRejectsUnsupportedProviderBeforeDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/classifier CLI override validation smoke")
	}

	_, err := runNamedClassifierRoutingCLIJSON(t, buildYouCLIBinary(t), "small",
		"--default-worker-model-provider", "not-a-provider",
	)
	if err == nil {
		t.Fatal("you run with an unsupported model provider succeeded")
	}
}

func startNamedClassifierRoutingServer(t *testing.T, classification string) *support.FunctionalAPIServer {
	return startNamedClassifierRoutingServerWithCustomization(t, classification, nil)
}

func startNamedClassifierRoutingServerWithCustomization(t *testing.T, classification string, customize func(homeDir, factoryDir string) error) *support.FunctionalAPIServer {
	t.Helper()

	homeDir := t.TempDir()
	initialized, err := configinit.Init(homeDir)
	if err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	factoryDir, err := factoryconfig.MapNamedFactoryDir(initialized.NamedFactoriesRoot, classifier.PackagedFactoryName)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(@you/classifier): %v", err)
	}
	if customize != nil {
		if err := customize(homeDir, factoryDir); err != nil {
			t.Fatalf("customize @you/classifier: %v", err)
		}
	}
	mockWorkersPath := writeClassifierRoutingMockWorkers(t, classification)
	mockWorkers, err := factoryconfig.LoadMockWorkersConfig(mockWorkersPath)
	if err != nil {
		t.Fatalf("LoadMockWorkersConfig: %v", err)
	}
	operatorDefaults, err := operatorconfig.ResolveFromHomeWithEnvironment(homeDir, operatorconfig.Defaults{}, operatorconfig.FlagOverrides{})
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment: %v", err)
	}

	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.SystemConfigHomeDir = homeDir
			cfg.OperatorDefaults = operatorDefaults
			cfg.MockWorkersConfig = mockWorkers
		},
	})
}

func customizeMediumClassifierRoute(homeDir, factoryDir string) error {
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryData, err := os.ReadFile(factoryPath)
	if err != nil {
		return err
	}
	var factoryDocument map[string]any
	if err := json.Unmarshal(factoryData, &factoryDocument); err != nil {
		return err
	}
	routes := factoryDocument["workstations"].([]any)[0].(map[string]any)["classificationRoutes"].([]any)
	routes[0].(map[string]any)["outputs"] = []any{map[string]any{"workType": "task", "state": "medium"}}
	factoryDocument["workers"].([]any)[2].(map[string]any)["preset"] = "custom-medium"
	updatedFactory, err := json.Marshal(factoryDocument)
	if err != nil {
		return err
	}
	if err := os.WriteFile(factoryPath, updatedFactory, 0o600); err != nil {
		return err
	}

	configPath := operatorconfig.DefaultConfigPath(homeDir)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var configDocument map[string]any
	if err := json.Unmarshal(configData, &configDocument); err != nil {
		return err
	}
	presets := configDocument["workerPresets"].([]any)
	configDocument["workerPresets"] = append(presets, map[string]any{"id": "custom-medium", "modelProvider": "CODEX", "model": "gpt-5.4"})
	updatedConfig, err := json.Marshal(configDocument)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, updatedConfig, 0o600)
}

func writeClassifierRoutingMockWorkers(t *testing.T, classification string) string {
	t.Helper()
	classifierCommand, classifierArgs := mockWorkerEchoCommand(classification)
	smallCommand, smallArgs := mockWorkerEchoCommand("small result")
	mediumCommand, mediumArgs := mockWorkerEchoCommand("medium result")
	largeCommand, largeArgs := mockWorkerEchoCommand("large result")
	return writeMockWorkersConfigFile(t, factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{WorkerName: "classify-complexity", WorkstationName: "classify-complexity", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: classifierCommand, Args: classifierArgs}},
			{WorkerName: "run-small", WorkstationName: "run-small", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: smallCommand, Args: smallArgs}},
			{WorkerName: "run-medium", WorkstationName: "run-medium", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: mediumCommand, Args: mediumArgs}},
			{WorkerName: "run-large", WorkstationName: "run-large", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: largeCommand, Args: largeArgs}},
		},
	}, "mock-workers-packaged-classifier-routing.json")
}

func runNamedClassifierRoutingCLIJSON(t *testing.T, binaryPath, classification string, operatorOverrideArgs ...string) (factoryapi.InvocationResponse, error) {
	t.Helper()

	homeDir := t.TempDir()
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"--json", "run"}
	args = append(args, operatorOverrideArgs...)
	args = append(args,
		"--named", classifier.PackagedFactoryName,
		"--with-mock-workers", "--no-record",
		"--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		"--quiet", writeClassifierRoutingMockWorkers(t, classification),
		"classifier CLI routing "+classification,
	)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr != nil && stderr.Len() > 0 {
		runErr = fmt.Errorf("%w\nstderr:\n%s", runErr, stderr.String())
	}
	var response factoryapi.InvocationResponse
	if stdout.Len() > 0 {
		if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
			t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
		}
	}
	return response, runErr
}

func postNamedClassifierRoutingInvocation(t *testing.T, server *support.FunctionalAPIServer, text string) factoryapi.InvocationResponse {
	t.Helper()
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(text),
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST classifier invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST classifier invocation status = %d, want 200: %s", response.StatusCode, string(payload))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode classifier invocation: %v", err)
	}
	return decoded
}

func assertNamedClassifierDispatchRoute(t *testing.T, server *support.FunctionalAPIServer, wantWorkstation string) {
	t.Helper()
	snapshot := server.GetEngineStateSnapshot(t)
	if countClassifierDispatches(snapshot.DispatchHistory, classifier.ClassifierWorkstation) != 1 {
		t.Fatalf("classifier dispatches = %#v, want exactly one", snapshot.DispatchHistory)
	}
	for _, workstation := range []string{"run-small", "run-medium", "run-large"} {
		want := 0
		if workstation == wantWorkstation {
			want = 1
		}
		if got := countClassifierDispatches(snapshot.DispatchHistory, workstation); got != want {
			t.Fatalf("dispatches to %q = %d, want %d; history: %#v", workstation, got, want, snapshot.DispatchHistory)
		}
	}
}

func countClassifierDispatches(dispatches []interfaces.CompletedDispatch, workstation string) int {
	count := 0
	for _, dispatch := range dispatches {
		if dispatch.WorkstationName == workstation {
			count++
		}
	}
	return count
}
