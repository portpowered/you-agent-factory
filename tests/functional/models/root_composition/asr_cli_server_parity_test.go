package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

const localAIASRRedactionSentinel = "ASR_PARITY_SECRET_QUERY_7c9f2a"

type localAIASRParityCase struct {
	name          string
	mode          localai.Mode
	expectedClass models.InvocationFailureClass
}

type localAIASRNormalizedOutput struct {
	Name            string
	Modality        string
	ContentType     string
	MediaType       string
	Content         string
	ArtifactRef     string
	ArtifactMedia   string
	ArtifactSize    int64
	ArtifactPresent bool
	Properties      map[string]string
}

type localAIASRPublicObservation struct {
	Outputs []localAIASRNormalizedOutput
}

type localAIASRCLIResult struct {
	Observation localAIASRPublicObservation
	Response    factoryapi.GenericModelInvocationResponse
	Stdout      string
	Stderr      string
	Err         error
}

type localAIASRHTTPResult struct {
	Observation localAIASRPublicObservation
	Failure     *localAIOMNIFailureDiagnostic
	Status      int
	Err         error
}

type localAIASRPathCheckpoint struct {
	backendRequests int
	backendFailures int
	fixtureCalls    int
}

type localAIASRPathTrace struct {
	label           string
	backendRequests []models.InvokeModelRequest
	backendFailures []error
	fixtureASRCalls []localai.Call
}

type localAIASRBackendRecorder struct {
	mu       sync.Mutex
	requests []models.InvokeModelRequest
	failures []error
	attempts *atomic.Int32
	next     serviceedges.ModelInvocationBackend
}

// TestModelsASRControlledCLIHTTPAndExplicitServerParity proves the complete
// public ASR matrix through one reusable root-built process per path group.
// Each case owns its fixture, cache home, Factory directory, managed host,
// and request trace, so the parallel cases cannot share mutable runtime state.
func TestModelsASRControlledCLIHTTPAndExplicitServerParity(t *testing.T) {
	t.Parallel()

	cases := localAIASRParityCases(t)
	var totalAttempts atomic.Int32
	t.Cleanup(func() {
		if got := totalAttempts.Load(); got > int32(len(cases)*3) {
			t.Errorf("LocalAI ASR backend attempts = %d, want at most %d", got, len(cases)*3)
		}
	})
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runLocalAIASRParityCase(t, testCase, &totalAttempts)
		})
	}
}

// TestModelsASRIncompleteOutputSelectionIsTypedAndEffectFree proves the
// customer-facing root composition rejects both an unselected and a partial
// ASR output set before reading inputs, resolving assets, starting a host, or
// invoking the backend. The two cases share the production catalog shape but
// each owns its complete Process.Execute fixture and effect observers.
func TestModelsASRIncompleteOutputSelectionIsTypedAndEffectFree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		outputSlot string
		wantClass  models.InvocationFailureClass
	}{
		{name: "no output selection", outputSlot: "", wantClass: models.InvocationFailureClassInvalidParameter},
		{name: "partial output selection", outputSlot: "transcript", wantClass: models.InvocationFailureClassInvalidSlot},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newInvalidGenericCLIProcess(t, genericConformanceFactoryConfig)
			defer fixture.close(t)

			args := []string{
				"you", "models", "invoke", models.BuiltInModelNameASR,
				"--operation", models.OperationASR,
				"--input", `{"name":"audio","modality":"AUDIO","contentType":"audio/wav","mediaType":"audio/wav","content":"RIFF-ASR-FIXTURE"}`,
			}
			if testCase.outputSlot != "" {
				args = append(args, "--output-map", testCase.outputSlot+"="+filepath.Join(t.TempDir(), testCase.outputSlot+".out"))
			}
			inputs := support.FakeInputs(t.Context(), args)
			inputs.Input.Env = fixture.environment
			inputs.Input.WorkingDirectory = fixture.directory
			err := fixture.process.Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute returned nil, want incomplete-output BAD_REQUEST")
			}
			if inputs.Stdout() != "" {
				t.Fatalf("incomplete ASR output stdout = %q, want empty", inputs.Stdout())
			}
			var typed *models.InvocationFailure
			if !errors.As(err, &typed) || typed == nil || typed.Class != testCase.wantClass || typed.Operation != models.OperationASR {
				t.Fatalf("incomplete ASR output error = %#v, want %s ASR failure", err, testCase.wantClass)
			}
			if !strings.Contains(typed.Message, "transcript") || !strings.Contains(typed.Message, "segments") {
				t.Fatalf("incomplete ASR output message = %q, want every required output name", typed.Message)
			}
			var diagnostic factoryapi.ErrorResponse
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &diagnostic); decodeErr != nil {
				t.Fatalf("decode incomplete ASR diagnostic: %v; stderr=%q; error=%v", decodeErr, inputs.Stderr(), err)
			}
			if diagnostic.Code != factoryapi.ErrorResponseCode("BAD_REQUEST") || diagnostic.Family != factoryapi.ErrorFamilyBadRequest || diagnostic.Message != typed.Message {
				t.Fatalf("incomplete ASR diagnostic = %#v, want BAD_REQUEST with typed message", diagnostic)
			}
			fixture.assertNoEffects(t)
		})
	}
}

func localAIASRParityCases(t *testing.T) []localAIASRParityCase {
	t.Helper()
	return []localAIASRParityCase{
		{name: "healthy", mode: localai.ModeNormal},
		// The second healthy case makes the isolation assertion observable while
		// the full CLI/HTTP/server matrix is running concurrently.
		{name: "healthy-isolated", mode: localai.ModeNormal},
		{
			name:          "backend-unavailable",
			mode:          localai.ModeBackendUnavailable,
			expectedClass: models.InvocationFailureClassBackendReadiness,
		},
		{
			name:          "protocol-mismatch",
			mode:          localai.ModeProtocolMismatch,
			expectedClass: models.InvocationFailureClassBackendProtocol,
		},
		{
			name:          "malformed-response",
			mode:          localai.ModeMalformedResponse,
			expectedClass: models.InvocationFailureClassMalformedResponse,
		},
	}
}

func runLocalAIASRParityCase(
	t *testing.T,
	testCase localAIASRParityCase,
	totalAttempts *atomic.Int32,
) {
	t.Helper()
	home := functionalTempDir(t)
	environment := localAIOMNIEnvironment(home)
	fixture := functionalStartLocalAI(t, localai.Options{Mode: testCase.mode})
	writeGenericConformanceCaches(t, home)
	dir := functionalScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))
	row := localAIASRConformanceRow(t, testCase.name)

	var recorder localAIASRBackendRecorder
	recorder.attempts = totalAttempts
	localEdges, localNetwork, localLauncher := localAIASRParityEdges(home, fixture, &recorder)
	localProcess := functionalBuildProcess(t, localEdges)
	localCheckpoint := localAIASRPathCheckpointAt(&recorder, fixture)
	localResult := executeLocalAIASRParityCLI(t, localProcess, dir, environment, row, "")
	localTrace := localAIASRPathTraceAt(t, "direct local CLI", &recorder, fixture, localCheckpoint)
	closeRootProcess(t, localProcess, "close local ASR process")
	assertLocalAIOMNIHostReleased(t, localLauncher, "local ASR")

	hostedEdges, hostedNetwork, hostedLauncher := localAIASRParityEdges(home, fixture, &recorder)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       environment,
		Edges:                     hostedEdges,
	})

	httpCheckpoint := localAIASRPathCheckpointAt(&recorder, fixture)
	httpResult := executeLocalAIASRHTTP(t, server.URL()+"/models/invocations", row)
	httpTrace := localAIASRPathTraceAt(t, "direct HTTP", &recorder, fixture, httpCheckpoint)
	serverCheckpoint := localAIASRPathCheckpointAt(&recorder, fixture)
	serverResult := executeLocalAIASRParityCLI(
		t, localAIOMNIExecutor{fn: func(input root.Input) error {
			return server.Execute(t, input)
		}}, dir, environment, row, server.URL(),
	)
	serverTrace := localAIASRPathTraceAt(t, "configured --server CLI", &recorder, fixture, serverCheckpoint)

	paths := []localAIASRPathTrace{localTrace, httpTrace, serverTrace}
	if testCase.expectedClass == "" {
		assertLocalAIASRSuccessParity(t, row, localResult, httpResult, serverResult)
		assertLocalAIASRBackendInputs(t, row, paths)
	} else {
		assertLocalAIASRFailureParity(t, testCase, row, localResult, httpResult, serverResult, fixture, home, dir, server.URL(), paths)
	}
	assertLocalAIASRFixtureCalls(t, row, testCase.expectedClass, paths)

	server.Close(t)
	assertLocalAIOMNIServerReleased(t, server, hostedLauncher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close LocalAI ASR fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if calls := localNetwork.Calls() + hostedNetwork.Calls(); calls != 0 {
		t.Fatalf("LocalAI ASR model-asset network calls = %d, want zero", calls)
	}
}

func localAIASRParityEdges(
	home string,
	fixture *localai.Fixture,
	recorder *localAIASRBackendRecorder,
) (serviceedges.Edges, *rejectingModelAssetHTTP, *recordingModelHostLauncher) {
	edges, network, _, launcher := localAIConformanceEdges(home, fixture)
	recorder.next = edges.ModelInvocationBackend
	edges.ModelInvocationBackend = recorder.Invoke
	return edges, network, launcher
}

func (recorder *localAIASRBackendRecorder) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	if recorder.attempts != nil {
		recorder.attempts.Add(1)
	}
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, localAIASRCloneRequest(request))
	next := recorder.next
	recorder.mu.Unlock()
	if next == nil {
		err := errors.New("LocalAI ASR fixture backend is not configured")
		recorder.recordFailure(err)
		return nil, nil, err
	}
	outputs, artifacts, err := next(ctx, request)
	if err != nil {
		recorder.recordFailure(err)
		return nil, nil, err
	}
	for _, output := range outputs {
		if output.Name != "segments" {
			continue
		}
		artifact, parseErr := (models.InferenceArtifactRef{}).Parse("artifact:segments")
		if parseErr != nil {
			recorder.recordFailure(parseErr)
			return nil, nil, parseErr
		}
		artifacts = append(artifacts, models.InferenceArtifact{
			Name: "segments", Artifact: artifact, MediaType: "application/json",
			SizeBytes: int64(len(output.Content)),
			Properties: map[string]string{
				"digest": "sha256:fixture-segments",
			},
		})
	}
	return outputs, artifacts, nil
}

func (recorder *localAIASRBackendRecorder) recordFailure(err error) {
	if err == nil {
		return
	}
	recorder.mu.Lock()
	recorder.failures = append(recorder.failures, err)
	recorder.mu.Unlock()
}

func (recorder *localAIASRBackendRecorder) Requests() []models.InvokeModelRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	result := make([]models.InvokeModelRequest, len(recorder.requests))
	for index, request := range recorder.requests {
		result[index] = localAIASRCloneRequest(request)
	}
	return result
}

func (recorder *localAIASRBackendRecorder) Failures() []error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]error(nil), recorder.failures...)
}

func localAIASRCloneRequest(request models.InvokeModelRequest) models.InvokeModelRequest {
	request.Input = request.Input.Clone()
	inputs := request.Inputs
	request.Inputs = make([]models.InferenceInput, len(inputs))
	for index, input := range inputs {
		request.Inputs[index] = input.Clone()
	}
	parameters := request.Parameters
	request.Parameters = make([]models.OperationParameter, len(parameters))
	for index, parameter := range parameters {
		request.Parameters[index] = parameter.Clone()
	}
	return request
}

func localAIASRPathCheckpointAt(
	recorder *localAIASRBackendRecorder,
	fixture *localai.Fixture,
) localAIASRPathCheckpoint {
	return localAIASRPathCheckpoint{
		backendRequests: len(recorder.Requests()),
		backendFailures: len(recorder.Failures()),
		fixtureCalls:    len(fixture.Calls()),
	}
}

func localAIASRPathTraceAt(
	t testing.TB,
	label string,
	recorder *localAIASRBackendRecorder,
	fixture *localai.Fixture,
	checkpoint localAIASRPathCheckpoint,
) localAIASRPathTrace {
	t.Helper()
	requests := recorder.Requests()
	failures := recorder.Failures()
	calls := fixture.Calls()
	if checkpoint.backendRequests > len(requests) || checkpoint.backendFailures > len(failures) || checkpoint.fixtureCalls > len(calls) {
		t.Fatalf("%s ASR path trace moved backwards: requests %d/%d failures %d/%d calls %d/%d", label, checkpoint.backendRequests, len(requests), checkpoint.backendFailures, len(failures), checkpoint.fixtureCalls, len(calls))
	}
	asrCalls := make([]localai.Call, 0)
	for _, call := range calls[checkpoint.fixtureCalls:] {
		if call.Method == "AudioTranscription" {
			asrCalls = append(asrCalls, call)
		}
	}
	return localAIASRPathTrace{
		label:           label,
		backendRequests: requests[checkpoint.backendRequests:],
		backendFailures: failures[checkpoint.backendFailures:],
		fixtureASRCalls: asrCalls,
	}
}

func executeLocalAIASRParityCLI(
	t testing.TB,
	process localAIOMNIProcess,
	dir string,
	environment []string,
	row conformance.Row,
	serverURL string,
) localAIASRCLIResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), localAIASRCLIArgs(row, serverURL))
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = dir
	err := process.Execute(inputs.Input)
	result := localAIASRCLIResult{Stdout: inputs.Stdout(), Stderr: inputs.Stderr(), Err: err}
	if err != nil {
		return result
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &result.Response); err != nil {
		result.Err = fmt.Errorf("%s ASR CLI response was not JSON", row.Label)
		return result
	}
	if err := assertConformanceResponse(row, result.Response); err != nil {
		result.Err = fmt.Errorf("%s ASR CLI semantic response failed: %w", row.Label, err)
		return result
	}
	result.Observation, result.Err = localAIASRObservationFromResponse(result.Response)
	return result
}

func localAIASRCLIArgs(row conformance.Row, serverURL string) []string {
	args := []string{"you", "--json"}
	if strings.TrimSpace(serverURL) != "" {
		args = append(args, "--server", strings.TrimSuffix(serverURL, "/"))
	}
	args = append(args, "models", "invoke", conformanceModelName(row.Operation.Name), "--operation", row.Operation.Name)
	for _, input := range row.Inputs {
		args = append(args, "--input", fmt.Sprintf(`{"name":%q,"modality":%q,"contentType":%q,"mediaType":%q,"content":%q}`,
			input.Name, input.Modality, input.ContentType, input.MediaType, input.Content))
	}
	return args
}

func executeLocalAIASRHTTP(t testing.TB, endpoint string, row conformance.Row) localAIASRHTTPResult {
	t.Helper()
	response, failure, status, err := postConformanceInvocation(t.Context(), endpoint, row)
	result := localAIASRHTTPResult{Status: status, Err: err}
	if err != nil {
		return result
	}
	if status < 200 || status >= 300 {
		result.Failure = &localAIOMNIFailureDiagnostic{
			Code: string(failure.Code), Family: failure.Family, Message: failure.Message,
		}
		return result
	}
	result.Observation, result.Err = localAIASRObservationFromResponse(response)
	return result
}

func localAIASRObservationFromResponse(
	response factoryapi.GenericModelInvocationResponse,
) (localAIASRPublicObservation, error) {
	if response.Failure != nil || len(response.Outputs) != 2 {
		return localAIASRPublicObservation{}, errors.New("unexpected ASR response shape")
	}
	observation := localAIASRPublicObservation{Outputs: make([]localAIASRNormalizedOutput, 0, len(response.Outputs))}
	for _, output := range response.Outputs {
		if output.Content == nil || output.ContentType == nil || output.MediaType == nil {
			return localAIASRPublicObservation{}, fmt.Errorf("ASR output %q omitted content metadata", output.Name)
		}
		normalized := localAIASRNormalizedOutput{
			Name: output.Name, Modality: string(output.Modality), ContentType: *output.ContentType,
			MediaType: *output.MediaType, Content: *output.Content,
		}
		if output.Artifact != nil {
			normalized.ArtifactPresent = true
			normalized.ArtifactRef = output.Artifact.ArtifactRef
			if output.Artifact.MediaType != nil {
				normalized.ArtifactMedia = *output.Artifact.MediaType
			}
			if output.Artifact.SizeBytes != nil {
				normalized.ArtifactSize = *output.Artifact.SizeBytes
			}
			if output.Artifact.Properties != nil {
				normalized.Properties = make(map[string]string, len(*output.Artifact.Properties))
				for key, value := range *output.Artifact.Properties {
					normalized.Properties[key] = value
				}
			}
		}
		observation.Outputs = append(observation.Outputs, normalized)
	}
	return observation, nil
}

func assertLocalAIASRSuccessParity(
	t *testing.T,
	row conformance.Row,
	localResult localAIASRCLIResult,
	httpResult localAIASRHTTPResult,
	serverResult localAIASRCLIResult,
) {
	t.Helper()
	if localResult.Err != nil || httpResult.Err != nil || serverResult.Err != nil || localResult.Stdout == "" || serverResult.Stdout == "" {
		t.Fatalf("%s did not complete successfully on all ASR paths: local=%v http=%v server=%v", row.Label, localResult.Err, httpResult.Err, serverResult.Err)
	}
	if localResult.Observation.Outputs == nil || !reflect.DeepEqual(localResult.Observation, httpResult.Observation) || !reflect.DeepEqual(localResult.Observation, serverResult.Observation) {
		t.Fatalf("%s ASR public observations differ across direct CLI, HTTP, and --server CLI: local=%#v http=%#v server=%#v", row.Label, localResult.Observation, httpResult.Observation, serverResult.Observation)
	}
	if err := assertConformanceResponse(row, localResult.Response); err != nil {
		t.Fatalf("%s ASR semantic response failed: %v", row.Label, err)
	}
	if len(localResult.Observation.Outputs) != 2 || localResult.Observation.Outputs[0].Name != "transcript" || localResult.Observation.Outputs[1].Name != "segments" {
		t.Fatalf("%s ASR output order = %#v, want transcript then segments", row.Label, localResult.Observation.Outputs)
	}
	transcript, segments := localResult.Observation.Outputs[0], localResult.Observation.Outputs[1]
	if transcript.Modality != string(factoryapi.ModelInvocationContentTypeText) || transcript.ContentType != "text/plain" || transcript.MediaType != "text/plain" || transcript.ArtifactPresent {
		t.Fatalf("%s transcript metadata = %#v, want text/plain inline output", row.Label, transcript)
	}
	if segments.Modality != string(factoryapi.ModelInvocationContentTypeJSON) || segments.ContentType != "application/json" || segments.MediaType != "application/json" || !segments.ArtifactPresent || segments.ArtifactRef != "artifact:segments" || segments.ArtifactMedia != "application/json" || segments.ArtifactSize != int64(len(segments.Content)) || segments.Properties["digest"] != "sha256:fixture-segments" {
		t.Fatalf("%s segments metadata = %#v, want truthful opaque artifact metadata", row.Label, segments)
	}
}

func assertLocalAIASRBackendInputs(t *testing.T, row conformance.Row, paths []localAIASRPathTrace) {
	t.Helper()
	for _, path := range paths {
		if len(path.backendRequests) != 1 {
			t.Fatalf("%s %s backend request count = %d, want exactly one", row.Label, path.label, len(path.backendRequests))
		}
		request := path.backendRequests[0]
		if request.Operation != models.OperationASR || len(request.Inputs) != len(row.Inputs) {
			t.Fatalf("%s %s ASR backend request = %#v, want operation/input count preserved", row.Label, path.label, request)
		}
		for index, input := range request.Inputs {
			want := row.Inputs[index]
			if input.Name != want.Name || input.Modality != want.Modality || input.ContentType != want.ContentType || input.MediaType != want.MediaType || input.Content != want.Content {
				t.Fatalf("%s %s ASR input %d = %#v, request.Input=%#v, want %#v", row.Label, path.label, index, input, request.Input, want)
			}
		}
		if len(path.backendFailures) != 0 {
			t.Fatalf("%s %s successful ASR path had backend failures = %#v", row.Label, path.label, path.backendFailures)
		}
	}
}

func assertLocalAIASRFailureParity(
	t *testing.T,
	testCase localAIASRParityCase,
	row conformance.Row,
	localResult localAIASRCLIResult,
	httpResult localAIASRHTTPResult,
	serverResult localAIASRCLIResult,
	fixture *localai.Fixture,
	home string,
	dir string,
	serverURL string,
	paths []localAIASRPathTrace,
) {
	t.Helper()
	if localResult.Err == nil || httpResult.Err != nil || httpResult.Failure == nil || serverResult.Err == nil {
		t.Fatalf("%s ASR failure did not reach all public paths: local=%v httpErr=%v httpFailure=%#v server=%v", row.Label, localResult.Err, httpResult.Err, httpResult.Failure, serverResult.Err)
	}
	if localResult.Stdout != "" || serverResult.Stdout != "" || httpResult.Observation.Outputs != nil {
		t.Fatalf("%s ASR failure published partial output: local=%q http=%#v server=%q", row.Label, localResult.Stdout, httpResult.Observation, serverResult.Stdout)
	}
	localDiagnostic, ok := localAIOMNIErrorDiagnosticFromError(localResult.Err)
	if !ok {
		t.Fatalf("%s local ASR failure was not a coded diagnostic: %T %v", row.Label, localResult.Err, localResult.Err)
	}
	serverDiagnostic, ok := localAIOMNIErrorDiagnosticFromError(serverResult.Err)
	if !ok {
		t.Fatalf("%s --server ASR failure was not a coded diagnostic: %T %v", row.Label, serverResult.Err, serverResult.Err)
	}
	for _, diagnostic := range []*localAIOMNIFailureDiagnostic{localDiagnostic, httpResult.Failure, serverDiagnostic} {
		if diagnostic.Code != "MODEL_BACKEND_NOT_READY" && diagnostic.Code != "MODEL_BACKEND_FAILURE" {
			t.Fatalf("%s ASR diagnostic = %#v, want backend readiness/protocol category", row.Label, diagnostic)
		}
	}
	wantCode := "MODEL_BACKEND_FAILURE"
	if testCase.expectedClass == models.InvocationFailureClassBackendReadiness {
		wantCode = "MODEL_BACKEND_NOT_READY"
	}
	if localDiagnostic.Code != wantCode || httpResult.Failure.Code != wantCode || serverDiagnostic.Code != wantCode ||
		localDiagnostic.Family != factoryapi.ErrorFamilyInternalServerError || httpResult.Failure.Family != factoryapi.ErrorFamilyInternalServerError || serverDiagnostic.Family != localDiagnostic.Family {
		t.Fatalf("%s ASR diagnostic parity = local=%#v http=%#v server=%#v, want code %s and stable family", row.Label, localDiagnostic, httpResult.Failure, serverDiagnostic, wantCode)
	}
	if localDiagnostic.Message != httpResult.Failure.Message || localDiagnostic.Message != serverDiagnostic.Message || strings.TrimSpace(localDiagnostic.Message) == "" {
		t.Fatalf("%s ASR safe failure messages differ or are empty: local=%q http=%q server=%q", row.Label, localDiagnostic.Message, httpResult.Failure.Message, serverDiagnostic.Message)
	}
	var typed *models.InvocationFailure
	if !errors.As(localResult.Err, &typed) || typed == nil || typed.Class != testCase.expectedClass || typed.Operation != models.OperationASR {
		t.Fatalf("%s local ASR typed failure = %#v, want class %s and operation ASR", row.Label, typed, testCase.expectedClass)
	}
	redacted := []string{localAIASRRedactionSentinel, fixture.Endpoint(), home, dir, serverURL}
	for _, value := range redacted {
		if strings.Contains(localDiagnostic.Message, value) || strings.Contains(httpResult.Failure.Message, value) || strings.Contains(serverDiagnostic.Message, value) || strings.Contains(localResult.Stderr, value) || strings.Contains(serverResult.Stderr, value) || strings.Contains(localResult.Err.Error(), value) || strings.Contains(serverResult.Err.Error(), value) {
			t.Fatalf("%s ASR diagnostics leaked redacted value %q", row.Label, value)
		}
	}
	for _, path := range paths {
		if len(path.backendRequests) > 1 || len(path.backendFailures) > 1 || len(path.fixtureASRCalls) > 1 {
			t.Fatalf("%s %s retried a failed ASR operation: requests=%d failures=%d fixtureCalls=%d", row.Label, path.label, len(path.backendRequests), len(path.backendFailures), len(path.fixtureASRCalls))
		}
	}
}

func assertLocalAIASRFixtureCalls(
	t *testing.T,
	row conformance.Row,
	expectedClass models.InvocationFailureClass,
	paths []localAIASRPathTrace,
) {
	t.Helper()
	for _, path := range paths {
		if expectedClass == models.InvocationFailureClassBackendReadiness {
			continue
		}
		if len(path.fixtureASRCalls) != 1 {
			t.Fatalf("%s %s controlled AudioTranscription calls = %d, want exactly one", row.Label, path.label, len(path.fixtureASRCalls))
		}
		if path.fixtureASRCalls[0].Prompt != row.Inputs[0].Content {
			t.Fatalf("%s %s ASR fixture prompt = %q, want %q", row.Label, path.label, path.fixtureASRCalls[0].Prompt, row.Inputs[0].Content)
		}
	}
}

func localAIASRConformanceRow(t *testing.T, label string) conformance.Row {
	t.Helper()
	for _, row := range conformance.Build(models.GenericOperationCatalog{}).Rows {
		if row.Operation.Name != models.OperationASR || row.Variant != conformance.VariantAudio {
			continue
		}
		row = row.Clone()
		row.Label = models.OperationASR + "/" + label
		row.Inputs[0].Content = "ASR_AUDIO_" + strings.ToUpper(strings.ReplaceAll(label, "-", "_"))
		if label == "malformed-response" {
			row.Inputs[0].Content += "_" + localAIASRRedactionSentinel
		}
		return row
	}
	t.Fatal("conformance matrix has no ASR/audio row")
	return conformance.Row{}
}

// TestModelsASRControlledStartupCrashDoesNotPublish proves a managed-host
// process-start failure remains a typed, redacted public failure at the CLI
// boundary and does not reach the ASR backend.
func TestModelsASRControlledStartupCrashDoesNotPublish(t *testing.T) {
	t.Parallel()
	const secret = "HF_TOKEN=fixture-secret endpoint=https://private.invalid:7437/cache"
	crashed := &crashedGenericCLIHostLauncher{waitErr: errors.New(secret)}
	process, dir, environment, fixture, network, _ := buildLocalAIASRProcess(t, localai.Options{}, func(edges *serviceedges.Edges) {
		edges.ModelHostProcessLauncher = crashed
	})
	row := localAIASRConformanceRow(t, "startup-crash")
	result := executeLocalAIASRParityCLI(t, process, dir, environment, row, "")
	if result.Err == nil || (!errors.Is(result.Err, models.ErrHostProcessCrash) && !errors.Is(result.Err, models.ErrHostRuntimeNotReady)) {
		t.Fatalf("ASR startup crash error = %v, want provider-neutral host failure", result.Err)
	}
	if crashed.StartCalls() == 0 || result.Stdout != "" {
		t.Fatalf("ASR startup crash effects = starts %d stdout %q, want one start and no output", crashed.StartCalls(), result.Stdout)
	}
	if strings.Contains(result.Err.Error(), secret) || strings.Contains(result.Stderr, secret) || strings.Contains(result.Err.Error(), "7437") {
		t.Fatalf("ASR startup crash leaked private process details: err=%v stderr=%q", result.Err, result.Stderr)
	}
	closeRootProcess(t, process, "close ASR startup-crash process")
	if err := fixture.Close(); err != nil {
		t.Fatalf("close ASR startup-crash fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if network.Calls() != 0 {
		t.Fatalf("ASR startup-crash model-asset network calls = %d, want zero", network.Calls())
	}
}

// TestModelsASRControlledHealthTimeoutStopsReadiness proves a cancelled ASR
// readiness negotiation stops the managed host and publishes no result.
func TestModelsASRControlledHealthTimeoutStopsReadiness(t *testing.T) {
	t.Parallel()
	protocol := &blockingGenericCLIProtocol{}
	protocol.init()
	process, dir, environment, fixture, network, launcher := buildLocalAIASRProcess(t, localai.Options{}, func(edges *serviceedges.Edges) {
		edges.ModelHostProtocolNegotiator = protocol
	})
	row := localAIASRConformanceRow(t, "health-timeout")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, localAIASRCLIArgs(row, ""))
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = dir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	waitForGenericCLIEventOrResult(t, protocol.started, done, "ASR health negotiation start")
	err := waitForGenericCLIResult(t, done, "ASR health timeout")
	if err == nil || (!errors.Is(err, models.ErrInferenceCancelled) && !errors.Is(err, context.DeadlineExceeded)) {
		t.Fatalf("ASR health-timeout error = %v, want cancellation-class timeout", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("ASR health-timeout stdout = %q, want no output", inputs.Stdout())
	}
	closeRootProcess(t, process, "close ASR health-timeout process")
	assertLocalAIOMNIHostReleased(t, launcher, "ASR health-timeout")
	if err := fixture.Close(); err != nil {
		t.Fatalf("close ASR health-timeout fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if network.Calls() != 0 {
		t.Fatalf("ASR health-timeout model-asset network calls = %d, want zero", network.Calls())
	}
}

// TestModelsASRControlledCancellationStopsBackend proves cancellation after
// the public ASR backend starts releases the host path without partial output.
func TestModelsASRControlledCancellationStopsBackend(t *testing.T) {
	t.Parallel()
	backend := &localAIASRBlockingBackend{started: make(chan struct{})}
	process, dir, environment, fixture, network, launcher := buildLocalAIASRProcess(t, localai.Options{}, func(edges *serviceedges.Edges) {
		edges.ModelInvocationBackend = backend.Invoke
	})
	row := localAIASRConformanceRow(t, "cancellation")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, localAIASRCLIArgs(row, ""))
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = dir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	waitForGenericCLIEventOrResult(t, backend.started, done, "ASR backend start")
	cancel()
	err := waitForGenericCLIResult(t, done, "ASR cancellation")
	if err == nil || (!errors.Is(err, models.ErrInferenceCancelled) && !errors.Is(err, context.Canceled)) {
		t.Fatalf("ASR cancellation error = %v, want cancellation-class failure", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("ASR cancellation stdout = %q, want no output", inputs.Stdout())
	}
	closeRootProcess(t, process, "close ASR cancellation process")
	assertLocalAIOMNIHostReleased(t, launcher, "ASR cancellation")
	if err := fixture.Close(); err != nil {
		t.Fatalf("close ASR cancellation fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if network.Calls() != 0 {
		t.Fatalf("ASR cancellation model-asset network calls = %d, want zero", network.Calls())
	}
}

// TestModelsASRControlledMappedPublicationRollsBack proves the two named ASR
// outputs publish atomically through the public CLI output edges when the
// second destination fails.
func TestModelsASRControlledMappedPublicationRollsBack(t *testing.T) {
	t.Parallel()
	outputDirectory := functionalTempDir(t)
	transcriptPath := filepath.Join(outputDirectory, "transcript.out")
	segmentsPath := filepath.Join(outputDirectory, "segments.out")
	if err := os.WriteFile(transcriptPath, []byte("old transcript"), 0o644); err != nil {
		t.Fatalf("seed ASR transcript output: %v", err)
	}
	if err := os.WriteFile(segmentsPath, []byte("old segments"), 0o644); err != nil {
		t.Fatalf("seed ASR segments output: %v", err)
	}
	effects := &genericCLIOutputFailureEffects{failedTarget: segmentsPath}
	process, dir, environment, fixture, network, launcher := buildLocalAIASRProcess(t, localai.Options{}, func(edges *serviceedges.Edges) {
		edges.ModelCLIOutputCreateTempFile = effects.CreateTemp
		edges.ModelCLIOutputInspectPath = effects.Inspect
		edges.ModelCLIOutputRemovePath = effects.Remove
		edges.ModelCLIOutputRenamePath = effects.Rename
	})
	row := localAIASRConformanceRow(t, "mapped-rollback")
	args := localAIASRCLIArgs(row, "")
	args = append([]string{args[0]}, args[2:]...)
	args = append(args, "--output-map", "transcript="+transcriptPath, "--output-map", "segments="+segmentsPath)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = dir
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "injected mapped publication failure") {
		t.Fatalf("ASR mapped publication error = %v, want injected second-publication failure", err)
	}
	if !effects.failed.Load() {
		t.Fatal("ASR mapped publication failure edge was not exercised")
	}
	assertFunctionalFile(t, transcriptPath, "old transcript")
	assertFunctionalFile(t, segmentsPath, "old segments")
	entries, readErr := os.ReadDir(outputDirectory)
	if readErr != nil {
		t.Fatalf("read ASR mapped output directory: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".you-model-output-") || strings.HasPrefix(entry.Name(), ".you-model-output-backup-") {
			t.Fatalf("partial ASR mapped output artifact %q remains after rollback", entry.Name())
		}
	}
	closeRootProcess(t, process, "close ASR mapped-rollback process")
	assertLocalAIOMNIHostReleased(t, launcher, "ASR mapped-rollback")
	if err := fixture.Close(); err != nil {
		t.Fatalf("close ASR mapped-rollback fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if network.Calls() != 0 {
		t.Fatalf("ASR mapped-rollback model-asset network calls = %d, want zero", network.Calls())
	}
}

type localAIASRBlockingBackend struct {
	started chan struct{}
	once    sync.Once
}

func (backend *localAIASRBlockingBackend) Invoke(ctx context.Context, _ models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	backend.once.Do(func() { close(backend.started) })
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

func buildLocalAIASRProcess(
	t *testing.T,
	options localai.Options,
	mutate func(*serviceedges.Edges),
) (support.Process, string, []string, *localai.Fixture, *rejectingModelAssetHTTP, *recordingModelHostLauncher) {
	t.Helper()
	home := functionalTempDir(t)
	fixture := functionalStartLocalAI(t, options)
	writeGenericConformanceCaches(t, home)
	dir := functionalScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))
	edges, network, _, launcher := localAIConformanceEdges(home, fixture)
	if mutate != nil {
		mutate(&edges)
	}
	return functionalBuildProcess(t, edges), dir, localAIOMNIEnvironment(home), fixture, network, launcher
}
