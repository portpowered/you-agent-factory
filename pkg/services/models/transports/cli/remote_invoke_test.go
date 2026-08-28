package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestHTTPServiceInvokeRoutesNamedInputsWithExactBinaryOrder(t *testing.T) {
	t.Parallel()

	png := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff, 0x01, 0x02}
	wantHash := sha256.Sum256(png)
	var postCalls int
	var received factoryapi.GenericModelInvocationRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/models/llm":
			_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
		case request.Method == http.MethodPost && request.URL.Path == "/models/invocations":
			postCalls++
			if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
				t.Fatalf("decode generic request: %v", err)
			}
			prompt := "Describe this image"
			if received.Inputs == nil || len(*received.Inputs) != 2 {
				t.Fatalf("received inputs = %#v, want prompt and image", received.Inputs)
			}
			inputs := *received.Inputs
			if inputs[0].Name != "prompt" || inputs[0].Content == nil || *inputs[0].Content != prompt {
				t.Fatalf("received prompt input = %#v, want ordered inline prompt", inputs[0])
			}
			if inputs[1].Name != "image" || inputs[1].ContentBase64 == nil || inputs[1].Content != nil {
				t.Fatalf("received image input = %#v, want binary carrier only", inputs[1])
			}
			if !bytes.Equal(*inputs[1].ContentBase64, png) {
				t.Fatalf("received image bytes hash = %s, want %s", hashBytes(*inputs[1].ContentBase64), hex.EncodeToString(wantHash[:]))
			}
			answer := "The image is a PNG fixture."
			_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
				Outputs: []factoryapi.ModelInvocationOutput{{
					Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
				}},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	var output, diagnostics bytes.Buffer
	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(_ context.Context, path string, maxBytes int64) ([]byte, error) {
			if path != "fixture.png" {
				return nil, fmt.Errorf("unexpected input path %q", path)
			}
			if maxBytes != genericCLIInputMaxFileBytes {
				return nil, fmt.Errorf("input limit = %d, want %d", maxBytes, genericCLIInputMaxFileBytes)
			}
			return append([]byte(nil), png...), nil
		},
	}
	serverWithSignedQuery := server.URL + "?signature=super-secret"
	if err := service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: "llm", Operation: "", Server: serverWithSignedQuery,
		InputMappings: []string{"prompt=Describe this image", "image=@fixture.png"},
		Output:        &output, Verbose: true, Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("remote generic invoke error = %v", err)
	}
	if got, want := output.String(), "The image is a PNG fixture."; got != want {
		t.Fatalf("remote generic stdout = %q, want %q", got, want)
	}
	if postCalls != 1 {
		t.Fatalf("generic POST calls = %d, want one", postCalls)
	}
	if strings.Contains(diagnostics.String(), "Describe this image") || strings.Contains(diagnostics.String(), hex.EncodeToString(wantHash[:])) || strings.Contains(diagnostics.String(), "super-secret") {
		t.Fatalf("diagnostics leaked input content: %q", diagnostics.String())
	}
}

func TestHTTPServiceInvokeRoutesNamedInputsToJSONResponse(t *testing.T) {
	t.Parallel()

	var postCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/models/invocations" {
			http.NotFound(writer, request)
			return
		}
		postCalls++
		var received factoryapi.GenericModelInvocationRequest
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatalf("decode generic JSON request: %v", err)
		}
		answer := "json answer"
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
			Outputs: []factoryapi.ModelInvocationOutput{{
				Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
			}},
		})
	}))
	t.Cleanup(server.Close)

	var output bytes.Buffer
	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(_ context.Context, _ string, _ int64) ([]byte, error) {
			return []byte("png bytes"), nil
		},
	}
	if err := service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
		InputMappings: []string{"prompt=hello", "image=@fixture.png"}, Server: server.URL, JSON: true, Output: &output,
	}); err != nil {
		t.Fatalf("remote generic JSON invoke error = %v", err)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode remote generic JSON stdout: %v", err)
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "json answer" {
		t.Fatalf("remote generic JSON response = %#v, want named text output", response)
	}
	if postCalls != 1 {
		t.Fatalf("generic JSON POST calls = %d, want one", postCalls)
	}
}

func TestHTTPServiceInvokePreservesInlineJSONAndRepeatedMediaInputs(t *testing.T) {
	t.Parallel()

	firstImage := []byte{0x89, 'P', 'N', 'G', 0x01}
	secondImage := []byte{0x89, 'P', 'N', 'G', 0x02}
	var received factoryapi.GenericModelInvocationRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/models/invocations" {
			http.NotFound(writer, request)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatalf("decode repeated generic request: %v", err)
		}
		if received.Inputs == nil || len(*received.Inputs) != 4 {
			t.Fatalf("received inputs = %#v, want prompt, JSON, and two images", received.Inputs)
		}
		inputs := *received.Inputs
		if inputs[0].Name != "prompt" || inputs[0].Content == nil || *inputs[0].Content != "hello" {
			t.Fatalf("prompt input = %#v, want inline text", inputs[0])
		}
		if inputs[1].Name != "parameters" || inputs[1].Content == nil || *inputs[1].Content != `{"normalize":true}` {
			t.Fatalf("JSON input = %#v, want canonical JSON content", inputs[1])
		}
		if inputs[2].Name != "image" || inputs[3].Name != "image" {
			t.Fatalf("repeated image inputs = %#v, want authored order", inputs[2:])
		}
		if inputs[2].ContentBase64 == nil || inputs[3].ContentBase64 == nil ||
			!bytes.Equal(*inputs[2].ContentBase64, firstImage) || !bytes.Equal(*inputs[3].ContentBase64, secondImage) {
			t.Fatalf("repeated image bytes = %#v, want exact ordered bytes", inputs[2:])
		}
		answer := "ordered inputs accepted"
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
			Outputs: []factoryapi.ModelInvocationOutput{{
				Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
			}},
		})
	}))
	t.Cleanup(server.Close)

	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(_ context.Context, path string, _ int64) ([]byte, error) {
			switch path {
			case "first.png":
				return append([]byte(nil), firstImage...), nil
			case "second.png":
				return append([]byte(nil), secondImage...), nil
			default:
				return nil, fmt.Errorf("unexpected input path %q", path)
			}
		},
	}
	var output bytes.Buffer
	if err := service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
		InputMappings: []string{
			"prompt=hello", `parameters=json:{"normalize":true}`, "image=@first.png", "image=@second.png",
		}, Server: server.URL, Output: &output,
	}); err != nil {
		t.Fatalf("remote repeated generic invoke error = %v", err)
	}
	if output.String() != "ordered inputs accepted" {
		t.Fatalf("remote repeated generic stdout = %q, want accepted response", output.String())
	}
}

func TestHTTPServiceInvokeInputFailuresDoNotPostGenericInvocation(t *testing.T) {
	t.Parallel()

	var postCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
			return
		}
		postCalls++
		http.Error(writer, "unexpected invocation", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	reader := func(_ context.Context, path string, maxBytes int64) ([]byte, error) {
		switch path {
		case "missing.png":
			return nil, errors.New("file does not exist")
		case "empty.png":
			return []byte{}, nil
		case "large.png":
			return bytes.Repeat([]byte{'x'}, int(maxBytes+1)), nil
		default:
			return []byte("not an image"), nil
		}
	}
	service := &httpService{http: testHTTPProtocol(t), inputFileReader: reader}
	for _, testCase := range []struct {
		name   string
		inputs []string
		want   string
		cause  string
	}{
		{name: "malformed", inputs: []string{"prompt"}, want: "expected slot=value"},
		{name: "missing required", inputs: []string{"prompt=hello"}, want: "required input slot is missing"},
		{name: "duplicate singleton", inputs: []string{"prompt=one", "prompt=two", "image=@ok.png"}, want: "at most one value"},
		{name: "unsupported inline media", inputs: []string{"prompt=hello", "image=inline"}, want: "requires a file value"},
		{name: "unreadable", inputs: []string{"prompt=hello", "image=@missing.png"}, want: "failed to load --input input", cause: "file does not exist"},
		{name: "empty", inputs: []string{"prompt=hello", "image=@empty.png"}, want: "failed to load --input input", cause: "file is empty"},
		{name: "over limit", inputs: []string{"prompt=hello", "image=@large.png"}, want: "failed to load --input input", cause: "exceeds"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := service.Invoke(InvokeConfig{
				Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
				InputMappings: testCase.inputs, Server: server.URL, JSON: true, Output: &output,
			})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, testCase.want)
			}
			if testCase.cause != "" {
				var failure *clidiag.LocalFailure
				if !errors.As(err, &failure) || failure.Cause == nil || !strings.Contains(failure.Cause.Error(), testCase.cause) {
					t.Fatalf("Invoke() local failure = %#v, want cause %q", failure, testCase.cause)
				}
			}
			if output.Len() != 0 {
				t.Fatalf("failed invoke output = %q, want empty", output.String())
			}
		})
	}
	if postCalls != 0 {
		t.Fatalf("generic POST calls after input failures = %d, want zero", postCalls)
	}
}

func TestHTTPServiceInvokeRemoteOutageTimeoutAndCancellationAreSafeFailures(t *testing.T) {
	t.Parallel()

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		serverURL := server.URL
		server.Close()

		var output, diagnostics bytes.Buffer
		service := &httpService{http: testHTTPProtocol(t)}
		err := service.Invoke(InvokeConfig{
			Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
			InputMappings: []string{"prompt=hello", "image=@fixture.png"},
			Server:        serverURL + "?token=do-not-log", Output: &output, Verbose: true, Diagnostics: &diagnostics,
		})
		if err == nil || !strings.Contains(err.Error(), "models endpoint not reachable") {
			t.Fatalf("unreachable invoke error = %v, want safe endpoint failure", err)
		}
		if output.Len() != 0 {
			t.Fatalf("unreachable invoke output = %q, want empty", output.String())
		}
		if !strings.Contains(diagnostics.String(), "error=unreachable") || strings.Contains(diagnostics.String(), "do-not-log") {
			t.Fatalf("unreachable diagnostics = %q, want safe unreachable metadata", diagnostics.String())
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		postStarted := make(chan struct{})
		postRelease := make(chan struct{})
		postDone := make(chan struct{})
		var postOnce sync.Once
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				writeRemoteOMNICatalog(writer)
				return
			}
			if request.Method != http.MethodPost {
				http.NotFound(writer, request)
				return
			}
			postOnce.Do(func() { close(postStarted) })
			select {
			case <-request.Context().Done():
			case <-postRelease:
			}
			close(postDone)
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		var output, diagnostics bytes.Buffer
		service := &httpService{
			http: testHTTPProtocol(t),
			inputFileReader: func(context.Context, string, int64) ([]byte, error) {
				return []byte("PNG"), nil
			},
		}
		err := service.Invoke(InvokeConfig{
			Context: ctx, ModelName: "llm", Operation: modelinference.OperationOMNI,
			InputMappings: []string{"prompt=hello", "image=@fixture.png"},
			Server:        server.URL, Output: &output, Verbose: true, Diagnostics: &diagnostics,
		})
		if err == nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout invoke error = %v, want context deadline", err)
		}
		select {
		case <-postStarted:
		default:
			t.Fatal("timeout invoke did not reach the generic POST")
		}
		if output.Len() != 0 || !strings.Contains(diagnostics.String(), "error=timeout") {
			t.Fatalf("timeout output/diagnostics = %q / %q, want no output and timeout metadata", output.String(), diagnostics.String())
		}
		close(postRelease)
		select {
		case <-postDone:
		case <-time.After(time.Second):
			t.Fatal("timeout server handler did not observe request cancellation")
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		t.Parallel()
		postStarted := make(chan struct{})
		postRelease := make(chan struct{})
		postDone := make(chan struct{})
		var postOnce sync.Once
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				writeRemoteOMNICatalog(writer)
				return
			}
			if request.Method != http.MethodPost {
				http.NotFound(writer, request)
				return
			}
			postOnce.Do(func() { close(postStarted) })
			select {
			case <-request.Context().Done():
			case <-postRelease:
			}
			close(postDone)
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithCancel(context.Background())
		var output, diagnostics bytes.Buffer
		service := &httpService{
			http: testHTTPProtocol(t),
			inputFileReader: func(context.Context, string, int64) ([]byte, error) {
				return []byte("PNG"), nil
			},
		}
		done := make(chan error, 1)
		go func() {
			done <- service.Invoke(InvokeConfig{
				Context: ctx, ModelName: "llm", Operation: modelinference.OperationOMNI,
				InputMappings: []string{"prompt=hello", "image=@fixture.png"},
				Server:        server.URL, Output: &output, Verbose: true, Diagnostics: &diagnostics,
			})
		}()
		// The bounded guard keeps a broken cancellation path from hanging the test suite.
		select {
		case <-postStarted:
		case <-time.After(time.Second):
			t.Fatal("cancellation invoke did not reach the generic POST")
		}
		cancel()
		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation invoke error = %v, want context cancellation", err)
			}
		case <-time.After(time.Second):
			t.Fatal("cancellation invoke did not return after context cancellation")
		}
		if output.Len() != 0 || !strings.Contains(diagnostics.String(), "error=canceled") {
			t.Fatalf("cancellation output/diagnostics = %q / %q, want no output and cancellation metadata", output.String(), diagnostics.String())
		}
		close(postRelease)
		select {
		case <-postDone:
		case <-time.After(time.Second):
			t.Fatal("cancellation server handler did not observe request cancellation")
		}
	})
}

func TestHTTPServiceInvokeTypedServerFailuresRemainClassifiedAndDoNotSucceed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		code   string
		family factoryapi.ErrorFamily
	}{
		{name: "bad request", status: http.StatusBadRequest, code: "BAD_REQUEST", family: factoryapi.ErrorFamilyBadRequest},
		{name: "not found", status: http.StatusNotFound, code: "NOT_FOUND", family: factoryapi.ErrorFamilyNotFound},
		{name: "conflict", status: http.StatusConflict, code: "REQUEST_CONFLICT", family: factoryapi.ErrorFamilyConflict},
		{name: "service unavailable", status: http.StatusServiceUnavailable, code: "MODEL_BACKEND_NOT_READY", family: factoryapi.ErrorFamilyInternalServerError},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var postCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					writeRemoteOMNICatalog(writer)
					return
				}
				if request.Method != http.MethodPost {
					http.NotFound(writer, request)
					return
				}
				postCalls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(testCase.status)
				_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
					Code: factoryapi.ErrorResponseCode(testCase.code), Family: testCase.family,
					Message: "controlled server failure",
				})
			}))
			t.Cleanup(server.Close)

			var output bytes.Buffer
			service := &httpService{
				http: testHTTPProtocol(t),
				inputFileReader: func(context.Context, string, int64) ([]byte, error) {
					return []byte("PNG"), nil
				},
			}
			err := service.Invoke(InvokeConfig{
				Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
				InputMappings: []string{"prompt=hello", "image=@fixture.png"}, Server: server.URL,
				Output: &output,
			})
			var apiErr *clihttp.APIError
			if err == nil || !errors.As(err, &apiErr) {
				t.Fatalf("server failure error = %v (%T), want typed API error", err, err)
			}
			if apiErr.StatusCode != testCase.status || apiErr.CLIErrorCode() != testCase.code || apiErr.CLIErrorFamily() != testCase.family {
				t.Fatalf("API error = %#v, want status/code/family %d/%s/%s", apiErr, testCase.status, testCase.code, testCase.family)
			}
			if output.Len() != 0 || postCalls.Load() != 1 {
				t.Fatalf("server failure output/calls = %q/%d, want empty/one", output.String(), postCalls.Load())
			}
		})
	}
}

func TestHTTPServiceInvokeTypedFailureEnvelopeDoesNotReportSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
			Failure: &factoryapi.ModelInvocationFailure{
				Class:   factoryapi.ModelInvocationFailureClassBackendProtocol,
				Message: "controlled backend failure",
			},
		})
	}))
	t.Cleanup(server.Close)

	var output bytes.Buffer
	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(context.Context, string, int64) ([]byte, error) {
			return []byte("PNG"), nil
		},
	}
	err := service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
		InputMappings: []string{"prompt=hello", "image=@fixture.png"}, Server: server.URL,
		Output: &output,
	})
	if err == nil || !strings.Contains(err.Error(), "controlled backend failure") {
		t.Fatalf("typed failure envelope error = %v, want controlled failure", err)
	}
	var failure *modelinference.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != modelinference.InvocationFailureClassBackendProtocol {
		t.Fatalf("typed failure envelope = %v, want backend protocol classification", err)
	}
	if output.Len() != 0 {
		t.Fatalf("typed failure envelope output = %q, want empty", output.String())
	}
}

func TestHTTPServiceInvokeRejectsMalformedOrIncompleteResponses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: "{"},
		{name: "missing outputs", body: `{}`},
		{name: "empty outputs", body: `{"outputs":[]}`},
		{name: "missing output identity", body: `{"outputs":[{"modality":"TEXT","content":"answer"}]}`},
		{name: "missing output carrier", body: `{"outputs":[{"name":"text","modality":"TEXT"}]}`},
		{name: "empty output carrier", body: `{"outputs":[{"name":"text","modality":"TEXT","content":""}]}`},
		{name: "empty artifact reference", body: `{"outputs":[{"name":"audio","modality":"AUDIO","artifact":{"artifactRef":""}}]}`},
		{name: "trailing JSON", body: `{"outputs":[{"name":"text","modality":"TEXT","content":"answer"}]} {}`},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var postCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					writeRemoteOMNICatalog(writer)
					return
				}
				if request.Method != http.MethodPost {
					http.NotFound(writer, request)
					return
				}
				postCalls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(testCase.body))
			}))
			t.Cleanup(server.Close)

			var output bytes.Buffer
			service := &httpService{
				http: testHTTPProtocol(t),
				inputFileReader: func(context.Context, string, int64) ([]byte, error) {
					return []byte("PNG"), nil
				},
			}
			err := service.Invoke(InvokeConfig{
				Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
				InputMappings: []string{"prompt=hello", "image=@fixture.png"}, Server: server.URL,
				JSON: true, Output: &output,
			})
			if err == nil || !strings.Contains(err.Error(), "malformed models response") {
				t.Fatalf("malformed response error = %v, want safe malformed response", err)
			}
			var coded interface {
				CLIErrorCode() string
				CLIErrorFamily() factoryapi.ErrorFamily
			}
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "MODEL_BACKEND_FAILURE" || coded.CLIErrorFamily() != factoryapi.ErrorFamilyInternalServerError {
				t.Fatalf("malformed response error = %v, want coded backend failure", err)
			}
			var failure *modelinference.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != modelinference.InvocationFailureClassMalformedResponse {
				t.Fatalf("malformed response error = %v, want malformed-response classification", err)
			}
			if output.Len() != 0 || postCalls.Load() != 1 {
				t.Fatalf("malformed response output/calls = %q/%d, want empty/one", output.String(), postCalls.Load())
			}
		})
	}
}

func TestHTTPServiceInvokeConcurrentNamedInputsIsolateBinaryBytes(t *testing.T) {
	t.Parallel()

	fixtures := map[string][]byte{
		"first.png":  {0x89, 'P', 'N', 'G', 0x00, 0xff, 0x01},
		"second.png": {0x89, 'P', 'N', 'G', 0x10, 0x80, 0x02},
	}
	var getCalls, postCalls atomic.Int32
	var receivedMu sync.Mutex
	receivedHashes := make(map[string]struct{}, len(fixtures))
	handlerErrors := make(chan error, len(fixtures))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			getCalls.Add(1)
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		postCalls.Add(1)
		var received factoryapi.GenericModelInvocationRequest
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			handlerErrors <- fmt.Errorf("decode concurrent request: %w", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		if received.Inputs == nil {
			handlerErrors <- errors.New("concurrent request has no inputs")
			http.Error(writer, "missing inputs", http.StatusBadRequest)
			return
		}
		var image []byte
		for _, input := range *received.Inputs {
			if input.Name == "image" && input.ContentBase64 != nil {
				image = append([]byte(nil), (*input.ContentBase64)...)
			}
		}
		if len(image) == 0 {
			handlerErrors <- errors.New("concurrent request has no binary image")
			http.Error(writer, "missing image", http.StatusBadRequest)
			return
		}
		hash := hashBytes(image)
		receivedMu.Lock()
		receivedHashes[hash] = struct{}{}
		receivedMu.Unlock()
		answer := "response-" + hash
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
			Outputs: []factoryapi.ModelInvocationOutput{{
				Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
			}},
		})
	}))
	t.Cleanup(server.Close)

	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(_ context.Context, path string, _ int64) ([]byte, error) {
			data, ok := fixtures[path]
			if !ok {
				return nil, fmt.Errorf("unknown fixture %q", path)
			}
			return append([]byte(nil), data...), nil
		},
	}
	start := make(chan struct{})
	results := make(chan struct {
		path   string
		output string
		err    error
	}, len(fixtures))
	var waitGroup sync.WaitGroup
	for path := range fixtures {
		path := path
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			var output bytes.Buffer
			err := service.Invoke(InvokeConfig{
				Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
				InputMappings: []string{"prompt=" + path, "image=@" + path},
				Server:        server.URL, Output: &output,
			})
			results <- struct {
				path   string
				output string
				err    error
			}{path: path, output: output.String(), err: err}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Errorf("concurrent %s error = %v", result.path, result.err)
			continue
		}
		want := "response-" + hashBytes(fixtures[result.path])
		if result.output != want {
			t.Errorf("concurrent %s output = %q, want %q", result.path, result.output, want)
		}
	}
	if handlerErrCount := len(handlerErrors); handlerErrCount != 0 {
		t.Fatalf("concurrent handler errors = %d, first = %v", handlerErrCount, <-handlerErrors)
	}
	if getCalls.Load() != int32(len(fixtures)) || postCalls.Load() != int32(len(fixtures)) {
		t.Fatalf("concurrent GET/POST calls = %d/%d, want %d/%d", getCalls.Load(), postCalls.Load(), len(fixtures), len(fixtures))
	}
	receivedMu.Lock()
	defer receivedMu.Unlock()
	if len(receivedHashes) != len(fixtures) {
		t.Fatalf("received image hashes = %#v, want one exact hash per fixture", receivedHashes)
	}
	for _, fixture := range fixtures {
		if _, ok := receivedHashes[hashBytes(fixture)]; !ok {
			t.Fatalf("received image hashes = %#v, missing %s", receivedHashes, hashBytes(fixture))
		}
	}
}

func TestHTTPServiceInvokeRecoversAfterTypedServerFailure(t *testing.T) {
	t.Parallel()

	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		call := postCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
				Code:    factoryapi.ErrorResponseCode("MODEL_BACKEND_NOT_READY"),
				Family:  factoryapi.ErrorFamilyInternalServerError,
				Message: "controlled first failure",
			})
			return
		}
		answer := "recovered response"
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
			Outputs: []factoryapi.ModelInvocationOutput{{
				Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
			}},
		})
	}))
	t.Cleanup(server.Close)
	service := &httpService{
		http: testHTTPProtocol(t),
		inputFileReader: func(context.Context, string, int64) ([]byte, error) {
			return []byte("PNG"), nil
		},
	}
	invoke := func(output *bytes.Buffer) error {
		return service.Invoke(InvokeConfig{
			Context: context.Background(), ModelName: "llm", Operation: modelinference.OperationOMNI,
			InputMappings: []string{"prompt=hello", "image=@fixture.png"}, Server: server.URL,
			Output: output,
		})
	}

	var firstOutput bytes.Buffer
	firstErr := invoke(&firstOutput)
	if firstErr == nil || !strings.Contains(firstErr.Error(), "controlled first failure") || firstOutput.Len() != 0 {
		t.Fatalf("first invocation error/output = %v/%q, want typed failure and no output", firstErr, firstOutput.String())
	}
	var secondOutput bytes.Buffer
	if err := invoke(&secondOutput); err != nil {
		t.Fatalf("recovery invocation error = %v", err)
	}
	if secondOutput.String() != "recovered response" {
		t.Fatalf("recovery invocation output = %q, want recovered response", secondOutput.String())
	}
	if postCalls.Load() != 2 {
		t.Fatalf("recovery POST calls = %d, want one request per invocation", postCalls.Load())
	}
}

func writeRemoteOMNICatalog(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
}

func remoteOMNIModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "llm",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationOMNI,
			Inputs: func() *[]factoryapi.ModelInvocationSlot {
				slots := []factoryapi.ModelInvocationSlot{
					{Name: "prompt", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText), Required: &required, MediaTypes: remotePointerSlice([]string{"text/plain"})},
					{Name: "image", Modality: remotePointer(factoryapi.ModelInvocationContentTypeImage), Required: &required, Repeatable: remotePointer(true), MediaTypes: remotePointerSlice([]string{"image/*"})},
					{Name: "parameters", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON), MediaTypes: remotePointerSlice([]string{"application/json"})},
				}
				return &slots
			}(),
			Outputs: func() *[]factoryapi.ModelInvocationSlot {
				slots := []factoryapi.ModelInvocationSlot{{Name: "text", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)}}
				return &slots
			}(),
		}},
	}
}

func remotePointer[T any](value T) *T { return &value }

func remotePointerSlice[T any](value []T) *[]T { return &value }

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
