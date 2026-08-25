package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// YOU_MODELS_BACKEND_ENDPOINT is an opt-in composition seam for delivered
// artifact and protocol fixtures. An unset value leaves the normal production
// graph unchanged; the command does not discover or claim a real backend from
// the environment.
const modelBackendEndpointEnvironment = "YOU_MODELS_BACKEND_ENDPOINT"

const (
	environmentEmbedBackendName     = "localai-backend-localai-llamacpp-functional.tar.gz"
	environmentEmbedBackendLocation = "https://github.com/portpowered/infinite-you/releases/download/lmx-v1-embeddings-end-to-end/localai-backend-localai-llamacpp-functional.tar.gz"
	environmentEmbedBackendBytes    = int64(25)
	environmentEmbedBackendSHA256   = "daed908ff6377212bb87323b0bddf0a891961abbc87cdbc1da4cf9fbe71bffc8"
)

type environmentModelBackend struct {
	client   *http.Client
	endpoint string
}

func modelBackendEdgesFromEnvironment() serviceedges.Edges {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv(modelBackendEndpointEnvironment)), "/")
	if endpoint == "" {
		return serviceedges.Edges{}
	}
	backend := environmentModelBackend{client: &http.Client{Timeout: 5 * time.Minute}, endpoint: endpoint}
	return serviceedges.Edges{
		ModelAssetHTTPClient: environmentModelAssetHTTPClient{
			client:   backend.client,
			endpoint: endpoint,
		},
		ModelAssetEndpoints: models.RuntimeAssetEndpoints{
			BaseURL: endpoint, APIBaseURL: endpoint,
		},
		ModelAssetHostPlatform:        models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelInvocationBackend:        backend.invoke,
		ModelASRBackend:               backend.asr,
		ModelEmbeddingBackend:         backend.embed,
		ModelResolveBackendArtifact:   environmentBackendArtifact,
		ModelHostProcessLauncher:      environmentHostLauncher{endpoint: endpoint},
		ModelHostProtocolNegotiator:   environmentHostProtocol{},
		ModelHostCompatibilityChecker: environmentHostCompatibility{},
	}
}

// environmentModelAssetHTTPClient routes the immutable release URL used by
// the pinned backend selector to the same opt-in fixture endpoint as model
// assets. The selector still receives the production-shaped GitHub identity,
// so this seam cannot alter the artifact manifest or publishing path.
type environmentModelAssetHTTPClient struct {
	client   *http.Client
	endpoint string
}

func (client environmentModelAssetHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, fmt.Errorf("model asset request is required")
	}
	endpoint, err := url.Parse(client.endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("model asset fixture endpoint is invalid")
	}
	clone := request.Clone(request.Context())
	clone.URL = cloneURL(request.URL)
	clone.URL.Scheme = endpoint.Scheme
	clone.URL.Host = endpoint.Host
	if endpoint.Path != "" && endpoint.Path != "/" {
		clone.URL.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(clone.URL.Path, "/")
	}
	return client.client.Do(clone)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	clone := *value
	return &clone
}

type environmentGenericRequest struct {
	ModelName string                    `json:"modelName"`
	Operation string                    `json:"operation"`
	Inputs    []environmentGenericInput `json:"inputs"`
}

type environmentGenericInput struct {
	Name          string `json:"name"`
	Modality      string `json:"modality"`
	ContentType   string `json:"contentType"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

type environmentGenericResponse struct {
	Outputs []environmentGenericOutput `json:"outputs"`
}

type environmentGenericOutput struct {
	Name          string                     `json:"name"`
	Modality      string                     `json:"modality"`
	ContentType   string                     `json:"contentType"`
	MediaType     string                     `json:"mediaType"`
	ContentBase64 string                     `json:"contentBase64"`
	Content       string                     `json:"content,omitempty"`
	Artifact      *environmentArtifactOutput `json:"artifact,omitempty"`
}

type environmentArtifactOutput struct {
	Ref        string            `json:"ref"`
	Name       string            `json:"name"`
	MediaType  string            `json:"mediaType"`
	SizeBytes  int64             `json:"sizeBytes"`
	Properties map[string]string `json:"properties,omitempty"`
}

type environmentEmbeddingRequest struct {
	Text       string         `json:"text"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type environmentEmbeddingResponse struct {
	Embeddings []float64 `json:"embeddings"`
}

func (backend environmentModelBackend) invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	inputs := request.Inputs
	if len(inputs) == 0 {
		inputs = []models.InferenceInput{request.Input}
	}
	wireInputs := make([]environmentGenericInput, 0, len(inputs))
	for _, input := range inputs {
		wireInputs = append(wireInputs, environmentGenericInput{
			Name: input.Name, Modality: string(input.Modality), ContentType: input.ContentType,
			MediaType: input.MediaType, ContentBase64: base64.StdEncoding.EncodeToString([]byte(input.Content)),
		})
	}
	var response environmentGenericResponse
	if err := backend.post(ctx, "/invoke", environmentGenericRequest{
		ModelName: request.Model.NameOrURI, Operation: request.Operation, Inputs: wireInputs,
	}, &response); err != nil {
		return nil, nil, err
	}
	content := make([]models.InferenceContent, 0, len(response.Outputs))
	artifacts := make([]models.InferenceArtifact, 0)
	for _, output := range response.Outputs {
		data, err := outputContent(output)
		if err != nil {
			return nil, nil, err
		}
		content = append(content, models.InferenceContent{
			Name: output.Name, Modality: models.Modality(output.Modality), ContentType: output.ContentType,
			MediaType: output.MediaType, Content: string(data),
		})
		artifact, err := environmentArtifact(output.Artifact)
		if err != nil {
			return nil, nil, err
		}
		if artifact != nil {
			artifacts = append(artifacts, *artifact)
		}
	}
	return content, artifacts, nil
}

func (backend environmentModelBackend) embed(
	ctx context.Context,
	request models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error) {
	var response environmentEmbeddingResponse
	if err := backend.post(ctx, "/embed", environmentEmbeddingRequest{
		Text: request.Text, Parameters: request.Parameters,
	}, &response); err != nil {
		return models.EmbeddingBackendResponse{}, err
	}
	return models.EmbeddingBackendResponse{
		Embeddings: append([]float64(nil), response.Embeddings...),
	}, nil
}

type environmentASRRequest struct {
	AudioBase64 string         `json:"audioBase64"`
	MediaType   string         `json:"mediaType"`
	Prompt      string         `json:"prompt,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type environmentASRResponse struct {
	Text      string                      `json:"text"`
	Segments  []environmentASRSegment     `json:"segments"`
	Artifacts []environmentArtifactOutput `json:"artifacts,omitempty"`
}

type environmentASRSegment struct {
	ID    int32  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

func (backend environmentModelBackend) asr(
	ctx context.Context,
	request models.ASRBackendRequest,
) (models.ASRBackendResponse, error) {
	var response environmentASRResponse
	if err := backend.post(ctx, "/asr", environmentASRRequest{
		AudioBase64: base64.StdEncoding.EncodeToString(request.Audio), MediaType: request.MediaType,
		Prompt: request.Prompt, Parameters: request.Parameters,
	}, &response); err != nil {
		return models.ASRBackendResponse{}, err
	}
	segments := make([]models.ASRBackendSegment, len(response.Segments))
	for index, segment := range response.Segments {
		segments[index] = models.ASRBackendSegment{ID: segment.ID, Start: segment.Start, End: segment.End, Text: segment.Text}
	}
	artifacts := make([]models.InferenceArtifact, 0, len(response.Artifacts))
	for _, output := range response.Artifacts {
		artifact, err := environmentArtifact(&output)
		if err != nil {
			return models.ASRBackendResponse{}, err
		}
		if artifact != nil {
			artifacts = append(artifacts, *artifact)
		}
	}
	return models.ASRBackendResponse{Text: response.Text, Segments: segments, Artifacts: artifacts}, nil
}

func (backend environmentModelBackend) post(ctx context.Context, path string, request, response any) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode model backend request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create model backend request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := backend.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("call model backend: %w", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 4096))
		return fmt.Errorf("model backend returned HTTP %d: %s", httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(httpResponse.Body).Decode(response); err != nil {
		return fmt.Errorf("decode model backend response: %w", err)
	}
	return nil
}

func outputContent(output environmentGenericOutput) ([]byte, error) {
	if output.ContentBase64 == "" {
		return []byte(output.Content), nil
	}
	content, err := base64.StdEncoding.DecodeString(output.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("decode model backend output %q: %w", output.Name, err)
	}
	return content, nil
}

func environmentArtifact(output *environmentArtifactOutput) (*models.InferenceArtifact, error) {
	if output == nil || strings.TrimSpace(output.Ref) == "" {
		return nil, nil
	}
	ref, err := (models.InferenceArtifactRef{}).Parse(output.Ref)
	if err != nil {
		return nil, fmt.Errorf("parse model backend artifact %q: %w", output.Ref, err)
	}
	return &models.InferenceArtifact{
		Name: output.Name, Artifact: ref, MediaType: output.MediaType,
		SizeBytes: output.SizeBytes, Properties: output.Properties,
	}, nil
}
func environmentBackendArtifact(
	_ context.Context,
	request serviceedges.ModelBackendArtifactSelectionRequest,
) (serviceedges.ModelBackendArtifactSelection, error) {
	selections := map[string]serviceedges.ModelBackendArtifactSelection{
		"localai-llamacpp": {
			Name: environmentEmbedBackendName, Location: environmentEmbedBackendLocation,
			Bytes: environmentEmbedBackendBytes, SHA256: environmentEmbedBackendSHA256,
		},
		"localai-whisper": {
			Name:     "localai-backend-localai-whisper-linux-amd64-fixture.tar.gz",
			Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-fixture/localai-backend-localai-whisper-linux-amd64-fixture.tar.gz",
			Bytes:    26, SHA256: "d1481b62fccf94404c3ca599efa30c432d87bdad4bc7493c7e8f82ff84e0e61b",
		},
		"localai-vibevoice": {
			Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
			Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
			Bytes:    22, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
		},
	}
	selection, ok := selections[request.Backend]
	if !ok {
		return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("model backend fixture does not support %q", request.Backend)
	}
	return selection, nil
}

type environmentHostLauncher struct{ endpoint string }

func (launcher environmentHostLauncher) Start(
	_ context.Context,
	_ serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	return &environmentHostProcess{endpoint: launcher.endpoint, done: make(chan error, 1)}, nil
}

type environmentHostProcess struct {
	endpoint string
	done     chan error
	once     sync.Once
}

func (process *environmentHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *environmentHostProcess) Wait() error            { return <-process.done }
func (process *environmentHostProcess) Stop(context.Context) error {
	process.once.Do(func() { process.done <- nil })
	return nil
}

type environmentHostProtocol struct{}

func (environmentHostProtocol) Negotiate(
	_ context.Context,
	_ string,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: "localai-backend-v1", Backend: request.Backend, Ready: true,
	}, nil
}

type environmentHostCompatibility struct{}

func (environmentHostCompatibility) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}
