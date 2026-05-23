package models

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestRenderList_WritesDiscoveredModelsTable(t *testing.T) {
	var out bytes.Buffer
	err := RenderList(factoryapi.ListModelsResponse{
		Results: []factoryapi.ModelSummary{{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
			Status:           factoryapi.ModelStatusREADY,
			LoadState:        factoryapi.UNLOADED,
			Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
			Modalities:       []factoryapi.ModelOperationContentType{factoryapi.ModelOperationContentTypeAudio, factoryapi.ModelOperationContentTypeText},
			Resources:        []factoryapi.ModelResourceSummary{{Name: "voice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
		}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "OMNIVOICE_Q4_K_M", "LOCAL", "READY", "UNLOADED", "TTS", "AUDIO,TEXT"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("rendered table missing %q:\n%s", want, got)
		}
	}
}

func TestQueryModel_NotFoundUsesFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	_, err := QueryModel(port, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInvoke_JSONWritesMetadataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/invocations" {
			t.Fatalf("path = %q, want invocation path", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","worker":"tts-worker","operation":"TTS","providerLocality":"LOCAL","content":[{"type":"AUDIO","file":"artifacts/output.wav"}],"bindings":[]}`)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	var out bytes.Buffer
	if err := Invoke(InvokeConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		Port:      port,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"operation":"TTS"`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInvoke_AudioWritesOutputFile(t *testing.T) {
	audioBytes := []byte("RIFF....WAVE")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(audioBytes)
	}))
	defer server.Close()

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	port := server.Listener.Addr().(*net.TCPAddr).Port
	if err := Invoke(InvokeConfig{
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		Port:       port,
		Output:     io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !bytes.Equal(got, audioBytes) {
		t.Fatalf("output bytes = %q, want %q", got, audioBytes)
	}
}

func TestPull_JSONWritesPullMetadataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want pull path", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}]}`)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	var out bytes.Buffer
	if err := Pull(PullConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Port:      port,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"outcome":"PULLED"`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}
