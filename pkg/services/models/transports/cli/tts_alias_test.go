package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

func TestRootAdapter_DirectTTSAliasMatchesNamedGenericAudioOutput(t *testing.T) {
	t.Parallel()

	wantAudio := []byte{0x00, 0xff, 0x10, 0x80, 0x7f, 0x01, 0x00}
	var requests []modelinference.InvokeModelRequest
	service := newTTSGenericCLIService(t, func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
		requests = append(requests, request)
		return modelinference.InvokeModelResult{
			Outputs: []modelinference.InferenceOutput{{
				Name: "audio", Modality: modelinference.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: string(wantAudio),
			}},
		}, nil
	})

	genericPath := filepath.Join(t.TempDir(), "generic.wav")
	var genericOutput bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS,
		Operation: modelinference.OperationTTS, InputMappings: []string{"text=hello"},
		OutputPath: genericPath, Output: &genericOutput,
	}); err != nil {
		t.Fatalf("generic --input invocation error = %v", err)
	}

	aliasPath := filepath.Join(t.TempDir(), "alias.wav")
	var aliasOutput bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS,
		Operation: modelinference.OperationTTS, Text: "hello", OutputPath: aliasPath,
		Output: &aliasOutput,
	}); err != nil {
		t.Fatalf("direct --text/--output invocation error = %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		out  *bytes.Buffer
	}{
		{name: "generic", path: genericPath, out: &genericOutput},
		{name: "alias", path: aliasPath, out: &aliasOutput},
	} {
		got, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatalf("read %s output: %v", test.name, err)
		}
		if !bytes.Equal(got, wantAudio) {
			t.Fatalf("%s output = %v, want exact audio %v", test.name, got, wantAudio)
		}
		wantStatus := "Wrote audio: " + test.path + "\n"
		if test.out.String() != wantStatus {
			t.Fatalf("%s stdout = %q, want %q", test.name, test.out.String(), wantStatus)
		}
	}
	if len(requests) != 2 {
		t.Fatalf("generic invocation count = %d, want two", len(requests))
	}
	for index, request := range requests {
		if request.Model.NameOrURI != modelinference.BuiltInModelNameTTS || request.Operation != modelinference.OperationTTS {
			t.Fatalf("request[%d] identity = %#v, want tts/TTS", index, request)
		}
		if len(request.Inputs) != 1 || request.Inputs[0].Name != "text" ||
			request.Inputs[0].Modality != modelinference.ModalityText || request.Inputs[0].Content != "hello" {
			t.Fatalf("request[%d] inputs = %#v, want one named text input", index, request.Inputs)
		}
	}
	if requests[0].Model != requests[1].Model || requests[0].Operation != requests[1].Operation ||
		requests[0].Inputs[0] != requests[1].Inputs[0] {
		t.Fatalf("generic and alias requests differ:\ngeneric=%#v\nalias=%#v", requests[0], requests[1])
	}
}

func TestRootAdapter_DirectTTSAudioStdoutIsRawAndDiagnosticFree(t *testing.T) {
	t.Parallel()

	wantAudio := []byte{0x00, 0xff, 0x0a, 0x80, 0x7f}
	service := newTTSGenericCLIService(t, func(_ context.Context, _ modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
		return modelinference.InvokeModelResult{
			Outputs: []modelinference.InferenceOutput{{
				Name: "audio", Modality: modelinference.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: string(wantAudio),
			}},
		}, nil
	})

	var stdout bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS,
		Operation: modelinference.OperationTTS, Text: "pipe me", Output: &stdout,
	}); err != nil {
		t.Fatalf("direct TTS stdout invocation error = %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), wantAudio) {
		t.Fatalf("stdout = %v, want exact raw audio %v", stdout.Bytes(), wantAudio)
	}
}

func TestCompositionFacade_RoutesDirectTTSAliasThroughOwnedGenericPath(t *testing.T) {
	t.Parallel()

	wantAudio := []byte{0x52, 0x49, 0xff, 0x46}
	invoked := false
	root := compositionModelsRoot{
		getModel: func(_ context.Context, name string) (modelinference.Detail, error) {
			return ttsGenericCLIDetail(name), nil
		},
		invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
			invoked = true
			return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
				Name: "audio", Modality: modelinference.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: string(wantAudio),
			}}}, nil
		},
	}
	service := modelscli.NewWithOutputFileSystem(
		compositionHTTPProtocol(t), compositionInvocation{root: root}, localOutputFileSystem{},
	)
	if service == nil {
		t.Fatal("NewWithOutputFileSystem() = nil, want composition facade")
	}

	path := filepath.Join(t.TempDir(), "tts.wav")
	var stdout bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameTTS,
		Operation: modelinference.OperationTTS, Text: "hello", OutputPath: path, Output: &stdout,
	}); err != nil {
		t.Fatalf("composition direct TTS invocation error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read composition TTS output: %v", err)
	}
	if !bytes.Equal(got, wantAudio) || !invoked {
		t.Fatalf("composition output = %v, owned generic invoked = %v; want exact audio and true", got, invoked)
	}
	if stdout.String() != "Wrote audio: "+path+"\n" {
		t.Fatalf("composition stdout = %q, want Wrote audio status", stdout.String())
	}
}

func newTTSGenericCLIService(
	t *testing.T,
	invoke func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error),
) modelscli.Service {
	t.Helper()
	scope := testRuntimeScope(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: ttsGenericCLIDetail(request.Name)}, nil
			},
			invokeModel: invoke,
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: localOutputFileSystem{},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want TTS generic service")
	}
	return service
}

func ttsGenericCLIDetail(name string) modelinference.Detail {
	required := true
	return modelinference.Detail{Summary: modelinference.Summary{
		Name: name,
		Operations: []modelinference.Operation{{
			Name: modelinference.OperationTTS,
			Inputs: []modelinference.OperationSlot{{
				Name: "text", Modality: modelinference.ModalityText,
				ContentTypes: []string{"TEXT"}, MediaTypes: []string{"text/plain"}, Required: &required,
			}},
			Outputs: []modelinference.OperationSlot{{
				Name: "audio", Modality: modelinference.ModalityAudio,
				ContentTypes: []string{"AUDIO"}, MediaTypes: []string{"audio/wav"},
			}},
		}},
	}}
}
