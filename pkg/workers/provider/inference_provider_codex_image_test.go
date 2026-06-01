package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workcontent/materialize"
)

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
	if providerErr.Type != interfaces.ProviderErrorTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.ProviderErrorTypePermanentBadRequest)
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
	if providerErr.Type != interfaces.ProviderErrorTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.ProviderErrorTypePermanentBadRequest)
	}
	if !strings.Contains(providerErr.Message, `input_tokens[0].color.content[0].url`) ||
		!strings.Contains(providerErr.Message, `media url inaccessible`) {
		t.Fatalf("provider error message = %q", providerErr.Message)
	}
	if fakeExec.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", fakeExec.calls)
	}
}
