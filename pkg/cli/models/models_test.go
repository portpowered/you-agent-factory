package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestModelsList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[]}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	if err := List(ListConfig{
		Port:        server.Listener.Addr().(*net.TCPAddr).Port,
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models list request",
		"endpointPath=/models",
		"port=",
		"models list response",
		"status=200",
		"resultCount=1",
	})
}

func TestModelsVerboseLogsInspectInvokeAndPullMetadataWithoutInputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/OMNIVOICE_Q4_K_M":
			_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"capabilities":[],"diagnostics":{}}`)
		case "/models/OMNIVOICE_Q4_K_M/invocations":
			_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","worker":"tts-worker","operation":"TTS","providerLocality":"LOCAL","content":[{"type":"AUDIO","file":"artifacts/sensitive-generated-output.wav"}],"bindings":[]}`)
		case "/models/OMNIVOICE_Q4_K_M/pull":
			_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/ghp_successResponseToken1234567890/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	var diagnostics bytes.Buffer
	if err := Inspect(InspectConfig{ModelName: "OMNIVOICE_Q4_K_M", Port: port, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Invoke(InvokeConfig{ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS", Text: "secret direct input", Port: port, JSON: true, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := Pull(PullConfig{ModelName: "OMNIVOICE_Q4_K_M", Port: port, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect request",
		"modelName=\"OMNIVOICE_Q4_K_M\"",
		"status=READY",
		"models invoke request",
		"operation=\"TTS\"",
		"worker=tts-worker",
		"models pull request",
		"outcome=PULLED",
		"downloadedFiles=1",
	})
	for _, forbidden := range []string{"secret direct input", "sensitive-generated-output.wav", "ghp_successResponseToken1234567890"} {
		if strings.Contains(diag, forbidden) {
			t.Fatalf("diagnostics leaked model input, response content, or token %q:\n%s", forbidden, diag)
		}
	}
}

func TestModelsVerboseFailureUsesBoundedNonJSONErrorPreview(t *testing.T) {
	longFailureBody := strings.Repeat("x", modelsErrorBodyPreviewSize+30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, longFailureBody)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Port:        server.Listener.Addr().(*net.TCPAddr).Port,
		ModelName:   "broken",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected queryModel to fail")
	}
	gotErr := err.Error()
	wantPreview := longFailureBody[:modelsErrorBodyPreviewSize] + "..."
	if !strings.Contains(gotErr, wantPreview) {
		t.Fatalf("error = %q, want bounded preview %q", gotErr, wantPreview)
	}
	if strings.Contains(gotErr, longFailureBody) {
		t.Fatalf("error included full response body")
	}
	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect response",
		"endpointPath=/models/broken",
		"status=502",
		"responseBytes=230",
	})
	if strings.Contains(diag, longFailureBody[:40]) {
		t.Fatalf("diagnostics leaked model input or response content:\n%s", diag)
	}
}

func TestModelsVerboseLogsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Port:        server.Listener.Addr().(*net.TCPAddr).Port,
		ModelName:   "missing",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("queryModel error = %v, want ErrModelNotFound", err)
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models inspect response",
		"endpointPath=/models/missing",
		"status=404",
	})
}

func assertDiagnosticsContains(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}
