package definitions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startDefinitionsValidationServer(t *testing.T, rootDir string) (*support.FunctionalAPIServer, *support.RecordingCommandRunner) {
	t.Helper()

	runner := support.NewRecordingCommandRunner("runtime must not execute during definitions validation")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	return server, runner
}

func seedNamedFactoryRoot(t *testing.T, rootDir, name, workType string) {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, functionalNamedFactoryPayloadWithWorkType(t, name, workType), 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func functionalNamedFactoryPayloadWithWorkType(t *testing.T, name, workType string) []byte {
	t.Helper()
	return []byte(functionalNamedFactoryPayloadJSON(name, workType))
}

func functionalNamedFactoryPayloadJSON(name, workType string) string {
	return `{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
}

func getCurrentFactory(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", resp.StatusCode)
	}
	var current factoryapi.Factory
	decodeJSONResponse(t, resp, &current, "decode current factory response")
	return current
}

func saveCurrentFactoryDefinitionExpectStatus(t *testing.T, serverURL, body string, wantStatus int) *http.Response {
	t.Helper()
	return putFactoryForSessionRequestExpectStatus(
		t,
		serverURL,
		"/factory-sessions/~default/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func createNamedFactoryExpectStatus(t *testing.T, serverURL, body string, wantStatus int) *http.Response {
	t.Helper()
	requestBody := fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, body)
	return putFactoryForSessionRequestExpectStatus(
		t,
		serverURL,
		"/factory-sessions/~default/factory",
		requestBody,
		wantStatus,
	)
}

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func putFactoryForSessionRequestExpectStatus(
	t *testing.T,
	serverURL,
	path string,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, serverURL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new factory session save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("PUT %s status = %d, want %d: %s", path, resp.StatusCode, wantStatus, payload)
	}
	return resp
}

func decodeJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func advancedFactoryVersion(t *testing.T, version *factoryapi.HybridLogicalTimestamp) factoryapi.HybridLogicalTimestamp {
	t.Helper()
	if version == nil {
		t.Fatal("factory version = nil, want version metadata")
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  version.Logical + 1,
		Physical: version.Physical.UTC().Add(time.Nanosecond),
	}
}

func versionDocument(version factoryapi.HybridLogicalTimestamp) map[string]string {
	return map[string]string{
		"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
}

func hasValidationTarget(
	targets []factoryapi.FactoryValidationTarget,
	code string,
	subjectType factoryapi.FactoryValidationSubjectType,
	subjectID string,
	location factoryapi.FactoryValidationSubjectLocation,
) bool {
	for _, target := range targets {
		if target.Code != code {
			continue
		}
		if target.Subject.Type == subjectType && target.Subject.Id == subjectID && target.Subject.Location == location {
			return true
		}
	}
	return false
}

func assertFactoryWorkType(t *testing.T, factory factoryapi.Factory, want string, contextLabel string) {
	t.Helper()
	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 || (*factory.WorkTypes)[0].Name != want {
		t.Fatalf("%s work types = %#v, want %s", contextLabel, factory.WorkTypes, want)
	}
}

func assertValidationRunnerIdle(t *testing.T, runner *support.RecordingCommandRunner) {
	t.Helper()
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during validate-only API calls", runner.CallCount())
	}
}
