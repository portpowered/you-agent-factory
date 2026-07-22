package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
)

type fixedOpenCodeIdentifier struct{ executable string }

func testProviderExecRunner(t testing.TB) CommandRunner {
	t.Helper()
	effect, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	return workerprocess.AdaptCommandRunner(effect)
}

func (i fixedOpenCodeIdentifier) Identify(context.Context, string) (opencodeadapter.Installation, error) {
	executable := i.executable
	if executable == "" {
		executable = "opencode"
	}
	return opencodeadapter.Installation{Executable: executable, Fingerprint: "test-installation"}, nil
}

type fixedOpenCodeDiscoverer struct{ mode opencodeadapter.Mode }

func (d fixedOpenCodeDiscoverer) Discover(context.Context, opencodeadapter.Installation) (opencodeadapter.Decision, error) {
	return opencodeadapter.Decision{Version: "1.2.3", Mode: d.mode}, nil
}

func openCodeResolverForTest(t *testing.T, mode opencodeadapter.Mode) *opencodeadapter.Resolver {
	return openCodeResolverForExecutable(t, mode, "")
}

func openCodeResolverForExecutable(t *testing.T, mode opencodeadapter.Mode, executable string) *opencodeadapter.Resolver {
	t.Helper()
	resolver, err := opencodeadapter.NewResolver(
		fixedOpenCodeIdentifier{executable: executable},
		fixedOpenCodeDiscoverer{mode: mode},
		0,
		0,
	)
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}
	return resolver
}

func newScriptWrapProviderForTest(t *testing.T, runner CommandRunner, modelProvider string) *ScriptWrapProvider {
	t.Helper()
	var resolver *opencodeadapter.Resolver
	if modelProvider == string(modelprovider.ProviderOpenCode) {
		resolver = openCodeResolverForTest(t, opencodeadapter.ModeFinalOnly)
	}
	return NewScriptWrapProviderWithDependencies(
		false, nil, runner, resolver, nil, nil, "", nil, nil,
	)
}

func InputTokens(tokens ...factoryruntime.RuntimeToken) []any {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]any, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, token)
	}
	return out
}

func CommandRequestInputTokens(request CommandRequest) []factoryruntime.RuntimeToken {
	return cloneInputTokens(request.InputTokens)
}

func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, pair := range env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func mapValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func assertExecutionMetadataEqual(t *testing.T, want, got work.ExecutionMetadata) {
	t.Helper()
	if got.DispatchCreatedTick != want.DispatchCreatedTick {
		t.Fatalf("DispatchCreatedTick = %d, want %d", got.DispatchCreatedTick, want.DispatchCreatedTick)
	}
	if got.CurrentTick != want.CurrentTick {
		t.Fatalf("CurrentTick = %d, want %d", got.CurrentTick, want.CurrentTick)
	}
	if got.RequestID != want.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, want.RequestID)
	}
	if got.TraceID != want.TraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.ReplayKey != want.ReplayKey {
		t.Fatalf("ReplayKey = %q, want %q", got.ReplayKey, want.ReplayKey)
	}
	if len(got.WorkIDs) != len(want.WorkIDs) {
		t.Fatalf("WorkIDs length = %d, want %d: %#v", len(got.WorkIDs), len(want.WorkIDs), got.WorkIDs)
	}
	for i := range want.WorkIDs {
		if got.WorkIDs[i] != want.WorkIDs[i] {
			t.Fatalf("WorkIDs[%d] = %q, want %q; full WorkIDs: %#v", i, got.WorkIDs[i], want.WorkIDs[i], got.WorkIDs)
		}
	}
}

func intPtr(value int) *int {
	return &value
}

type april11FailureShapeFixture struct {
	Samples []april11FailureShapeSample `json:"samples"`
}

type april11FailureShapeSample struct {
	Name                  string                          `json:"name"`
	ExitCode              int                             `json:"exit_code"`
	Stdout                string                          `json:"stdout"`
	Stderr                string                          `json:"stderr"`
	WantType              workerexecution.WorkFailureType `json:"want_type"`
	WantMessage           string                          `json:"want_message"`
	WantRetryable         bool                            `json:"want_retryable"`
	WantTerminal          bool                            `json:"want_terminal"`
	WantThrottlePause     bool                            `json:"want_throttle_pause"`
	RejectMessageContains []string                        `json:"reject_message_contains"`
}

func loadApril11FailureShapeFixture(t *testing.T) april11FailureShapeFixture {
	t.Helper()

	path := filepath.Join("testdata", "april11_2026_failure_shapes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read April 11 failure-shape fixture: %v", err)
	}

	var fixture april11FailureShapeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode April 11 failure-shape fixture: %v", err)
	}
	if len(fixture.Samples) == 0 {
		t.Fatal("expected April 11 failure-shape fixture to contain samples")
	}
	return fixture
}

type recordingProviderExec struct {
	request CommandRequest
	result  CommandResult
	err     error
	calls   int
}

func (r *recordingProviderExec) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	r.calls++
	r.request = CommandRequest(workerexecution.CloneSubprocessExecutionRequest(req))
	return r.result, r.err
}

type envPrintingProviderExec struct{}

func (envPrintingProviderExec) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	return CommandResult{
		Stdout: []byte(strings.Join(req.Env, "\n")),
	}, nil
}

func runGitSetup(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := testProviderExecRunner(t).Run(ctx, CommandRequest{
		Command: "git",
		Args:    args,
		Env:     isolatedGitEnv(os.Environ()),
		WorkDir: dir,
	})
	if err != nil {
		t.Fatalf("git %s returned system error: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git %s exit code = %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), result.ExitCode, result.Stdout, result.Stderr)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func writeExecutableTestScript(t *testing.T, path string, content string) {
	t.Helper()

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		t.Fatalf("creating executable temp %s: %v", path, err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmpFile.Chmod(0o755); err != nil {
		cleanup()
		t.Fatalf("chmod temp executable %s: %v", tmpPath, err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		cleanup()
		t.Fatalf("writing %s: %v", tmpPath, err)
	}
	if err := tmpFile.Sync(); err != nil {
		cleanup()
		t.Fatalf("syncing %s: %v", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		t.Fatalf("closing %s: %v", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		t.Fatalf("renaming %s to %s: %v", tmpPath, path, err)
	}
	syncDirectoryForExecutableWrite(t, dir)
	releaseExecutableWriteLock(t, path)
}

func writeProviderOutputFixture(t *testing.T, path string, stdout, stderr []byte, exitCode int) string {
	t.Helper()
	dir := filepath.Dir(path)
	stdoutPath := filepath.Join(dir, "fixture.stdout")
	stderrPath := filepath.Join(dir, "fixture.stderr")
	if err := os.WriteFile(stdoutPath, stdout, 0o600); err != nil {
		t.Fatalf("write fixture stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, stderr, 0o600); err != nil {
		t.Fatalf("write fixture stderr: %v", err)
	}

	if runtime.GOOS == "windows" {
		path += ".cmd"
		body := fmt.Sprintf("@echo off\r\ntype %q\r\ntype %q 1>&2\r\nexit /b %d\r\n", stdoutPath, stderrPath, exitCode)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write Windows provider fixture: %v", err)
		}
		return path
	}

	body := fmt.Sprintf("#!/bin/sh\ncat %q\ncat %q 1>&2\nexit %d\n", stdoutPath, stderrPath, exitCode)
	writeExecutableTestScript(t, path, body)
	return path
}

func releaseExecutableWriteLock(t *testing.T, path string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return
	}

	executable, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening executable %s after write: %v", path, err)
	}
	if err := executable.Close(); err != nil {
		t.Fatalf("closing executable %s after write: %v", path, err)
	}
}

func syncDirectoryForExecutableWrite(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return
	}

	directory, err := os.Open(dir)
	if err != nil {
		t.Fatalf("opening directory %s for sync: %v", dir, err)
	}
	defer func() {
		if err := directory.Close(); err != nil {
			t.Fatalf("closing directory %s after executable write sync: %v", dir, err)
		}
	}()
	if err := directory.Sync(); err != nil {
		t.Fatalf("syncing directory %s after executable write: %v", dir, err)
	}
}

func writeEditorMarkerScript(t *testing.T, markerPath string) string {
	t.Helper()
	dir := t.TempDir()
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		script := filepath.Join(dir, "editor.bat")
		content := "@echo off\r\necho invoked > %1\r\nexit /b 42\r\n"
		writeExecutableTestScript(t, script, content)
		return script + " " + markerPath
	}

	script := filepath.Join(dir, "editor.sh")
	content := "#!/bin/sh\nprintf invoked > \"$1\"\nexit 42\n"
	writeExecutableTestScript(t, script, content)
	return script + " " + markerPath
}

func providerGitTestEnv(envVars map[string]string) []string {
	return isolatedGitEnv(buildProviderEnv(os.Environ(), envVars))
}

func isolatedGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || inheritedGitRepoEnv[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

var inheritedGitRepoEnv = map[string]bool{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_PREFIX":                       true,
	"GIT_QUARANTINE_PATH":              true,
	"GIT_WORK_TREE":                    true,
}

func assertStringSlicesEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("expected %d args, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("expected arg %d to be %q, got %q; full args: %v", i, want[i], got[i], got)
		}
	}
}

func assertCommandRequestAssemblyMatchesProviderBehavior(t *testing.T, want, got CommandRequest) {
	t.Helper()
	if got.Command != want.Command {
		t.Fatalf("expected command %q, got %q", want.Command, got.Command)
	}
	assertStringSlicesEqual(t, want.Args, got.Args)
	if string(got.Stdin) != string(want.Stdin) {
		t.Fatalf("expected stdin %q, got %q", string(want.Stdin), string(got.Stdin))
	}
	if got.WorkDir != want.WorkDir {
		t.Fatalf("expected workdir %q, got %q", want.WorkDir, got.WorkDir)
	}
}

func assertStringSliceDoesNotContain(t *testing.T, values []string, forbidden string) {
	t.Helper()
	for _, value := range values {
		if value == forbidden {
			t.Fatalf("expected args not to contain %q, got %v", forbidden, values)
		}
	}
}

func assertEnvContains(t *testing.T, env []string, want string) {
	t.Helper()
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected env to contain %q", want)
}

func assertEnvValue(t *testing.T, env []string, name, want string) {
	t.Helper()
	values := envSliceToMap(env)
	if got := values[name]; got != want {
		t.Fatalf("expected env %s=%q, got %q", name, want, got)
	}
}

func assertEnvEntryCount(t *testing.T, env []string, name string, want int) {
	t.Helper()
	prefix := name + "="
	got := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected env %s to appear %d time(s), got %d in %v", name, want, got, env)
	}
}

func assertProviderAutomationDefaults(t *testing.T, env []string) {
	t.Helper()
	for _, entry := range providerAutomationEnvDefaults {
		assertEnvValue(t, env, entry.Name, entry.Value)
	}
}
func TestScriptWrapProvider_Infer_CodexImageContentEmitsOrderedImageArgs(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	imageOne := "fixtures/one.png"
	imageTwo := "fixtures/two.png"
	imageOnePath := filepath.Join(workspace, filepath.FromSlash(imageOne))
	imageTwoPath := filepath.Join(workspace, filepath.FromSlash(imageTwo))
	if err := os.MkdirAll(filepath.Join(workspace, "fixtures"), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(imageOnePath, []byte("image-one"), 0o644); err != nil {
		t.Fatalf("write first image: %v", err)
	}
	if err := os.WriteFile(imageTwoPath, []byte("image-two"), 0o644); err != nil {
		t.Fatalf("write second image: %v", err)
	}
	imageOneURL, err := work.FilesystemPathToContentURL(imageOne)
	if err != nil {
		t.Fatalf("image one url: %v", err)
	}
	imageTwoURL, err := work.FilesystemPathToContentURL(imageTwo)
	if err != nil {
		t.Fatalf("image two url: %v", err)
	}

	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProviderWithDependencies(
		false, nil, fakeExec, nil, nil, nil, "", nil,
		work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
			switch {
			case strings.HasSuffix(rawURL, "/fixtures/one.png"):
				return imageOnePath, func() {}, nil
			case strings.HasSuffix(rawURL, "/fixtures/two.png"):
				return imageTwoPath, func() {}, nil
			default:
				return "", func() {}, fmt.Errorf("unexpected content URL %q", rawURL)
			}
		}),
	)

	_, err = provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect the images",
		WorkingDirectory: workspace,
		InputTokens: InputTokens(
			factoryruntime.RuntimeToken{
				ID: "token-1",
				Color: factoryruntime.RuntimeTokenColor{
					Content: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeText, Text: "before"},
						{Type: work.WorkContentPartTypeImage, URL: imageOneURL},
					},
				},
			},
			factoryruntime.RuntimeToken{
				ID: "token-2",
				Color: factoryruntime.RuntimeTokenColor{
					Content: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeImage, URL: imageTwoURL},
						{Type: work.WorkContentPartTypeText, Text: "after"},
					},
				},
			},
		),
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	wantArgs := []string{"exec", "--model", "gpt-5-codex", "-i", imageOnePath, "-i", imageTwoPath, "-"}
	assertStringSlicesEqual(t, wantArgs, fakeExec.request.Args)
	if string(fakeExec.request.Stdin) != "inspect the images" {
		t.Fatalf("expected codex stdin to carry the prompt, got %q", string(fakeExec.request.Stdin))
	}
}

func TestScriptWrapProvider_Infer_CodexTextOnlyContentDoesNotEmitImageArgs(t *testing.T) {
	t.Parallel()
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProviderWithDependencies(
		false, nil, fakeExec, nil, nil, nil, "", nil, nil,
	)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "text only",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeText, Text: "only text"},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	wantArgs := []string{"exec", "--model", "gpt-5-codex", "-"}
	assertStringSlicesEqual(t, wantArgs, fakeExec.request.Args)
}

func TestScriptWrapProvider_Infer_CodexMissingImageFailsBeforeRunner(t *testing.T) {
	t.Parallel()
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProviderWithDependencies(
		false, nil, fakeExec, nil, nil, nil, "", nil,
		work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
			return "", func() {}, fmt.Errorf("media url not readable: %s", rawURL)
		}),
	)

	missingURL, err := work.FilesystemPathToContentURL("fixtures/missing.png")
	if err != nil {
		t.Fatalf("missing url: %v", err)
	}

	_, err = provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect",
		WorkingDirectory: t.TempDir(),
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeImage, URL: missingURL},
				},
			},
		}),
	})
	if err == nil {
		t.Fatal("expected missing image to fail")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, providerErr)
	}
	if providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if !strings.Contains(providerErr.Message, `input_tokens[0].color.content[0].url`) ||
		!strings.Contains(providerErr.Message, `media url not readable`) ||
		!strings.Contains(providerErr.Message, `fixtures/missing.png`) {
		t.Fatalf("provider error message = %q", providerErr.Message)
	}
	if fakeExec.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", fakeExec.calls)
	}
}

func TestScriptWrapProvider_Infer_CodexRemoteImageMaterializesToTempPath(t *testing.T) {
	t.Parallel()
	body := []byte("remote-image")
	remoteURL := "https://assets.example.test/remote.png"
	materializedPath := filepath.Join(t.TempDir(), "remote.png")
	if err := os.WriteFile(materializedPath, body, 0o600); err != nil {
		t.Fatalf("write materialized fixture: %v", err)
	}

	fakeExec := &codexImageMaterializationAssertExec{
		recordingProviderExec: recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}},
		wantBody:              body,
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil,
		fakeExec, nil, nil, nil, "", nil,
		work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
			if rawURL != remoteURL {
				return "", func() {}, fmt.Errorf("unexpected content URL %q", rawURL)
			}
			return materializedPath, func() {}, nil
		}))

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "inspect remote image",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeImage, URL: remoteURL},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if fakeExec.imagePath == remoteURL {
		t.Fatalf("expected materialized temp path, got remote URL %q", fakeExec.imagePath)
	}
}

type codexImageMaterializationAssertExec struct {
	recordingProviderExec
	wantBody  []byte
	imagePath string
}

func (e *codexImageMaterializationAssertExec) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	for i, arg := range req.Args {
		if arg == "-i" && i+1 < len(req.Args) {
			e.imagePath = req.Args[i+1]
			got, err := os.ReadFile(e.imagePath)
			if err != nil {
				return CommandResult{}, err
			}
			if string(got) != string(e.wantBody) {
				return CommandResult{}, fmt.Errorf("materialized body = %q, want %q", got, e.wantBody)
			}
			break
		}
	}
	return e.recordingProviderExec.Run(ctx, req)
}

func TestScriptWrapProvider_Infer_CodexInaccessibleRemoteImageFailsBeforeRunner(t *testing.T) {
	t.Parallel()
	remoteURL := "https://assets.example.test/missing.png"

	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProviderWithDependencies(false, nil,
		fakeExec, nil, nil, nil, "", nil,
		work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
			return "", func() {}, fmt.Errorf("media url inaccessible: %s", rawURL)
		}))

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "inspect remote image",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeImage, URL: remoteURL},
				},
			},
		}),
	})
	if err == nil {
		t.Fatal("expected inaccessible remote image to fail")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, providerErr)
	}
	if providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if !strings.Contains(providerErr.Message, `input_tokens[0].color.content[0].url`) ||
		!strings.Contains(providerErr.Message, `media url inaccessible`) {
		t.Fatalf("provider error message = %q", providerErr.Message)
	}
	if fakeExec.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", fakeExec.calls)
	}
}

// Smoke: one Codex dispatch materializes both file:// and remote https URLs to distinct -i paths.
func TestScriptWrapProvider_Infer_CodexBatchLocalAndRemoteImageURLs(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	localPath := filepath.Join(workspace, "local.png")
	if err := os.WriteFile(localPath, []byte("local-image"), 0o644); err != nil {
		t.Fatalf("write local image: %v", err)
	}
	localURL, err := work.FilesystemPathToContentURL(localPath)
	if err != nil {
		t.Fatalf("local content url: %v", err)
	}

	remoteBody := []byte("remote-image")
	remoteURL := "https://assets.example.test/remote.png"
	remotePath := filepath.Join(t.TempDir(), "remote.png")
	if err := os.WriteFile(remotePath, remoteBody, 0o600); err != nil {
		t.Fatalf("write materialized fixture: %v", err)
	}

	fakeExec := &codexMixedImageAssertExec{
		recordingProviderExec: recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}},
		wantLocalPath:         localPath,
		wantRemoteBody:        remoteBody,
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil,
		fakeExec, nil, nil, nil, "", nil,
		work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
			switch rawURL {
			case localURL:
				return localPath, func() {}, nil
			case remoteURL:
				return remotePath, func() {}, nil
			default:
				return "", func() {}, fmt.Errorf("unexpected content URL %q", rawURL)
			}
		}))

	_, err = provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect both images",
		WorkingDirectory: workspace,
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeImage, URL: localURL},
					{Type: work.WorkContentPartTypeImage, URL: remoteURL},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if !fakeExec.sawLocal || !fakeExec.sawRemote {
		t.Fatalf("image args: local=%t remote=%t, want both materialized", fakeExec.sawLocal, fakeExec.sawRemote)
	}
}

type codexMixedImageAssertExec struct {
	recordingProviderExec
	wantLocalPath  string
	wantRemoteBody []byte
	sawLocal       bool
	sawRemote      bool
}

func (e *codexMixedImageAssertExec) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	for i, arg := range req.Args {
		if arg != "-i" || i+1 >= len(req.Args) {
			continue
		}
		path := req.Args[i+1]
		switch {
		case path == e.wantLocalPath:
			e.sawLocal = true
		default:
			got, err := os.ReadFile(path)
			if err != nil {
				return CommandResult{}, err
			}
			if string(got) == string(e.wantRemoteBody) {
				e.sawRemote = true
			}
		}
	}
	return e.recordingProviderExec.Run(ctx, req)
}

func TestInferenceProgressPublishingCommandRunner_PublishesOrderedFragments(t *testing.T) {
	t.Parallel()

	scriptPath := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "stream"), []byte("stdout-chunk\n"), []byte("stderr-chunk\n"), 0)
	command := scriptPath
	var args []string
	if runtime.GOOS != "windows" {
		command = "/bin/sh"
		args = []string{scriptPath}
	}

	var publishedMu sync.Mutex
	var published []InferenceProgressFragment
	runner := NewInferenceProgressPublishingCommandRunnerWithRunner(testProviderExecRunner(t), func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	result, err := runner.Run(context.Background(), CommandRequest{
		Command:    command,
		Args:       args,
		DispatchID: "dispatch-stream-1",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(string(result.Stdout), "stdout-chunk") {
		t.Fatalf("stdout = %q, want stdout-chunk", result.Stdout)
	}
	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) < 2 {
		t.Fatalf("published events = %d, want at least 2", len(published))
	}

	var sawResponse bool
	var sawProgress bool
	for _, fragment := range published {
		if fragment.DispatchID != "dispatch-stream-1" {
			t.Fatalf("dispatch = %q, want dispatch-stream-1", fragment.DispatchID)
		}
		switch fragment.Kind {
		case ResponseFragmentKind:
			sawResponse = true
		case ProgressFragmentKind:
			sawProgress = true
		default:
			t.Fatalf("unexpected kind %q", fragment.Kind)
		}
	}
	if !sawResponse || !sawProgress {
		t.Fatalf("published fragments = %#v, want both response and progress kinds", published)
	}
}

func TestInferenceProgressPublishingCommandRunner_CursorPublishesDiagnosticsAndLaterValidEventsInOrder(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, string(modelprovider.ProviderCursor))
	writeProviderOutputFixture(t, scriptPath, []byte(
		"{not json}\n"+
			"{\"type\":\"mystery\"}\n"+
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-session-123\"}\n"+
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"cursor-session-123\"}\n",
	), nil, 0)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var publishedMu sync.Mutex
	var published []InferenceProgressFragment
	runner := NewInferenceProgressPublishingCommandRunnerWithRunner(testProviderExecRunner(t), func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	result, err := runner.Run(context.Background(), CommandRequest{
		Command:    string(modelprovider.ProviderCursor),
		DispatchID: "dispatch-stream-cursor",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 4 {
		t.Fatalf("published fragments = %#v, want 4 ordered fragments; result=%#v", published, result)
	}
	var diagnostics []InferenceProgressFragment
	var drafts []factorysessions.ResponseEventDraft
	for _, fragment := range published {
		if fragment.CanonicalDraft == nil {
			diagnostics = append(diagnostics, fragment)
			continue
		}
		draft, ok := fragment.CanonicalDraft.(factorysessions.ResponseEventDraft)
		if !ok {
			t.Fatalf("canonical draft type = %T, want factorysessions.ResponseEventDraft", fragment.CanonicalDraft)
		}
		drafts = append(drafts, draft)
	}
	if len(diagnostics) != 2 || len(drafts) != 2 {
		t.Fatalf("published fragments = %#v, want two diagnostics and two structured drafts", published)
	}
	assertInferenceProgressFragment(t, diagnostics[0], "dispatch-stream-cursor", ProgressFragmentKind, "Cursor stream ignored a malformed JSON record", nil)
	assertInferenceProgressFragment(t, diagnostics[1], "dispatch-stream-cursor", ProgressFragmentKind, "Cursor stream ignored an unknown record type", nil)
	for index, wantPhase := range []factorysessions.ResponseEventPhase{factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventPhaseCompleted} {
		draft := drafts[index]
		if draft.Kind != factorysessions.ResponseEventKindMessage || draft.Phase != wantPhase || draft.DispatchID != "dispatch-stream-cursor" {
			t.Fatalf("drafts[%d] = %#v, want MESSAGE/%s for dispatch", index, draft, wantPhase)
		}
		if draft.ProviderSessionRef != "cursor-session-123" || draft.Provenance.Provider != "cursor" {
			t.Fatalf("drafts[%d] correlation = %#v, want Cursor session", index, draft)
		}
	}
}

func TestInferenceProgressPublishingCommandRunner_WithoutPublisherPreservesExecBehavior(t *testing.T) {
	t.Parallel()
	scriptPath := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "nostream"), []byte("stdout-fallback\n"), []byte("stderr-fallback\n"), 7)

	runner := NewInferenceProgressPublishingCommandRunnerWithRunner(testProviderExecRunner(t), nil, nil)
	result, err := runner.Run(context.Background(), CommandRequest{
		Command: scriptPath,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(string(result.Stdout), "stdout-fallback") {
		t.Fatalf("stdout = %q, want stdout-fallback", result.Stdout)
	}
	if !strings.Contains(string(result.Stderr), "stderr-fallback") {
		t.Fatalf("stderr = %q, want stderr-fallback", result.Stderr)
	}
}

func assertInferenceProgressFragment(
	t *testing.T,
	fragment InferenceProgressFragment,
	wantDispatchID string,
	wantKind string,
	wantPayload string,
	wantSession *workerexecution.ProviderSessionMetadata,
) {
	t.Helper()
	if fragment.DispatchID != wantDispatchID {
		t.Fatalf("dispatch = %q, want %q", fragment.DispatchID, wantDispatchID)
	}
	if fragment.Kind != wantKind {
		t.Fatalf("kind = %q, want %q", fragment.Kind, wantKind)
	}
	if fragment.Payload != wantPayload {
		t.Fatalf("payload = %q, want %q", fragment.Payload, wantPayload)
	}
	if wantSession == nil {
		if fragment.ProviderSessionRef != nil {
			t.Fatalf("provider session = %#v, want nil", fragment.ProviderSessionRef)
		}
		return
	}
	if fragment.ProviderSessionRef == nil {
		t.Fatal("provider session = nil, want canonical session")
	}
	if *fragment.ProviderSessionRef != *wantSession {
		t.Fatalf("provider session = %#v, want %#v", fragment.ProviderSessionRef, wantSession)
	}
}

func agyTimeoutPartialDraftPublished(published []InferenceProgressFragment) bool {
	for _, fragment := range published {
		draft, ok := fragment.CanonicalDraft.(factorysessions.ResponseEventDraft)
		if !ok || draft.Kind != factorysessions.ResponseEventKindMessage || draft.Phase != factorysessions.ResponseEventPhaseCompleted {
			continue
		}
		var payload factorysessions.ResponseEventMessage
		if err := json.Unmarshal(draft.Payload, &payload); err != nil || !payload.Partial {
			continue
		}
		if len(payload.ContentBlocks) == 1 && payload.ContentBlocks[0].Text == "partial answer before timeout" {
			return true
		}
	}
	return false
}

type agyInferenceStubSession struct {
	launch agypty.ProcessLaunch
	result agypty.SessionResult
}

func (s *agyInferenceStubSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *agyInferenceStubSession) Close() error { return nil }

type agyInferenceStubAllocator struct {
	sessions []*agyInferenceStubSession
	result   agypty.SessionResult
}

func (a *agyInferenceStubAllocator) Allocate(_ context.Context, launch agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	session := &agyInferenceStubSession{launch: launch, result: a.result}
	a.sessions = append(a.sessions, session)
	return session, nil
}
