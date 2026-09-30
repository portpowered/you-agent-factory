package inference

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestManagedInferenceLoadsOrderedFileMediaBytes(t *testing.T) {
	media := []struct {
		kind  work.WorkContentPartType
		mime  string
		bytes []byte
	}{
		{work.WorkContentPartTypeImage, "image/png", []byte{0, 1, 255}},
		{work.WorkContentPartTypeAudio, "audio/wav", []byte{2, 0, 254}},
		{work.WorkContentPartTypeVideo, "video/mp4", []byte{3, 0, 253}},
	}
	paths := make(map[string]string)
	for _, item := range media {
		path := filepath.Join(t.TempDir(), string(item.kind))
		if err := os.WriteFile(path, item.bytes, 0600); err != nil {
			t.Fatal(err)
		}
		paths["file:///"+string(item.kind)] = path
	}
	cleaned := 0
	materializer := work.ContentMaterializeFunc(func(_ context.Context, url string) (string, work.ContentCleanup, error) {
		path := paths[url]
		if path == "" {
			t.Fatalf("unexpected materialization URL %q", url)
		}
		return path, func() { cleaned++ }, nil
	})
	model := &captureModelsService{result: models.InvokeModelResult{Status: models.ModelInvocationStatusCompleted, Outputs: []models.InferenceOutput{{Name: "text", Modality: models.ModalityText, Content: "ok"}}}}
	runner, err := New(validConfig(), Dependencies{Models: model, ContentMaterializer: materializer, MediaFiles: platformfilesystem.Local{}})
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	for _, item := range media {
		request.ModelBindings = append(request.ModelBindings, workers.ResolvedModelOperationBinding{Slot: "media", Source: workers.ModelOperationBindingSourceInput, Content: []work.WorkContentPart{{Type: item.kind, URL: "file:///" + string(item.kind), ContentType: item.mime}}})
	}
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if cleaned != len(media) {
		t.Fatalf("cleanup calls = %d, want %d", cleaned, len(media))
	}
	inputs := model.Request().Inputs
	if len(inputs) != len(media)+1 {
		t.Fatalf("inputs = %#v", inputs)
	}
	for i, item := range media {
		input := inputs[i+1]
		if input.Content != string(item.bytes) || input.ContentType != item.mime || input.MediaType != item.mime || strings.HasPrefix(input.Content, "file://") {
			t.Fatalf("media input[%d] = %#v", i, input)
		}
	}
}

func TestManagedInferenceRejectsOversizeAndCleansUp(t *testing.T) {
	cleaned := 0
	materializer := work.ContentMaterializeFunc(func(context.Context, string) (string, work.ContentCleanup, error) {
		return "fixture", func() { cleaned++ }, nil
	})
	model := &captureModelsService{}
	runner, err := New(validConfig(), Dependencies{Models: model, ContentMaterializer: materializer, MediaFiles: mediaReadOpener{open: func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxInferenceMediaBytes+1)))), nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	request.ModelBindings = append(request.ModelBindings, workers.ResolvedModelOperationBinding{Slot: "video", Source: workers.ModelOperationBindingSourceInput, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeVideo, URL: "file:///oversize", ContentType: "video/mp4"}}})
	_, err = runner.Execute(t.Context(), request)
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Cause == nil || !strings.Contains(providerErr.Cause.Error(), "exceeds") {
		t.Fatalf("error = %v, want size rejection", err)
	}
	if model.Calls() != 0 || cleaned != 1 {
		t.Fatalf("Models calls = %d, cleanup = %d", model.Calls(), cleaned)
	}
}

func TestManagedInferenceCleansUpOnReadFailureAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "read failure", true: "cancellation"}[cancel], func(t *testing.T) {
			ctx, cancelContext := context.WithCancel(t.Context())
			defer cancelContext()
			cleaned := 0
			materializer := work.ContentMaterializeFunc(func(context.Context, string) (string, work.ContentCleanup, error) {
				return "fixture", func() { cleaned++ }, nil
			})
			model := &captureModelsService{}
			runner, err := New(validConfig(), Dependencies{Models: model, ContentMaterializer: materializer, MediaFiles: mediaReadOpener{open: func(string) (io.ReadCloser, error) {
				if cancel {
					cancelContext()
					return io.NopCloser(strings.NewReader("bytes")), nil
				}
				return nil, errors.New("open failed")
			}}})
			if err != nil {
				t.Fatal(err)
			}
			request := validRequest()
			request.ModelBindings = append(request.ModelBindings, workers.ResolvedModelOperationBinding{Slot: "image", Source: workers.ModelOperationBindingSourceInput, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeImage, URL: "file:///image"}}})
			_, err = runner.Execute(ctx, request)
			if err == nil || cleaned != 1 || model.Calls() != 0 {
				t.Fatalf("error = %v, cleanup = %d, Models calls = %d", err, cleaned, model.Calls())
			}
		})
	}
}

func TestManagedInferenceMaterializesDataURLAndLeavesArtifactRefsUntouched(t *testing.T) {
	artifact, err := (models.InferenceArtifactRef{}).Parse("models-inference:artifact:existing")
	if err != nil {
		t.Fatal(err)
	}
	materializer := work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
		if rawURL != "data:image/png;base64,AQID" {
			t.Fatalf("materialized URL = %q", rawURL)
		}
		path := filepath.Join(t.TempDir(), "inline.png")
		if err := os.WriteFile(path, []byte{1, 2, 3}, 0600); err != nil {
			t.Fatal(err)
		}
		return path, func() {}, nil
	})
	inputs := []models.InferenceInput{
		{Name: "inline", Modality: models.ModalityImage, Content: "data:image/png;base64,AQID"},
		{Name: "artifact", Modality: models.ModalityAudio, Artifact: &artifact},
	}
	if err := (&runner{contentMaterializer: materializer, mediaFiles: platformfilesystem.Local{}}).materializeMediaInputs(t.Context(), inputs); err != nil {
		t.Fatal(err)
	}
	if inputs[0].Content != string([]byte{1, 2, 3}) || inputs[1].Artifact != &artifact || inputs[1].Content != "" {
		t.Fatalf("media input materialization = %#v", inputs)
	}
}

type mediaReadOpener struct {
	open func(string) (io.ReadCloser, error)
}

func (o mediaReadOpener) Open(path string) (io.ReadCloser, error) { return o.open(path) }
