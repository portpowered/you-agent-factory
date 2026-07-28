package factory_transformation

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

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCurrentFactoryPUT_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	body := `{
		"name":"alpha",
		"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
		"workTypes":[{"name":"story","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"queued-dup","type":"PROCESSING"}
		]}],
		"workers":[
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"},
			{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}
		],
		"workstations":[{
			"name":"process",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"missing-worker",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"missing-state"}]
		}]
	}`

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid current factory save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) < 2 {
		t.Fatalf("error targets = %#v, want multiple blocking validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDuplicateIdentifier) ||
		!hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDanglingWorkerReference) ||
		!hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want duplicate worker, dangling worker, and dangling place targets", errResp.Targets)
	}
}

func TestCurrentFactoryPUT_ReturnsCanonicalTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	body := `{
		"name":"alpha",
		"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
		"workTypes":[{"name":"story","states":[{"name":"queued","type":"INITIAL"}]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"process","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"worker-a","inputs":[{"workType":"story","state":"queued"}],"outputs":[{"workType":"story","state":"missing-state"}]}]
	}`

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid current factory save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || len(*errResp.Targets) == 0 {
		t.Fatalf("error targets = %#v, want canonical topology validation targets", errResp.Targets)
	}
	if !hasValidationTarget(
		*errResp.Targets,
		"factory.route.danglingPlaceReference",
		factoryapi.FactoryValidationSubjectTypeRoute,
		"process->story:missing-state",
		factoryapi.FactoryValidationSubjectLocationOutputs,
	) {
		t.Fatalf("error targets = %#v, want dangling output workstation target", errResp.Targets)
	}
}

func TestCurrentFactoryPUT_RejectsTypeCountCollisionBeforePersistingDefaultFactory(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}
	body, err := json.Marshal(map[string]any{
		"name":             "UNDEFINED",
		"factoryDirectory": "factory",
		"sourceDirectory":  "factory",
		"version":          versionDocument(advancedFactoryVersion(t, current.Version)),
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "in-review", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "processor",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":     "process",
			"behavior": "REPEATER",
			"type":     "MODEL_WORKSTATION",
			"worker":   "processor",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{
				{"workType": "task", "state": "in-review"},
				{"workType": "task", "state": "complete"},
			},
			"onContinue": []map[string]string{{"workType": "task", "state": "init"}},
			"onFailure":  []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal type-count collision save document: %v", err)
	}

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), string(body), http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode type-count collision save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || !hasValidationTarget(
		*errResp.Targets,
		"factory.workstation.conflictingWorkStateOutputs",
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"process",
		factoryapi.FactoryValidationSubjectLocationOutputs,
	) {
		t.Fatalf("error targets = %#v, want conflicting output workstation target", errResp.Targets)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Version == nil || *reloaded.Version != *current.Version {
		t.Fatalf("reloaded version = %#v, want unchanged %#v", reloaded.Version, current.Version)
	}
	assertFactoryWorkType(t, reloaded, "task", "reloaded factory after rejected type-count collision")
}

func startFactoryTransformationServer(t *testing.T, rootDir string) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
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

func saveCurrentFactoryDefinition(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatus(t, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func saveCurrentFactoryDefinitionExpectStatus(t *testing.T, serverURL, body string, wantStatus int) *http.Response {
	t.Helper()
	return saveCurrentFactoryDefinitionExpectStatusWithClient(t, http.DefaultClient, serverURL, body, wantStatus)
}

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func putFactoryForSessionRequestExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
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

	resp, err := client.Do(req)
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

func saveCurrentFactoryDefinitionExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	return putFactoryForSessionRequestExpectStatusWithClient(
		t,
		client,
		serverURL,
		"/factory-sessions/~default/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	resp, err := http.Post(support.DefaultSessionWorkURL(serverURL, "/work"), "application/json", bytes.NewBufferString(`{"name":"factory-transformation-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
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

func requireFactoryChangeAfter(t *testing.T, before []factoryapi.FactoryEvent, after []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	minSequence := -1
	for _, event := range before {
		if event.Context.Sequence > minSequence {
			minSequence = event.Context.Sequence
		}
	}
	for _, event := range after {
		if event.Context.Sequence > minSequence && event.Type == factoryapi.FactoryEventTypeFactoryChange {
			return event
		}
	}
	t.Fatalf("factory-change event not found after save; before=%d after=%d", len(before), len(after))
	return factoryapi.FactoryEvent{}
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

func functionalNamedFactoryPayloadWithWorkType(t *testing.T, name, workType string) []byte {
	t.Helper()
	return []byte(functionalNamedFactoryPayloadJSON(name, workType))
}

func functionalNamedFactoryBody(name, workType string, version ...factoryapi.HybridLogicalTimestamp) string {
	if len(version) == 0 {
		return functionalNamedFactoryPayloadJSON(name, workType)
	}
	return currentFactorySaveDocument(nil, name, workType, versionDocument(version[0]))
}

func currentFactorySaveDocument(t *testing.T, name, workType string, version any) string {
	if t != nil {
		t.Helper()
	}
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
			"inputs":   []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":  []map[string]string{{"workType": workType, "state": "done"}},
		}},
	}
	if version != nil {
		document["version"] = version
	}
	body, err := json.Marshal(document)
	if err != nil {
		if t != nil {
			t.Fatalf("marshal current factory save document: %v", err)
		}
		panic(err)
	}
	return string(body)
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

func functionalNamedFactoryPayloadJSON(name, workType string) string {
	return `{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
}
