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
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
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
