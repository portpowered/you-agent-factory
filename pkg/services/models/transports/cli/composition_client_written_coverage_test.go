package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRootAdapter_InvokeASRExplicitMappingsPublishClientWrittenIdentity(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	outputDir := t.TempDir()
	transcriptPath := filepath.Join(outputDir, "transcript.txt")
	segmentsPath := filepath.Join(outputDir, "segments.json")
	wantTranscript := "ASR transcript"
	wantSegments := `[{"text":"ASR transcript","start":0}]`
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel("asr", modelinference.OperationASR,
					[]modelinference.OperationSlot{{
						Name: "audio", Modality: modelinference.ModalityAudio,
						Required: boolPointer(true), MediaTypes: []string{"audio/*"},
					}},
					[]modelinference.OperationSlot{
						{Name: "transcript", Modality: modelinference.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}},
						{Name: "segments", Modality: modelinference.ModalityJSON, Required: boolPointer(true), MediaTypes: []string{"application/json"}},
					}), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if err := validateASRMappedRequest(request); err != nil {
					return modelinference.InvokeModelResult{}, err
				}
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "transcript", Modality: modelinference.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: wantTranscript},
					{Name: "segments", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: wantSegments},
				}}, nil
			},
		},
		InputFileReader: func(context.Context, string, int64) ([]byte, error) { return []byte("RIFF-fixture"), nil },
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: localOutputFileSystem{},
	})

	var output bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings:  []string{"audio=@meeting.wav"},
		OutputMappings: []string{"transcript=" + transcriptPath, "segments=" + segmentsPath}, Output: &output,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertClientWrittenFileIdentity(t, transcriptPath, wantTranscript, "text/plain", "sha256:b6e970499c43915bfb6c9ed275ab97b8c0c513f417bd92b38d847f60f4e32444")
	assertClientWrittenFileIdentity(t, segmentsPath, wantSegments, "application/json", "sha256:dde773107fd7fbefbc90259b6490f88c6a7a053071deb516fd278054251bbb10")
	assertASRClientWrittenResponse(t, output.Bytes(), wantTranscript, wantSegments)
}

func validateASRMappedRequest(request modelinference.InvokeModelRequest) error {
	if request.Operation != modelinference.OperationASR || len(request.Inputs) != 1 || request.Inputs[0].Name != "audio" {
		return fmt.Errorf("unexpected ASR request: %#v", request)
	}
	return nil
}

func assertASRClientWrittenResponse(t *testing.T, data []byte, wantTranscript, wantSegments string) {
	t.Helper()
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode ASR mapped response: %v", err)
	}
	if len(response.Outputs) != 2 {
		t.Fatalf("ASR mapped response outputs = %#v, want transcript and segments", response.Outputs)
	}
	for index, want := range []struct {
		name      string
		modality  factoryapi.ModelInvocationContentType
		mediaType string
		content   string
	}{
		{name: "transcript", modality: factoryapi.ModelInvocationContentTypeText, mediaType: "text/plain", content: wantTranscript},
		{name: "segments", modality: factoryapi.ModelInvocationContentTypeJSON, mediaType: "application/json", content: wantSegments},
	} {
		got := response.Outputs[index]
		if got.Name != want.name || got.Modality != want.modality || got.MediaType == nil || *got.MediaType != want.mediaType || got.Content == nil || *got.Content != want.content || got.Artifact != nil {
			t.Fatalf("ASR mapped response output[%d] = %#v, want inline %s metadata without artifact reference", index, got, want.name)
		}
	}
	if strings.Contains(string(data), "artifactRef") {
		t.Fatalf("ASR client-written response = %q, must not fabricate an artifactRef", data)
	}
}

func TestRootAdapter_InvokeASRExplicitMappingsRollbackClientWrittenFilesOnPublishFailure(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	outputDir := t.TempDir()
	transcriptPath := filepath.Join(outputDir, "transcript.txt")
	segmentsPath := filepath.Join(outputDir, "segments.json")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel("asr", modelinference.OperationASR,
					[]modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio, Required: boolPointer(true), MediaTypes: []string{"audio/*"}}},
					[]modelinference.OperationSlot{
						{Name: "transcript", Modality: modelinference.ModalityText, Required: boolPointer(true)},
						{Name: "segments", Modality: modelinference.ModalityJSON, Required: boolPointer(true)},
					}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "transcript", Modality: modelinference.ModalityText, Content: "ASR transcript"},
					{Name: "segments", Modality: modelinference.ModalityJSON, Content: `[{"text":"ASR transcript","start":0}]`},
				}}, nil
			},
		},
		InputFileReader: func(context.Context, string, int64) ([]byte, error) { return []byte("RIFF-fixture"), nil },
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: &renameFailureOutputFileSystem{localOutputFileSystem: localOutputFileSystem{}, failAt: 2},
	})

	var output bytes.Buffer
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "asr", Operation: modelinference.OperationASR,
		InputMappings:  []string{"audio=@meeting.wav"},
		OutputMappings: []string{"transcript=" + transcriptPath, "segments=" + segmentsPath}, Output: &output,
	})
	if err == nil || !strings.Contains(err.Error(), "publish output") {
		t.Fatalf("Invoke() error = %v, want publication failure", err)
	}
	for _, path := range []string{transcriptPath, segmentsPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed publication left %s with stat error %v", path, statErr)
		}
	}
	entries, readErr := os.ReadDir(outputDir)
	if readErr != nil {
		t.Fatalf("read output directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed publication left temporary files: %#v", entries)
	}
	if output.Len() != 0 {
		t.Fatalf("failed publication wrote a success response: %q", output.String())
	}
}

type renameFailureOutputFileSystem struct {
	localOutputFileSystem
	calls  int
	failAt int
}

func (fs *renameFailureOutputFileSystem) Rename(oldPath, newPath string) error {
	fs.calls++
	if fs.calls >= fs.failAt {
		return errors.New("publish output blocked")
	}
	return fs.localOutputFileSystem.Rename(oldPath, newPath)
}

func assertClientWrittenFileIdentity(t *testing.T, path, want, mediaType, digest string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mapped output %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("mapped output %s = %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat mapped output %s: %v", path, err)
	}
	if info.Size() != int64(len(got)) {
		t.Fatalf("mapped output %s size = %d, want %d", path, info.Size(), len(got))
	}
	gotMediaType := mime.TypeByExtension(filepath.Ext(path))
	if strings.SplitN(gotMediaType, ";", 2)[0] != mediaType {
		t.Fatalf("mapped output %s MIME = %q, want %q", path, gotMediaType, mediaType)
	}
	digestBytes := sha256.Sum256(got)
	gotDigest := "sha256:" + hex.EncodeToString(digestBytes[:])
	if gotDigest != digest {
		t.Fatalf("mapped output %s digest = %q, want %q", path, gotDigest, digest)
	}
}
