package testharness_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/testharness"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const finalWorkflow = `return { subject: args.subject };`

const childWorkflow = `return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    modelProvider: "codex",
  });
  return { child: child };
})();`

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type memoryStore struct {
	mu        sync.Mutex
	snapshots map[string][]byte
}

func newMemoryStore() *memoryStore { return &memoryStore{snapshots: make(map[string][]byte)} }

func (s *memoryStore) Save(sessionID string, encoded []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[sessionID] = append([]byte(nil), encoded...)
	return nil
}

func (s *memoryStore) Load(sessionID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, ok := s.snapshots[sessionID]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), encoded...), nil
}

type recordingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *recordingProvider) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return workerexecution.InferenceResponse{Content: `{"text":"provider result"}`}, nil
}

func (p *recordingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestNew_SelectsFakeAndKeepsInstancesIsolated(t *testing.T) {
	scenario := factorysessionexecution.FakeScenario{
		RequestID: "request-fake",
		Session: factorysessionexecution.SessionReadResult{
			SessionID: "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
		},
	}
	first, err := testharness.New(testharness.Config{
		Mode:        testharness.ModeFake,
		FakeOptions: []factorysessionexecution.FakeServiceOption{factorysessionexecution.WithFakeScenarios(scenario)},
	})
	if err != nil {
		t.Fatalf("New(first fake): %v", err)
	}
	second, err := testharness.New(testharness.Config{Mode: testharness.ModeFake})
	if err != nil {
		t.Fatalf("New(second fake): %v", err)
	}

	started, err := first.StartSync(context.Background(), startRequest("request-fake", finalWorkflow))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if _, err := second.GetSession(context.Background(), started.SessionID); err == nil {
		t.Fatal("second harness observed first harness session")
	}
}

func TestNewJavaScript_ForwardsClockProjectRootAndPersistence(t *testing.T) {
	projectRoot := t.TempDir()
	writeNamedWorkflow(t, projectRoot, "from-project", finalWorkflow)
	store := newMemoryStore()
	wantTime := time.Date(2035, time.January, 2, 3, 4, 5, 0, time.FixedZone("test", -8*60*60))
	config := javascriptConfig(projectRoot, fixedClock{now: wantTime}, store)
	service, err := testharness.New(config)
	if err != nil {
		t.Fatalf("New(JavaScript): %v", err)
	}

	started, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "request-project-root",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "from-project",
		},
		Args: map[string]any{"subject": "root"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Lifecycle == nil || read.Lifecycle.StartedAt == nil || !read.Lifecycle.StartedAt.Equal(wantTime.UTC()) {
		t.Fatalf("startedAt = %#v, want %s", read.Lifecycle, wantTime.UTC())
	}

	reloaded, err := testharness.New(config)
	if err != nil {
		t.Fatalf("New(reloaded JavaScript): %v", err)
	}
	if _, err := reloaded.GetSession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("GetSession from reused persistence: %v", err)
	}

	isolated, err := testharness.New(javascriptConfig(projectRoot, fixedClock{now: wantTime}, newMemoryStore()))
	if err != nil {
		t.Fatalf("New(isolated JavaScript): %v", err)
	}
	if _, err := isolated.GetSession(context.Background(), started.SessionID); err == nil {
		t.Fatal("independent persistence unexpectedly shared session state")
	}
}

func TestNewJavaScript_ForwardsLiveChildProviderAndMode(t *testing.T) {
	provider := &recordingProvider{}
	config := javascriptConfig(t.TempDir(), fixedClock{now: time.Now()}, newMemoryStore())
	config.ChildExecutorMode = factorysessionexecution.ChildExecutorModeLive
	config.Provider = provider
	service, err := testharness.New(config)
	if err != nil {
		t.Fatalf("New(JavaScript live): %v", err)
	}

	started, err := service.StartSync(context.Background(), startRequest("request-live-child", childWorkflow))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Status != string(factorysessionexecution.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if provider.callCount() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.callCount())
	}
}

func TestNew_RejectsUnsupportedAndIncompleteConfiguration(t *testing.T) {
	valid := javascriptConfig(t.TempDir(), fixedClock{now: time.Now()}, newMemoryStore())
	tests := []struct {
		name   string
		config testharness.Config
		want   string
	}{
		{name: "missing mode", config: testharness.Config{}, want: "unsupported mode"},
		{name: "fake runtime dependency", config: testharness.Config{Mode: testharness.ModeFake, ProjectRoot: t.TempDir()}, want: "does not accept"},
		{name: "fake fixture and options", config: testharness.Config{Mode: testharness.ModeFake, FakeFixturePath: "fixtures.json", FakeOptions: []factorysessionexecution.FakeServiceOption{factorysessionexecution.WithFakeScenarios()}}, want: "not both"},
		{name: "missing fake fixture", config: testharness.Config{Mode: testharness.ModeFake, FakeFixturePath: filepath.Join(t.TempDir(), "missing.json")}, want: "load fake fixtures"},
		{name: "missing root", config: withConfig(valid, func(c *testharness.Config) { c.ProjectRoot = "" }), want: "project root is required"},
		{name: "missing clock", config: withConfig(valid, func(c *testharness.Config) { c.Clock = nil }), want: "clock is required"},
		{name: "missing persistence", config: withConfig(valid, func(c *testharness.Config) { c.Persistence = nil }), want: "persistence is required"},
		{name: "missing child mode", config: withConfig(valid, func(c *testharness.Config) { c.ChildExecutorMode = "" }), want: "child executor mode"},
		{name: "live without provider", config: withConfig(valid, func(c *testharness.Config) { c.ChildExecutorMode = factorysessionexecution.ChildExecutorModeLive }), want: "provider is required"},
		{name: "fake with provider", config: withConfig(valid, func(c *testharness.Config) { c.Provider = &recordingProvider{} }), want: "provider is only valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testharness.New(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func javascriptConfig(projectRoot string, clock fixedClock, store *memoryStore) testharness.Config {
	return testharness.Config{
		Mode:              testharness.ModeJavaScript,
		ProjectRoot:       projectRoot,
		Clock:             clock,
		Persistence:       store,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeFake,
	}
}

func withConfig(config testharness.Config, change func(*testharness.Config)) testharness.Config {
	change(&config)
	return config
}

func startRequest(requestID, source string) factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source: factorysessionexecution.Source{
			Kind: workflowsource.KindInlineWorkflow,
			InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
				Dialect:      "you-workflow-v1",
				InlineSource: source,
			},
		},
		Args: map[string]any{"subject": "test"},
	}
}

func writeNamedWorkflow(t *testing.T, projectRoot, name, source string) {
	t.Helper()
	dir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".js"), []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
