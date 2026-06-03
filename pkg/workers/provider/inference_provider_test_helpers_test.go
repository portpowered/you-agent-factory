package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workcontent/materialize"
)

func InputTokens(tokens ...interfaces.Token) []any {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]any, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, token)
	}
	return out
}

func CommandRequestInputTokens(request CommandRequest) []interfaces.Token {
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

func assertExecutionMetadataEqual(t *testing.T, want, got interfaces.ExecutionMetadata) {
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
	Name                  string                       `json:"name"`
	ExitCode              int                          `json:"exit_code"`
	Stdout                string                       `json:"stdout"`
	Stderr                string                       `json:"stderr"`
	WantType              interfaces.WorkFailureType `json:"want_type"`
	WantMessage           string                       `json:"want_message"`
	WantRetryable         bool                         `json:"want_retryable"`
	WantTerminal          bool                         `json:"want_terminal"`
	WantThrottlePause     bool                         `json:"want_throttle_pause"`
	RejectMessageContains []string                     `json:"reject_message_contains"`
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
	r.request = CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
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

	result, err := ExecCommandRunner{}.Run(ctx, CommandRequest{
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

func writeEditorMarkerScript(t *testing.T, markerPath string) string {
	t.Helper()
	dir := t.TempDir()
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		script := filepath.Join(dir, "editor.bat")
		content := "@echo off\r\necho invoked > %1\r\nexit /b 42\r\n"
		if err := os.WriteFile(script, []byte(content), 0755); err != nil {
			t.Fatalf("writing editor marker script: %v", err)
		}
		return script + " " + markerPath
	}

	script := filepath.Join(dir, "editor.sh")
	content := "#!/bin/sh\nprintf invoked > \"$1\"\nexit 42\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("writing editor marker script: %v", err)
	}
	return script + " " + markerPath
}

func providerGitTestEnv(envVars map[string]string) []string {
	return isolatedGitEnv(buildProviderEnv(envVars))
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
	imageOneURL, err := workcontent.FilesystemPathToContentURL(imageOne)
	if err != nil {
		t.Fatalf("image one url: %v", err)
	}
	imageTwoURL, err := workcontent.FilesystemPathToContentURL(imageTwo)
	if err != nil {
		t.Fatalf("image two url: %v", err)
	}

	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err = provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect the images",
		WorkingDirectory: workspace,
		InputTokens: InputTokens(
			interfaces.Token{
				ID: "token-1",
				Color: interfaces.TokenColor{
					Content: []interfaces.WorkContentPart{
						{Type: interfaces.WorkContentPartTypeText, Text: "before"},
						{Type: interfaces.WorkContentPartTypeImage, URL: imageOneURL},
					},
				},
			},
			interfaces.Token{
				ID: "token-2",
				Color: interfaces.TokenColor{
					Content: []interfaces.WorkContentPart{
						{Type: interfaces.WorkContentPartTypeImage, URL: imageTwoURL},
						{Type: interfaces.WorkContentPartTypeText, Text: "after"},
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
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "text only",
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeText, Text: "only text"},
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
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	missingURL, err := workcontent.FilesystemPathToContentURL("fixtures/missing.png")
	if err != nil {
		t.Fatalf("missing url: %v", err)
	}

	_, err = provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect",
		WorkingDirectory: t.TempDir(),
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeImage, URL: missingURL},
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
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.WorkFailureTypePermanentBadRequest)
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
	body := []byte("remote-image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	fakeExec := &codexImageMaterializationAssertExec{
		recordingProviderExec: recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}},
		wantBody:              body,
	}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithMaterializeOptions(&materialize.Options{
			AllowPrivateURLs: true,
			HTTPClient:       server.Client(),
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "inspect remote image",
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeImage, URL: server.URL},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if fakeExec.imagePath == server.URL {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithMaterializeOptions(&materialize.Options{
			AllowPrivateURLs: true,
			HTTPClient:       server.Client(),
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "inspect remote image",
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeImage, URL: server.URL},
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
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.WorkFailureTypePermanentBadRequest)
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
	workspace := t.TempDir()
	localPath := filepath.Join(workspace, "local.png")
	if err := os.WriteFile(localPath, []byte("local-image"), 0o644); err != nil {
		t.Fatalf("write local image: %v", err)
	}
	localURL, err := workcontent.FilesystemPathToContentURL(localPath)
	if err != nil {
		t.Fatalf("local content url: %v", err)
	}

	remoteBody := []byte("remote-image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(remoteBody)
	}))
	defer server.Close()

	fakeExec := &codexMixedImageAssertExec{
		recordingProviderExec: recordingProviderExec{result: CommandResult{Stdout: []byte("codex output")}},
		wantLocalPath:         localPath,
		wantRemoteBody:        remoteBody,
	}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithMaterializeOptions(&materialize.Options{
			AllowPrivateURLs: true,
			HTTPClient:       server.Client(),
		}),
	)

	_, err = provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "inspect both images",
		WorkingDirectory: workspace,
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeImage, URL: localURL},
					{Type: interfaces.WorkContentPartTypeImage, URL: server.URL},
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
