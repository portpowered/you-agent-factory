package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workcontent/materialize"
)

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
