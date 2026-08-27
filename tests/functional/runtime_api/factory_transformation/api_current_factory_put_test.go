// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
package factory_transformation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// sharedFactoryTransformationFixture owns the one package-scoped application
// process used by the runtime-transformation scenarios. Scenario roots and
// session identities remain per-test; only immutable process wiring and the
// HTTP transport are shared.
type sharedFactoryTransformationFixture struct {
	baseURL        string
	factoryDir     string
	homeDir        string
	cancel         context.CancelFunc
	done           chan error
	process        support.ApplicationProcess
	providerRunner *support.ShapedProviderCommandRunner
	scriptRunner   *support.RecordingCommandRunner

	mu              sync.Mutex
	seenSessionIDs  map[string]struct{}
	processStartLog sync.Once
}

var factoryTransformationFixture *sharedFactoryTransformationFixture

func TestMain(m *testing.M) {
	fixture, err := startSharedFactoryTransformationFixture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start shared factory transformation server: %v\n", err)
		os.Exit(1)
	}
	factoryTransformationFixture = fixture

	exitCode := m.Run()
	fixture.stop()
	os.Exit(exitCode)
}

func startSharedFactoryTransformationFixture() (*sharedFactoryTransformationFixture, error) {
	ctx, cancel := context.WithCancel(context.Background())
	fixture := &sharedFactoryTransformationFixture{
		cancel:         cancel,
		seenSessionIDs: make(map[string]struct{}),
	}
	cleanup := func() {
		cancel()
		if fixture.done != nil {
			select {
			case <-fixture.done:
			case <-time.After(5 * time.Second):
			}
		}
		if fixture.process != nil {
			closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
			_ = fixture.process.Close(closeCtx)
			cancelClose()
		}
		if fixture.factoryDir != "" {
			_ = os.RemoveAll(fixture.factoryDir)
		}
		if fixture.homeDir != "" {
			_ = os.RemoveAll(fixture.homeDir)
		}
	}

	var err error
	fixture.factoryDir, err = os.MkdirTemp("", "factory-transformation-shared-root-")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create shared Factory root: %w", err)
	}
	fixture.homeDir, err = os.MkdirTemp("", "factory-transformation-shared-home-")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create shared operator home: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(fixture.factoryDir, interfaces.FactoryConfigFile),
		[]byte(functionalNamedFactoryPayloadJSON("shared-runtime", "shared-task")),
		0o644,
	); err != nil {
		cleanup()
		return nil, fmt.Errorf("write shared Factory config: %w", err)
	}

	api := support.NewProcessAPIServer()
	providerRunner := support.NewShapedProviderCommandRunner()
	scriptRunner := support.NewRecordingCommandRunner("factory-transformation-script-output")
	fixture.providerRunner = providerRunner
	fixture.scriptRunner = scriptRunner
	fixture.process, err = support.BuildProcessWithContext(ctx, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: providerRunner,
		ScriptCommandRunner:   scriptRunner,
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build shared root process: %w", err)
	}
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--continuously", "--with-server", "--quiet",
		"--dir", fixture.factoryDir, "--no-record",
	})
	inputs.Input.Env = []string{
		"HOME=" + fixture.homeDir,
		"USERPROFILE=" + fixture.homeDir,
	}
	inputs.Input.WorkingDirectory = fixture.factoryDir
	fixture.done = make(chan error, 1)
	go func() { fixture.done <- fixture.process.Execute(inputs.Input) }()

	fixture.baseURL, err = api.WaitForBaseURL(15 * time.Second)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("wait for shared server base URL: %w", err)
	}
	return fixture, nil
}

func (fixture *sharedFactoryTransformationFixture) stop() {
	if fixture == nil {
		return
	}
	if fixture.providerRunner != nil && fixture.scriptRunner != nil {
		fmt.Fprintf(
			os.Stdout,
			"FULL-003: controlled worker edge calls provider=%d script=%d\n",
			fixture.providerRunner.CallCount(),
			fixture.scriptRunner.CallCount(),
		)
	}
	fixture.cancel()
	if fixture.done != nil {
		select {
		case <-fixture.done:
		case <-time.After(5 * time.Second):
			fmt.Fprintln(os.Stderr, "timed out waiting for shared factory transformation process")
		}
	}
	if fixture.process != nil {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		if err := fixture.process.Close(closeCtx); err != nil {
			fmt.Fprintf(os.Stderr, "close shared factory transformation process: %v\n", err)
		}
		cancelClose()
	}
	if err := os.RemoveAll(fixture.factoryDir); err != nil {
		fmt.Fprintf(os.Stderr, "remove shared Factory root %s: %v\n", fixture.factoryDir, err)
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		fmt.Fprintf(os.Stderr, "remove shared operator home %s: %v\n", fixture.homeDir, err)
	}
}

type documentTransformationServer struct {
	baseURL   string
	rootDir   string
	sessionID string
	closeOnce sync.Once
}

func startDocumentTransformationServer(
	t *testing.T,
	rootDir string,
	targetName string,
) *documentTransformationServer {
	t.Helper()
	fixture := factoryTransformationFixture
	if fixture == nil {
		t.Fatal("shared factory transformation fixture is unavailable")
	}
	sessionID := openFactoryTransformationSession(t, fixture.baseURL, rootDir, targetName)
	fixture.mu.Lock()
	if _, exists := fixture.seenSessionIDs[sessionID]; exists {
		fixture.mu.Unlock()
		t.Fatalf("Factory Session ID %q was reused", sessionID)
	}
	fixture.seenSessionIDs[sessionID] = struct{}{}
	fixture.mu.Unlock()
	fixture.processStartLog.Do(func() {
		t.Logf(
			"FULL-003: package process start count=1; controlled worker edges provider=%T script=%T",
			fixture.providerRunner,
			fixture.scriptRunner,
		)
	})
	t.Logf("DOC-001: explicit Factory Session ID=%s target=%q", sessionID, targetName)

	server := &documentTransformationServer{
		baseURL:   fixture.baseURL,
		rootDir:   rootDir,
		sessionID: sessionID,
	}
	t.Logf("LAYOUT-002: Factory root=%s session=%s", rootDir, sessionID)
	t.Cleanup(func() {
		server.close(t)
	})
	return server
}

func (server *documentTransformationServer) close(t testing.TB) {
	t.Helper()
	server.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, server.baseURL, server.sessionID)
		t.Logf("DOC-001: closed explicit Factory Session ID=%s", server.sessionID)
	})
}

func (server *documentTransformationServer) URL() string {
	if server == nil {
		return ""
	}
	return server.baseURL
}

func (server *documentTransformationServer) SessionID() string {
	if server == nil {
		return ""
	}
	return server.sessionID
}

func (server *documentTransformationServer) RootDir() string {
	if server == nil {
		return ""
	}
	return server.rootDir
}

func (server *documentTransformationServer) FactoryURL() string {
	if server == nil {
		return ""
	}
	return sessionFactoryURL(server.baseURL, server.sessionID)
}

func (server *documentTransformationServer) GetFactoryEvents(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, server.baseURL, server.sessionID)
}

func seedDocumentNamedFactoryRoot(t *testing.T, rootDir, name, workType string) {
	t.Helper()
	fixture := factoryTransformationFixture
	if fixture == nil {
		t.Fatal("shared factory transformation fixture is unavailable")
	}
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, functionalNamedFactoryPayloadWithWorkType(t, name, workType), 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	env := []string{"HOME=" + fixture.homeDir, "USERPROFILE=" + fixture.homeDir}
	support.CreateAndActivateNamedFactoryAtRootWithProcess(
		t,
		fixture.process,
		env,
		sourceDir,
		rootDir,
		name,
		sourcePath,
	)
}

func openFactoryTransformationSession(t *testing.T, serverURL, folderPath, targetName string) string {
	t.Helper()
	var target *factoryapi.FactorySessionTargetRef
	if targetName != "" {
		target = &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: &targetName,
		}
	} else {
		target = &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		}
	}
	body, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target:     target,
	})
	if err != nil {
		t.Fatalf("marshal open Factory Session request: %v", err)
	}
	resp, err := http.Post(serverURL+"/factory-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST /factory-sessions status = %d, want 200: %s", resp.StatusCode, payload)
	}
	var opened factoryapi.OpenFactorySessionResponse
	decodeJSONResponse(t, resp, &opened, "decode open Factory Session response")
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open Factory Session response = %#v, want session id", opened)
	}
	if opened.Session.Id == "~default" || opened.Session.IsDefault {
		t.Fatalf("opened document Factory Session = %#v, want explicit non-default session", opened.Session)
	}
	wantKind := factoryapi.FactorySessionTargetRefKindDefault
	if targetName != "" {
		wantKind = factoryapi.FactorySessionTargetRefKindNamed
	}
	if opened.Session.Target.Kind != wantKind {
		t.Fatalf("opened Factory Session target kind = %q, want %q", opened.Session.Target.Kind, wantKind)
	}
	return opened.Session.Id
}

func TestCurrentFactoryPUT_SaveEditableCurrentFactoryDefinitionEmitsCanonicalFactoryChangeEvent(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startDocumentTransformationServer(t, rootDir, "alpha")
	initialEvents := server.GetFactoryEvents(t)

	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	saved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)))
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

func TestCurrentFactoryPUT_FactoryChangeVersionsAdvanceOnEverySave(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startDocumentTransformationServer(t, rootDir, "alpha")

	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	firstSaved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)))
	firstChange := requireLatestFactoryChange(t, server.GetFactoryEvents(t))
	firstPayload, err := firstChange.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode first factory-change payload: %v", err)
	}
	assertFactoryChangeVersion(t, firstPayload.Factory, firstSaved)
	if firstPayload.Factory.Version.Logical != current.Version.Logical+1 {
		t.Fatalf("first factory-change logical version = %d, want %d", firstPayload.Factory.Version.Logical, current.Version.Logical+1)
	}

	secondSaved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("alpha", "article", advancedFactoryVersion(t, firstSaved.Version)))
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

func TestCurrentFactoryPUT_SaveDefaultFactoryDefinitionPersistsAndRunsReplacement(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startDocumentTransformationServer(t, rootDir, "")

	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if current.Name != "UNDEFINED" {
		t.Fatalf("default current factory name = %q, want UNDEFINED", current.Name)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("default current factory id = %#v, want root-runtime", current.Id)
	}
	if current.Version == nil {
		t.Fatal("default current factory version = nil, want version metadata for save")
	}

	saved := saveCurrentFactoryForSession(
		t,
		server.URL(),
		server.SessionID(),
		functionalDefaultFactorySaveBody("root-runtime", "story", advancedFactoryVersion(t, current.Version)),
	)
	if saved.Name != "UNDEFINED" {
		t.Fatalf("saved default factory name = %q, want UNDEFINED", saved.Name)
	}
	if saved.WorkTypes == nil || len(*saved.WorkTypes) != 1 || (*saved.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved default factory work types = %#v, want story", saved.WorkTypes)
	}

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if reloaded.Name != "UNDEFINED" {
		t.Fatalf("reloaded default factory name = %q, want UNDEFINED", reloaded.Name)
	}
	if reloaded.WorkTypes == nil || len(*reloaded.WorkTypes) != 1 || (*reloaded.WorkTypes)[0].Name != "story" {
		t.Fatalf("reloaded default factory work types = %#v, want story", reloaded.WorkTypes)
	}
	storyResp := submitWorkForSessionAndExpectStatus(t, server.URL(), server.SessionID(), "story", "saved-default", http.StatusCreated)
	var storySubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, storyResp, &storySubmit, "decode story submit response")
	if storySubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for saved default factory submission")
	}
	submitWorkForSessionAndExpectStatus(t, server.URL(), server.SessionID(), "root-task", "old-default", http.StatusBadRequest)

	assertFunctionalSplitLayoutAtRoot(t, rootDir, "root-runtime")
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

	server := startDocumentTransformationServer(t, rootDir, "")
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	assertRawCurrentFactoryLogicalVersionIsString(t, server.URL(), server.SessionID())
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

	saved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), string(body))
	assertFactoryWorkType(t, saved, "story", "saved full readback document")

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	assertFactoryWorkType(t, reloaded, "story", "reloaded full readback document")
	submitWorkForSessionAndExpectStatus(t, server.URL(), server.SessionID(), "story", "full-readback-save", http.StatusCreated)
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

	server := startDocumentTransformationServer(t, rootDir, "")
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
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
	saved := saveCurrentFactoryForSessionWithClient(t, client, server.URL(), server.SessionID(), string(body))
	assertFactoryWorkType(t, saved, "story", "saved bundled default factory")
	assertPortableFile(t, filepath.Join(rootDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(rootDir, "scripts", "setup-workspace.py"), "print('portable script')\n")
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(rootDir, interfaces.FactoryConfigFile))

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	assertFactoryWorkType(t, reloaded, "story", "reloaded bundled default factory")
	submitWorkForSessionAndExpectStatus(t, server.URL(), server.SessionID(), "story", "bundled-default-save", http.StatusCreated)
}

func TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated(t *testing.T) {
	alphaRootDir := t.TempDir()
	betaRootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, alphaRootDir, "alpha", "alpha-task")
	createNamedFactoryFixture(
		t,
		betaRootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startDocumentTransformationServer(t, alphaRootDir, "alpha")
	betaServer := startDocumentTransformationServer(t, betaRootDir, "beta")

	start := make(chan struct{})
	operationsReady := make(chan struct{}, 2)
	verificationRelease := make(chan struct{})
	// Parallel subtests wait for this signal so both sessions issue their
	// public transformation and Work operations in the same lifecycle window.
	// The second signal keeps both public session closes in that same window.
	go func() {
		<-operationsReady
		<-operationsReady
		close(verificationRelease)
	}()

	run := func(
		name string,
		transformationServer *documentTransformationServer,
		factoryName,
		initialWorkType,
		savedWorkType string,
	) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			<-start

			ready := false
			defer func() {
				if !ready {
					operationsReady <- struct{}{}
				}
			}()

			initialEvents := transformationServer.GetFactoryEvents(t)
			current := getCurrentFactoryForSession(t, transformationServer.URL(), transformationServer.SessionID())
			if current.Name != factoryapi.FactoryName(factoryName) {
				t.Fatalf("%s session current factory name = %q, want %s", name, current.Name, factoryName)
			}
			assertFactoryWorkType(t, current, initialWorkType, name+" session current factory before save")

			saved := saveCurrentFactoryForSession(
				t,
				transformationServer.URL(),
				transformationServer.SessionID(),
				functionalNamedFactoryBody(factoryName, savedWorkType, advancedFactoryVersion(t, current.Version)),
			)
			if saved.Name != factoryapi.FactoryName(factoryName) {
				t.Fatalf("%s saved session factory name = %q, want %s", name, saved.Name, factoryName)
			}
			assertFactoryWorkType(t, saved, savedWorkType, name+" saved session factory")

			reloaded := getCurrentFactoryForSession(t, transformationServer.URL(), transformationServer.SessionID())
			if reloaded.Name != factoryapi.FactoryName(factoryName) {
				t.Fatalf("%s reloaded session factory name = %q, want %s", name, reloaded.Name, factoryName)
			}
			assertFactoryWorkType(t, reloaded, savedWorkType, name+" reloaded session factory")

			change := requireFactoryChangeAfter(t, initialEvents, transformationServer.GetFactoryEvents(t))
			changePayload, err := change.Payload.AsFactoryChangeEventPayload()
			if err != nil {
				t.Fatalf("%s decode Factory Event replay payload: %v", name, err)
			}
			if changePayload.Factory.Name != factoryapi.FactoryName(factoryName) {
				t.Fatalf("%s Factory Event replay name = %q, want %s", name, changePayload.Factory.Name, factoryName)
			}
			assertFactoryWorkType(t, changePayload.Factory, savedWorkType, name+" Factory Event replay")
			assertPersistedNamedFactoryWorkType(t, transformationServer.RootDir(), factoryName, savedWorkType)
			if initialWorkType != savedWorkType {
				submitWorkForSessionAndExpectStatus(
					t,
					transformationServer.URL(),
					transformationServer.SessionID(),
					initialWorkType,
					"old-session-work",
					http.StatusBadRequest,
				)
			}

			const workName = "concurrent-session-work"
			workResponse := submitWorkForSessionWithNameAndExpectStatus(
				t,
				transformationServer.URL(),
				transformationServer.SessionID(),
				workName,
				savedWorkType,
				workName,
				http.StatusCreated,
			)
			var submitted factoryapi.SubmitWorkResponse
			decodeJSONResponse(t, workResponse, &submitted, name+" decode session Work submit response")
			if submitted.SessionId == nil || *submitted.SessionId != transformationServer.SessionID() {
				t.Fatalf("%s submitted Work session ID = %#v, want %q", name, submitted.SessionId, transformationServer.SessionID())
			}
			if submitted.TraceId == "" {
				t.Fatalf("expected non-empty trace ID for %s session-scoped transformed Factory submission", name)
			}

			listed := listWorkForSession(t, transformationServer.URL(), transformationServer.SessionID())
			if len(listed.Results) != 1 {
				t.Fatalf("%s session Work results = %d, want exactly one isolated Work: %#v", name, len(listed.Results), listed.Results)
			}
			if listed.Results[0].WorkTypeName == nil || *listed.Results[0].WorkTypeName != savedWorkType {
				t.Fatalf("%s listed Work type = %#v, want %q", name, listed.Results[0].WorkTypeName, savedWorkType)
			}
			if listed.Results[0].Name != workName {
				t.Fatalf("%s listed Work name = %q, want %q", name, listed.Results[0].Name, workName)
			}

			ready = true
			t.Logf(
				"ISO-003: concurrent save/replay/Work complete session=%s root=%s",
				transformationServer.SessionID(),
				transformationServer.RootDir(),
			)
			operationsReady <- struct{}{}
			<-verificationRelease

			postVerification := getCurrentFactoryForSession(t, transformationServer.URL(), transformationServer.SessionID())
			if postVerification.Name != factoryapi.FactoryName(factoryName) {
				t.Fatalf("%s post-concurrency factory name = %q, want %s", name, postVerification.Name, factoryName)
			}
			assertFactoryWorkType(t, postVerification, savedWorkType, name+" post-concurrency factory")
			transformationServer.close(t)
		})
	}

	run("alpha", server, "alpha", "alpha-task", "alpha-task")
	run("beta", betaServer, "beta", "beta-task", "story")
	t.Logf(
		"ISO-003: releasing concurrent session operations alpha=%s beta=%s",
		server.SessionID(),
		betaServer.SessionID(),
	)
	close(start)
}

func TestCurrentFactoryPUT_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startDocumentTransformationServer(t, rootDir, "alpha")
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
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

	resp := saveCurrentFactoryForSessionExpectStatus(t, server.URL(), server.SessionID(), body, http.StatusBadRequest)
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
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startDocumentTransformationServer(t, rootDir, "alpha")

	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
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

	resp := saveCurrentFactoryForSessionExpectStatus(t, server.URL(), server.SessionID(), body, http.StatusBadRequest)
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

	server := startDocumentTransformationServer(t, rootDir, "")
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
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

	resp := saveCurrentFactoryForSessionExpectStatus(t, server.URL(), server.SessionID(), string(body), http.StatusBadRequest)
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

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if reloaded.Version == nil || *reloaded.Version != *current.Version {
		t.Fatalf("reloaded version = %#v, want unchanged %#v", reloaded.Version, current.Version)
	}
	assertFactoryWorkType(t, reloaded, "task", "reloaded factory after rejected type-count collision")
}

func TestCurrentFactoryPUT_RequiresAdvancedSaveVersion(t *testing.T) {
	// One explicit Factory Session for the whole advanced-version matrix. Each row
	// restores a deterministic "alpha"/"task" baseline before asserting so a
	// successful save (or rejected stale write) cannot contaminate later rows.
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "task")
	server := startDocumentTransformationServer(t, rootDir, "alpha")

	for _, tc := range advancedSaveVersionCases() {
		t.Run(tc.name, func(t *testing.T) {
			runAdvancedSaveVersionCase(t, server, tc)
		})
	}
}

type advancedSaveVersionCase struct {
	name      string
	version   func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any
	wantCode  factoryapi.ErrorResponseCode
	wantState string
}

func runAdvancedSaveVersionCase(t *testing.T, server *documentTransformationServer, tc advancedSaveVersionCase) {
	t.Helper()
	current := ensureAdvancedSaveVersionBaseline(t, server)
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata")
	}

	body := currentFactorySaveDocument(t, "alpha", "story", tc.version(t, *current.Version))
	if tc.wantCode != "" {
		resp := saveCurrentFactoryForSessionExpectStatus(t, server.URL(), server.SessionID(), body, http.StatusConflict)
		var errResp factoryapi.ErrorResponse
		decodeJSONResponse(t, resp, &errResp, "decode stale current factory save response")
		if errResp.Code != tc.wantCode {
			t.Fatalf("error code = %q, want %q", errResp.Code, tc.wantCode)
		}
	} else {
		saved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), body)
		assertFactoryWorkType(t, saved, "story", "saved current factory")
	}

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	assertFactoryWorkType(t, reloaded, tc.wantState, "reloaded current factory after version save")
}

// ensureAdvancedSaveVersionBaseline returns the current Factory after guaranteeing
// work type "task" so matrix rows remain independently isolatable on one server.
func ensureAdvancedSaveVersionBaseline(t *testing.T, server *documentTransformationServer) factoryapi.Factory {
	t.Helper()
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata")
	}
	if current.WorkTypes != nil && len(*current.WorkTypes) == 1 && (*current.WorkTypes)[0].Name == "task" {
		return current
	}
	return saveCurrentFactoryForSession(
		t,
		server.URL(),
		server.SessionID(),
		currentFactorySaveDocument(t, "alpha", "task", versionDocument(advancedFactoryVersion(t, current.Version))),
	)
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
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing version fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return nil
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingLogicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing logical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"physical": current.Physical.Add(time.Second).UTC().Format(time.RFC3339Nano)}
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingPhysicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing physical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"logical": strconv.FormatInt(current.Logical.Int64()+1, 10)}
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
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

func createNamedFactoryFixture(
	t *testing.T,
	rootDir string,
	name string,
	payload []byte,
) string {
	t.Helper()
	fixture := factoryTransformationFixture
	if fixture == nil {
		t.Fatal("shared factory transformation fixture is unavailable")
	}

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateNamedFactoryAtRootWithProcess(
		t,
		fixture.process,
		[]string{"HOME=" + fixture.homeDir, "USERPROFILE=" + fixture.homeDir},
		sourceDir,
		rootDir,
		name,
		sourcePath,
	)
}

func assertRawCurrentFactoryLogicalVersionIsString(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	resp, err := http.Get(sessionFactoryURL(serverURL, sessionID))
	if err != nil {
		t.Fatalf("GET %s: %v", sessionFactoryURL(serverURL, sessionID), err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET %s status = %d, want 200", sessionFactoryURL(serverURL, sessionID), resp.StatusCode)
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
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		factorySessionPath(sessionID),
		saveFactoryForSessionRequestBody(body),
		http.StatusOK,
	)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode session current factory save response")
	return saved
}

func saveCurrentFactoryForSessionWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	sessionID,
	body string,
) factoryapi.Factory {
	t.Helper()
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		client,
		serverURL,
		"/factory-sessions/"+url.PathEscape(sessionID)+"/factory",
		saveFactoryForSessionRequestBody(body),
		http.StatusOK,
	)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode session current factory save response")
	return saved
}

func saveCurrentFactoryForSessionExpectStatus(
	t *testing.T,
	serverURL,
	sessionID,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()
	return putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/"+url.PathEscape(sessionID)+"/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func sessionFactoryURL(serverURL, sessionID string) string {
	return serverURL + factorySessionPath(sessionID)
}

func listWorkForSession(t *testing.T, serverURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func factorySessionPath(sessionID string) string {
	return "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
}

func submitWorkForSessionAndExpectStatus(t *testing.T, serverURL, sessionID, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	return submitWorkForSessionWithNameAndExpectStatus(
		t,
		serverURL,
		sessionID,
		"factory-transformation-session-submit",
		workType,
		title,
		wantStatus,
	)
}

func submitWorkForSessionWithNameAndExpectStatus(
	t *testing.T,
	serverURL,
	sessionID,
	name,
	workType,
	title string,
	wantStatus int,
) *http.Response {
	t.Helper()
	endpoint := serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": workType,
		"payload":      map[string]string{"title": title},
	})
	if err != nil {
		t.Fatalf("marshal POST /factory-sessions/%s/work request: %v", sessionID, err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
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

func assertPersistedNamedFactoryWorkType(t *testing.T, rootDir, factoryName, wantWorkType string) {
	t.Helper()
	path := filepath.Join(rootDir, factoryName, interfaces.FactoryConfigFile)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var document struct {
		WorkTypes []struct {
			Name string `json:"name"`
		} `json:"workTypes"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode persisted Factory %s: %v", path, err)
	}
	if len(document.WorkTypes) != 1 || document.WorkTypes[0].Name != wantWorkType {
		t.Fatalf("persisted Factory %s work types = %#v, want [%q]", path, document.WorkTypes, wantWorkType)
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

func requireLatestFactoryChange(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == factoryapi.FactoryEventTypeFactoryChange {
			return events[i]
		}
	}
	t.Fatalf("factory-change event not found; events=%d", len(events))
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
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514","body":"You are the planner."}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","body":"Plan the work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
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
