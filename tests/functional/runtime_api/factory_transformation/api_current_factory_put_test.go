package factory_transformation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestCurrentFactoryPUT_SaveEditableCurrentFactoryDefinitionEmitsCanonicalFactoryChangeEvent(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	initialEvents := server.GetFactoryEvents(t)

	current := getCurrentFactory(t, server.URL())
	saved := saveCurrentFactoryDefinition(t, server.URL(), functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)))
	if saved.WorkTypes == nil || len(*saved.WorkTypes) != 1 || (*saved.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved current factory work types = %#v, want story", saved.WorkTypes)
	}

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	payload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	if payload.Factory.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("factory-change payload name = %q, want alpha", payload.Factory.Name)
	}
	if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) != 1 || (*payload.Factory.WorkTypes)[0].Name != "story" {
		t.Fatalf("factory-change payload work types = %#v, want story", payload.Factory.WorkTypes)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 || (*payload.Factory.Workstations)[0].Name != "plan-task" {
		t.Fatalf("factory-change payload workstations = %#v, want edited plan-task topology", payload.Factory.Workstations)
	}
}

func TestCurrentFactoryPUT_SaveDefaultFactoryDefinitionPersistsAndRunsReplacement(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)

	current := getCurrentFactory(t, server.URL())
	if current.Name != "UNDEFINED" {
		t.Fatalf("default current factory name = %q, want UNDEFINED", current.Name)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("default current factory id = %#v, want root-runtime", current.Id)
	}
	if current.Version == nil {
		t.Fatal("default current factory version = nil, want version metadata for save")
	}

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		functionalDefaultFactorySaveBody("root-runtime", "story", advancedFactoryVersion(t, current.Version)),
	)
	if saved.Name != "UNDEFINED" {
		t.Fatalf("saved default factory name = %q, want UNDEFINED", saved.Name)
	}
	if saved.WorkTypes == nil || len(*saved.WorkTypes) != 1 || (*saved.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved default factory work types = %#v, want story", saved.WorkTypes)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Name != "UNDEFINED" {
		t.Fatalf("reloaded default factory name = %q, want UNDEFINED", reloaded.Name)
	}
	if reloaded.WorkTypes == nil || len(*reloaded.WorkTypes) != 1 || (*reloaded.WorkTypes)[0].Name != "story" {
		t.Fatalf("reloaded default factory work types = %#v, want story", reloaded.WorkTypes)
	}
	if _, err := config.ReadCurrentFactoryPointer(rootDir); !os.IsNotExist(err) {
		t.Fatalf("default factory save current pointer error = %v, want missing pointer", err)
	}

	storyResp := submitWorkAndExpectStatus(t, server.URL(), "story", "saved-default", http.StatusCreated)
	var storySubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, storyResp, &storySubmit, "decode story submit response")
	if storySubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for saved default factory submission")
	}
	submitWorkAndExpectStatus(t, server.URL(), "root-task", "old-default", http.StatusBadRequest)
}

func TestCurrentFactoryPUT_DefaultFactoryAcceptsFullCurrentFactoryReadbackDocument(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	assertRawCurrentFactoryLogicalVersionIsString(t, server.URL())
	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal full current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode full current factory document: %v", err)
	}
	document["version"] = versionDocument(advancedFactoryVersion(t, current.Version))
	document["workTypes"] = []map[string]any{{
		"name": "story",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
	document["workers"] = []map[string]any{{
		"body":             "Plan work.",
		"executorProvider": "SCRIPT_WRAP",
		"model":            "claude-sonnet-4-20250514",
		"modelProvider":    "CLAUDE",
		"name":             "planner",
		"type":             "MODEL_WORKER",
	}}
	document["workstations"] = []map[string]any{{
		"behavior": "STANDARD",
		"body":     "Plan the story.",
		"inputs":   []map[string]string{{"state": "init", "workType": "story"}},
		"name":     "plan-task",
		"outputs":  []map[string]string{{"state": "done", "workType": "story"}},
		"type":     "MODEL_WORKSTATION",
		"worker":   "planner",
	}}
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal edited full current factory document: %v", err)
	}

	saved := saveCurrentFactoryDefinition(t, server.URL(), string(body))
	assertFactoryWorkType(t, saved, "story", "saved full readback document")

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, "story", "reloaded full readback document")
	submitWorkAndExpectStatus(t, server.URL(), "story", "full-readback-save", http.StatusCreated)
}

func TestCurrentFactoryPUT_DefaultFactoryMaterializesBundledFilesAndReturns(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal full current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode full current factory document: %v", err)
	}
	document["version"] = versionDocument(advancedFactoryVersion(t, current.Version))
	document["workTypes"] = []map[string]any{{
		"name": "story",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
	document["workers"] = []map[string]any{{
		"body":             "Plan work.",
		"executorProvider": "SCRIPT_WRAP",
		"model":            "claude-sonnet-4-20250514",
		"modelProvider":    "CLAUDE",
		"name":             "planner",
		"type":             "MODEL_WORKER",
	}}
	document["workstations"] = []map[string]any{{
		"behavior": "STANDARD",
		"body":     "Plan the story.",
		"inputs":   []map[string]string{{"state": "init", "workType": "story"}},
		"name":     "plan-task",
		"outputs":  []map[string]string{{"state": "done", "workType": "story"}},
		"type":     "MODEL_WORKSTATION",
		"worker":   "planner",
	}}
	document["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "ROOT_HELPER",
				"targetPath": "Makefile",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "test:\n\tgo test ./...\n",
				},
			},
			{
				"type":       "SCRIPT",
				"targetPath": "factory/scripts/setup-workspace.py",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "print('portable script')\n",
				},
			},
		},
	}
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal edited default factory document with bundled files: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	saved := saveCurrentFactoryDefinitionWithClient(t, client, server.URL(), string(body))
	assertFactoryWorkType(t, saved, "story", "saved bundled default factory")
	assertPortableFile(t, filepath.Join(rootDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(rootDir, "scripts", "setup-workspace.py"), "print('portable script')\n")
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(rootDir, interfaces.FactoryConfigFile))

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, "story", "reloaded bundled default factory")
	submitWorkAndExpectStatus(t, server.URL(), "story", "bundled-default-save", http.StatusCreated)
}

func TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	if _, err := config.PersistNamedFactory(rootDir, "beta", functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	betaSessionID := openNamedFactorySession(t, server.URL(), rootDir, "beta")

	sessionCurrent := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	if sessionCurrent.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("session current factory name = %q, want beta", sessionCurrent.Name)
	}
	assertFactoryWorkType(t, sessionCurrent, "beta-task", "session current factory before save")

	saved := saveCurrentFactoryForSession(t, server.URL(), betaSessionID, functionalNamedFactoryBody("beta", "story", advancedFactoryVersion(t, sessionCurrent.Version)))
	if saved.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("saved session factory name = %q, want beta", saved.Name)
	}
	assertFactoryWorkType(t, saved, "story", "saved session factory")

	reloaded := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	if reloaded.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("reloaded session factory name = %q, want beta", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "story", "reloaded session factory")

	defaultCurrent := getCurrentFactory(t, server.URL())
	if defaultCurrent.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("default current factory name = %q, want alpha", defaultCurrent.Name)
	}
	assertFactoryWorkType(t, defaultCurrent, "alpha-task", "default current factory after session save")
	assertCurrentFactoryPointer(t, rootDir, "alpha")

	storyResp := submitWorkForSessionAndExpectStatus(t, server.URL(), betaSessionID, "story", "session-story", http.StatusCreated)
	var storySubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, storyResp, &storySubmit, "decode session story submit response")
	if storySubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for session-scoped transformed factory submission")
	}
	submitWorkForSessionAndExpectStatus(t, server.URL(), betaSessionID, "beta-task", "old-session-work", http.StatusBadRequest)
	submitWorkAndExpectStatus(t, server.URL(), "alpha-task", "default-still-alpha", http.StatusCreated)
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
	if errResp.Code != factoryapi.INVALIDFACTORY {
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
	if errResp.Code != factoryapi.INVALIDFACTORY {
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

func TestCurrentFactoryPUT_RequiresAdvancedSaveVersion(t *testing.T) {
	for _, tc := range advancedSaveVersionCases() {
		t.Run(tc.name, func(t *testing.T) {
			runAdvancedSaveVersionCase(t, tc)
		})
	}
}

type advancedSaveVersionCase struct {
	name      string
	version   func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any
	wantCode  factoryapi.ErrorResponseCode
	wantState string
}

func runAdvancedSaveVersionCase(t *testing.T, tc advancedSaveVersionCase) {
	t.Helper()
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "task")
	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata")
	}

	body := currentFactorySaveDocument(t, "alpha", "story", tc.version(t, *current.Version))
	if tc.wantCode != "" {
		resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusConflict)
		var errResp factoryapi.ErrorResponse
		decodeJSONResponse(t, resp, &errResp, "decode stale current factory save response")
		if errResp.Code != tc.wantCode {
			t.Fatalf("error code = %q, want %q", errResp.Code, tc.wantCode)
		}
	} else {
		saved := saveCurrentFactoryDefinition(t, server.URL(), body)
		assertFactoryWorkType(t, saved, "story", "saved current factory")
	}

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, tc.wantState, "reloaded current factory after version save")
}

func advancedSaveVersionCases() []advancedSaveVersionCase {
	return []advancedSaveVersionCase{
		staleVersionCase("lower logical lower physical fails", -1, -time.Nanosecond),
		staleVersionCase("same logical equal physical fails", 0, 0),
		staleVersionCase("lower logical greater physical fails", -1, time.Second),
		staleVersionCase("same logical greater physical fails", 0, time.Second),
		missingVersionCase(),
		missingLogicalVersionCase(),
		missingPhysicalVersionCase(),
		advancedVersionPassCase(),
	}
}

func staleVersionCase(name string, logicalDelta int64, physicalDelta time.Duration) advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: name,
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return versionDocument(factoryapi.HybridLogicalTimestamp{
				Logical:  apitypes.Int64String(current.Logical.Int64() + logicalDelta),
				Physical: current.Physical.Add(physicalDelta),
			})
		},
		wantCode:  factoryapi.STALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing version fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return nil
		},
		wantCode:  factoryapi.STALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingLogicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing logical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"physical": current.Physical.Add(time.Second).UTC().Format(time.RFC3339Nano)}
		},
		wantCode:  factoryapi.STALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingPhysicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing physical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"logical": strconv.FormatInt(current.Logical.Int64()+1, 10)}
		},
		wantCode:  factoryapi.STALEFACTORYVERSION,
		wantState: "task",
	}
}

func advancedVersionPassCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "greater logical and physical passes",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return versionDocument(advancedFactoryVersion(t, &current))
		},
		wantState: "story",
	}
}

func startFactoryTransformationServer(t *testing.T, rootDir string) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.Logger = zap.NewNop()
		},
	})
}

func seedNamedFactoryRoot(t *testing.T, rootDir, name, workType string) {
	t.Helper()
	if _, err := config.PersistNamedFactory(rootDir, name, functionalNamedFactoryPayloadWithWorkType(t, name, workType)); err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(%s): %v", name, err)
	}
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

func assertRawCurrentFactoryLogicalVersionIsString(t *testing.T, serverURL string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", resp.StatusCode)
	}
	var document map[string]any
	decodeJSONResponse(t, resp, &document, "decode raw current factory response")
	version, ok := document["version"].(map[string]any)
	if !ok {
		t.Fatalf("raw current factory version = %#v, want object", document["version"])
	}
	if _, ok := version["logical"].(string); !ok {
		t.Fatalf("raw current factory version.logical = %#v, want string", version["logical"])
	}
}

func saveCurrentFactoryDefinition(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatus(t, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func saveCurrentFactoryDefinitionWithClient(t *testing.T, client *http.Client, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatusWithClient(t, client, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func saveCurrentFactoryDefinitionExpectStatus(t *testing.T, serverURL, body string, wantStatus int) *http.Response {
	t.Helper()
	return saveCurrentFactoryDefinitionExpectStatusWithClient(t, http.DefaultClient, serverURL, body, wantStatus)
}

func saveCurrentFactoryDefinitionExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, serverURL+"/factory-sessions/~default/factory", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new current factory request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("PUT /factory-sessions/~default/factory status = %d, want %d", resp.StatusCode, wantStatus)
	}
	return resp
}

func openNamedFactorySession(t *testing.T, serverURL, folderPath, name string) string {
	t.Helper()
	body, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: &name,
		},
	})
	if err != nil {
		t.Fatalf("marshal open factory session request: %v", err)
	}
	resp, err := http.Post(serverURL+"/factory-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("POST /factory-sessions status = %d, want 200", resp.StatusCode)
	}
	var opened factoryapi.OpenFactorySessionResponse
	decodeJSONResponse(t, resp, &opened, "decode open factory session response")
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open factory session response = %#v, want session id", opened)
	}
	return opened.Session.Id
}

func getCurrentFactoryForSession(t *testing.T, serverURL, sessionID string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Get(sessionFactoryURL(serverURL, sessionID))
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/factory: %v", sessionID, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/%s/factory status = %d, want 200", sessionID, resp.StatusCode)
	}
	var current factoryapi.Factory
	decodeJSONResponse(t, resp, &current, "decode session current factory response")
	return current
}

func saveCurrentFactoryForSession(t *testing.T, serverURL, sessionID, body string) factoryapi.Factory {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, sessionFactoryURL(serverURL, sessionID), bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new session current factory request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/%s/factory: %v", sessionID, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("PUT /factory-sessions/%s/factory status = %d, want 200", sessionID, resp.StatusCode)
	}
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode session current factory save response")
	return saved
}

func sessionFactoryURL(serverURL, sessionID string) string {
	return serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewBufferString(`{"name":"factory-transformation-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func submitWorkForSessionAndExpectStatus(t *testing.T, serverURL, sessionID, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	endpoint := serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	resp, err := http.Post(endpoint, "application/json", bytes.NewBufferString(`{"name":"factory-transformation-session-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/work %s: %v", sessionID, workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /factory-sessions/%s/work %s status = %d, want %d", sessionID, workType, resp.StatusCode, wantStatus)
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

func functionalDefaultFactorySaveBody(id, workType string, version factoryapi.HybridLogicalTimestamp) string {
	return `{
		"name":"UNDEFINED",
		"id":"` + id + `",
		"version":{"physical":"` + version.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(version.Logical.Int64(), 10) + `"},
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
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
