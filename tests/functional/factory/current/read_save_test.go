package current

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestAPIGetAndSaveCurrentFactoryWithinOneSession proves that a Factory Session
// can read its Current Factory, save a valid updated definition through the
// public session API, and read back the saved customer-visible topology within
// the same session.
func TestAPIGetAndSaveCurrentFactoryWithinOneSession(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-task", "initial current factory")

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)),
	)
	assertFactoryWorkType(t, saved, "story", "save response")

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("reloaded current factory name = %q, want alpha", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "story", "subsequent get within session")
}

// TestAPISaveCurrentFactoryValidatesBeforePersistence proves that an invalid Current
// Factory save is rejected through the public session API before persistence, returns
// a structured validation error, and leaves the prior Current Factory unchanged on
// subsequent readback within the same session.
func TestAPISaveCurrentFactoryValidatesBeforePersistence(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}
	assertFactoryWorkType(t, current, "alpha-task", "initial current factory")

	advanced := advancedFactoryVersion(t, current.Version)
	body := `{
		"name":"alpha",
		"version":{"physical":"` + advanced.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(advanced.Logical.Int64(), 10) + `"},
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
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) == 0 {
		t.Fatalf("error targets = %#v, want topology validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want dangling place reference target", errResp.Targets)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Version == nil || *reloaded.Version != *current.Version {
		t.Fatalf("reloaded version = %#v, want unchanged %#v", reloaded.Version, current.Version)
	}
	assertFactoryWorkType(t, reloaded, "alpha-task", "current factory after rejected save")
}

// TestAPICurrentFactoriesRemainSessionScoped proves that Current Factory saves stay
// isolated across Factory Sessions so a valid save in one session updates that
// session's readback while another session's Current Factory remains unchanged.
func TestAPICurrentFactoriesRemainSessionScoped(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	betaSessionID := openNamedFactorySession(t, server.URL(), rootDir, "beta")

	sessionCurrent := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	if sessionCurrent.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("session current factory name = %q, want beta", sessionCurrent.Name)
	}
	assertFactoryWorkType(t, sessionCurrent, "beta-task", "beta session current factory before save")

	saved := saveCurrentFactoryForSession(
		t,
		server.URL(),
		betaSessionID,
		functionalNamedFactoryBody("beta", "story", advancedFactoryVersion(t, sessionCurrent.Version)),
	)
	assertFactoryWorkType(t, saved, "story", "beta session save response")

	reloaded := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	assertFactoryWorkType(t, reloaded, "story", "beta session current factory after save")

	defaultCurrent := getCurrentFactory(t, server.URL())
	if defaultCurrent.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("default current factory name = %q, want alpha", defaultCurrent.Name)
	}
	assertFactoryWorkType(t, defaultCurrent, "alpha-task", "default session current factory after beta session save")
}

func TestCurrentFactoryEvents_InitialStructureIncludesBundledFileContent(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		currentFactoryEventDocumentWithBundledFiles(
			t,
			"root-runtime",
			"story",
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
		0o644,
	); err != nil {
		t.Fatalf("write factory config with bundled files: %v", err)
	}

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	payload := requireInitialStructurePayload(t, server.GetFactoryEvents(t))
	assertDocBundledFileInline(t, payload.Factory, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, payload.Factory, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_DocsCreateEditRenameDeleteRoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	created := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, created, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, created, "factory/scripts/setup-workspace.py", "print('setup')\n")

	alphaDir := filepath.Join(rootDir, "alpha")
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "overview.md"), "# Overview\n")
	assertPortableFile(t, filepath.Join(alphaDir, "scripts", "setup-workspace.py"), "print('setup')\n")

	afterCreate := getCurrentFactory(t, server.URL())
	edited := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterCreate,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview updated\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, edited, "factory/docs/overview.md", "# Overview updated\n")
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "overview.md"), "# Overview updated\n")

	afterEdit := getCurrentFactory(t, server.URL())
	renamed := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterEdit,
			[]map[string]any{
				docBundledFileEntry("factory/docs/guide.md", "# Overview updated\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, renamed, "factory/docs/guide.md", "# Overview updated\n")
	if _, err := os.Stat(filepath.Join(alphaDir, "docs", "overview.md")); !os.IsNotExist(err) {
		t.Fatalf("renamed-away doc stat error = %v, want not exist", err)
	}
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "guide.md"), "# Overview updated\n")

	afterRename := getCurrentFactory(t, server.URL())
	deleted := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterRename,
			[]map[string]any{
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	if findBundledFile(t, deleted, "factory/docs/guide.md") != nil {
		t.Fatalf("deleted doc still present in save response: %#v", deleted.SupportingFiles)
	}
	if _, err := os.Stat(filepath.Join(alphaDir, "docs", "guide.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted doc stat error = %v, want not exist", err)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if findBundledFile(t, reloaded, "factory/docs/guide.md") != nil {
		t.Fatalf("deleted doc still present after reload: %#v", reloaded.SupportingFiles)
	}
	assertScriptBundledFileInline(t, reloaded, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_DocsSaveEmitsFactoryChangeWithBundledFilesAndVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	initialEvents := server.GetFactoryEvents(t)
	current := getCurrentFactory(t, server.URL())

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	payload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	assertFactoryChangeVersion(t, payload.Factory, saved)
	assertDocBundledFileInline(t, payload.Factory, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, payload.Factory, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_RejectsInvalidDocTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	cases := []struct {
		name       string
		targetPath string
	}{
		{name: "outside docs root", targetPath: "factory/scripts/readme.md"},
		{name: "non canonical path", targetPath: "factory/docs/./notes.md"},
		{name: "escaping path", targetPath: "factory/docs/../secrets.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := saveCurrentFactoryDefinitionExpectStatus(
				t,
				server.URL(),
				currentFactoryDocumentWithBundledDocs(
					t,
					current,
					[]map[string]any{
						docBundledFileEntry(tc.targetPath, "invalid target\n"),
					},
				),
				http.StatusBadRequest,
			)
			resp.Body.Close()
		})
	}
}

func TestCurrentFactoryPUT_RejectsDuplicateDocTargetPaths(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	resp := saveCurrentFactoryDefinitionExpectStatus(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "first\n"),
				docBundledFileEntry("factory/docs/overview.md", "second\n"),
			},
		),
		http.StatusBadRequest,
	)
	resp.Body.Close()
}

func TestCurrentFactoryPUT_ShellEscapedBundledInlineReplayReturnsPayloadInvalid(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	body := `{
		"mode":"REPLACE_CURRENT",
		"factory":{
			"name":"alpha",
			"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
			"workTypes":[{"name":"alpha-task","states":[
				{"name":"init","type":"INITIAL"},
				{"name":"complete","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]}],
			"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514","body":"Plan work."}],
			"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"alpha-task","state":"init"}],"outputs":[{"workType":"alpha-task","state":"complete"}]}],
			"supportingFiles":{"bundledFiles":[
				{"type":"SCRIPT","targetPath":"factory/scripts/setup-workspace.py","content":{"encoding":"utf-8","inline":"print(\'setup\')\n"}}
			]}
		}
	}`

	req, err := http.NewRequest(http.MethodPut, server.URL()+"/factory-sessions/~default/factory", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new malformed current factory save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("PUT /factory-sessions/~default/factory status = %d, want 400", resp.StatusCode)
	}

	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode malformed bundled inline save response")
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want BAD_REQUEST", errResp.Code)
	}
	if errResp.Targets == nil || !hasValidationTargetCode(*errResp.Targets, factoryValidationCodePayloadInvalid) {
		t.Fatalf("error targets = %#v, want factory.payload.invalid decode target", errResp.Targets)
	}
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

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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
	assertPersistedFactoryJSONStripsInlineBundledContent(
		t,
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		"test:\n\tgo test ./...",
		"print('portable script')",
	)

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, "story", "reloaded bundled default factory")
	submitWorkAndExpectStatus(t, server.URL(), "story", "bundled-default-save", http.StatusCreated)
}

func TestCurrentFactoryPUT_FactoryChangeVersionsAdvanceOnEverySave(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	firstSaved := saveCurrentFactoryDefinition(t, server.URL(), functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)))
	firstChange := requireLatestFactoryChange(t, server.GetFactoryEvents(t))
	firstPayload, err := firstChange.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode first factory-change payload: %v", err)
	}
	assertFactoryChangeVersion(t, firstPayload.Factory, firstSaved)
	if firstPayload.Factory.Version.Logical != current.Version.Logical+1 {
		t.Fatalf("first factory-change logical version = %d, want %d", firstPayload.Factory.Version.Logical, current.Version.Logical+1)
	}

	secondSaved := saveCurrentFactoryDefinition(t, server.URL(), functionalNamedFactoryBody("alpha", "article", advancedFactoryVersion(t, firstSaved.Version)))
	secondChange := requireLatestFactoryChange(t, server.GetFactoryEvents(t))
	secondPayload, err := secondChange.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode second factory-change payload: %v", err)
	}
	assertFactoryChangeVersion(t, secondPayload.Factory, secondSaved)
	if secondPayload.Factory.Version.Logical != firstPayload.Factory.Version.Logical+1 {
		t.Fatalf("second factory-change logical version = %d, want %d", secondPayload.Factory.Version.Logical, firstPayload.Factory.Version.Logical+1)
	}
}

func TestCurrentFactoryPUT_RequiresAdvancedSaveVersion(t *testing.T) {
	for _, tc := range advancedSaveVersionCases() {
		t.Run(tc.name, func(t *testing.T) {
			runAdvancedSaveVersionCase(t, tc)
		})
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

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

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
	storyResp := submitWorkAndExpectStatus(t, server.URL(), "story", "saved-default", http.StatusCreated)
	var storySubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, storyResp, &storySubmit, "decode story submit response")
	if storySubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for saved default factory submission")
	}
	submitWorkAndExpectStatus(t, server.URL(), "root-task", "old-default", http.StatusBadRequest)

	assertFunctionalSplitLayoutAtRoot(t, rootDir, "root-runtime")
}

func TestCurrentFactoryPUT_SaveEditableCurrentFactoryDefinitionEmitsCanonicalFactoryChangeEvent(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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

func TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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
	storyResp := submitWorkForSessionAndExpectStatus(t, server.URL(), betaSessionID, "story", "session-story", http.StatusCreated)
	var storySubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, storyResp, &storySubmit, "decode session story submit response")
	if storySubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for session-scoped transformed factory submission")
	}
	submitWorkForSessionAndExpectStatus(t, server.URL(), betaSessionID, "beta-task", "old-session-work", http.StatusBadRequest)
	submitWorkAndExpectStatus(t, server.URL(), "alpha-task", "default-still-alpha", http.StatusCreated)
}

func TestCurrentFactoryPUT_NonDefaultSessionImportIsolatesDefaultFactoryAndMaterializesBundledFiles(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	betaSessionID := openNamedFactorySession(t, server.URL(), rootDir, "beta")

	defaultBefore := getCurrentFactory(t, server.URL())
	alphaConfigPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	alphaConfigBefore, err := os.ReadFile(alphaConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", alphaConfigPath, err)
	}

	sessionCurrent := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	if sessionCurrent.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("session current factory name = %q, want beta", sessionCurrent.Name)
	}
	importBody := nonDefaultSessionImportBodyWithBundledFiles(t, sessionCurrent, "imported-task")

	saved := saveCurrentFactoryForSession(t, server.URL(), betaSessionID, importBody)
	if saved.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("saved session import factory name = %q, want beta", saved.Name)
	}
	assertFactoryWorkType(t, saved, "imported-task", "saved non-default session import")

	reloaded := getCurrentFactoryForSession(t, server.URL(), betaSessionID)
	if reloaded.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("reloaded session factory name = %q, want beta", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "imported-task", "session GET after non-default import")

	defaultAfter := getCurrentFactory(t, server.URL())
	if defaultAfter.Name != defaultBefore.Name {
		t.Fatalf("default current factory name = %q, want %q", defaultAfter.Name, defaultBefore.Name)
	}
	assertFactoryWorkType(t, defaultAfter, "alpha-task", "default session after non-default import")
	alphaConfigAfter, err := os.ReadFile(alphaConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", alphaConfigPath, err)
	}
	if !bytes.Equal(alphaConfigBefore, alphaConfigAfter) {
		t.Fatalf("alpha on-disk factory config changed after non-default session import")
	}

	betaFactoryDir := filepath.Join(rootDir, "beta")
	assertPortableFile(t, filepath.Join(betaFactoryDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(betaFactoryDir, "docs", "README.md"), "# Session import factory\n")
	assertPortableFile(
		t,
		filepath.Join(betaFactoryDir, "scripts", "session-import.py"),
		"print('session import script')\n",
	)
	assertPersistedFactoryJSONStripsInlineBundledContent(
		t,
		filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile),
		"# Session import factory",
		"print('session import script')",
	)
	persistedBetaConfig, err := os.ReadFile(filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile), err)
	}
	for _, forbidden := range []string{"# Session import factory", "print('session import script')"} {
		if bytes.Contains(persistedBetaConfig, []byte(forbidden)) {
			t.Fatalf("persisted beta factory still contains inline portable content %q", forbidden)
		}
	}

	alphaMakefile := filepath.Join(rootDir, "alpha", "Makefile")
	if _, err := os.Stat(alphaMakefile); err == nil {
		t.Fatalf("default session factory directory %q should not contain imported Makefile", alphaMakefile)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s): %v", alphaMakefile, err)
	}

	submitWorkForSessionAndExpectStatus(t, server.URL(), betaSessionID, "imported-task", "session-import-submit", http.StatusCreated)
	submitWorkAndExpectStatus(t, server.URL(), "alpha-task", "default-still-alpha-after-import", http.StatusCreated)
}

func TestSessionFactoryPUT_UpsertCreateAllowsOmittedVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("upsert create response version = nil, want assigned version metadata")
	}
	if created.Version.Logical.Int64() < 1 {
		t.Fatalf("upsert create version logical = %d, want >= 1", created.Version.Logical.Int64())
	}
	assertFactoryWorkType(t, created, "beta-task", "upsert create response")
}

func TestSessionFactoryPUT_UpsertReplaceRejectsStaleVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	staleBody := upsertNamedFactoryRequestBody(currentFactorySaveDocument(t, "beta", "beta-task", versionDocument(*created.Version)))
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		server.URL(),
		"/factory-sessions/~default/factory",
		staleBody,
		http.StatusConflict,
	)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode stale upsert replace response")
	if errResp.Code != factoryapi.ErrorResponseCodeSTALEFACTORYVERSION {
		t.Fatalf("error code = %q, want STALE_FACTORY_VERSION", errResp.Code)
	}

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, "beta-task", "unchanged factory after stale upsert replace")
}

func TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	freshVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced := upsertNamedFactoryFromBody(
		t,
		server.URL(),
		currentFactorySaveDocument(t, "beta", "beta-replaced", versionDocument(freshVersion)),
	)
	if replaced.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("replaced factory name = %q, want beta", replaced.Name)
	}
	assertFactoryWorkType(t, replaced, "beta-replaced", "upsert replace response")
	if replaced.Version == nil || replaced.Version.Logical <= created.Version.Logical {
		t.Fatalf("replaced version = %#v, want logical > %#v", replaced.Version, created.Version.Logical)
	}
	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
}
