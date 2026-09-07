package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

const localAIOMNIRedactionSentinel = "OMNI_PARITY_SECRET_QUERY_7c9f2a"

type localAIOMNIParityCase struct {
	row       conformance.Row
	malformed bool
}

type localAIOMNIPublicObservation struct {
	Text        string
	Modality    string
	ContentType string
	MediaType   string
	OutputName  string
	OutputCount int
	HasArtifact bool
	Diagnostics string
}

type localAIOMNIFailureDiagnostic struct {
	Class   models.InvocationFailureClass
	Code    string
	Family  factoryapi.ErrorFamily
	Message string
}

type localAIOMNIPathCheckpoint struct {
	protocolRequests int
	protocolFailures int
	fixtureCalls     int
}

type localAIOMNIPathTrace struct {
	label            string
	protocolRequests []models.InvocationProtocolRequest
	protocolFailures []error
	fixturePredicts  []localai.Call
}

type localAIOMNICLIResult struct {
	Observation localAIOMNIPublicObservation
	Response    factoryapi.GenericModelInvocationResponse
	Stdout      string
	Stderr      string
	Err         error
}

type localAIOMNIProtocolRecorder struct {
	mu       sync.Mutex
	requests []models.InvocationProtocolRequest
	failures []error
	attempts *atomic.Int32
	next     interface {
		Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error)
	}
}

type localAIOMNIProcess interface {
	Execute(root.Input) error
}

type localAIOMNIExecutor struct {
	fn func(root.Input) error
}

func (executor localAIOMNIExecutor) Execute(input root.Input) error {
	return executor.fn(input)
}

func TestLocalAIOMNIControlledCLIHTTPAndExplicitServerParity(t *testing.T) {
	t.Parallel()

	cases := localAIOMNIParityCases()
	var totalAttempts atomic.Int32
	t.Cleanup(func() {
		if got := totalAttempts.Load(); got > int32(len(cases)*3) {
			t.Errorf("LocalAI OMNI invocation attempts = %d, want at most %d", got, len(cases)*3)
		}
	})
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.row.Label, func(t *testing.T) {
			t.Parallel()
			runLocalAIOMNIParityCase(t, testCase, &totalAttempts)
		})
	}
}

func localAIOMNIParityCases() []localAIOMNIParityCase {
	rows := conformance.Build(models.GenericOperationCatalog{}).Rows
	cases := make([]localAIOMNIParityCase, 0, 5)
	for _, row := range rows {
		if row.Operation.Name != models.OperationOMNI {
			continue
		}
		row = row.Clone()
		switch row.Variant {
		case conformance.VariantPromptOnly:
			row.Inputs[0].Content = "Describe the request"
		case conformance.VariantSingleImage:
			row.Inputs[0].Content = "Describe the image"
			row.Inputs[1].Content = "PNG_BYTES_FIRST"
		case conformance.VariantMultipleImage:
			row.Inputs[0].Content = "Compare these images in order"
			row.Inputs[1].Content = "PNG_BYTES_FIRST"
			row.Inputs[2].Content = "JPEG_BYTES_SECOND"
		case conformance.VariantVideo:
			row.Inputs[0].Content = "What happens at 0:30?"
			row.Inputs[1].Content = "MP4_BYTES_TIMELINE"
		}
		cases = append(cases, localAIOMNIParityCase{row: row})
	}

	malformed := cases[0].row.Clone()
	malformed.Label = models.OperationOMNI + "/malformed-response"
	malformed.Variant = "malformed-response"
	malformed.Inputs[0].Content = "Describe this " + localAIOMNIRedactionSentinel + "?"
	cases = append(cases, localAIOMNIParityCase{row: malformed, malformed: true})
	return cases
}

func runLocalAIOMNIParityCase(
	t *testing.T,
	testCase localAIOMNIParityCase,
	totalAttempts *atomic.Int32,
) {
	t.Helper()
	home := functionalTempDir(t)
	environment := localAIOMNIEnvironment(home)
	fixtureOptions := localai.Options{}
	if testCase.malformed {
		fixtureOptions.Mode = localai.ModeMalformedResponse
	}
	fixture := functionalStartLocalAI(t, fixtureOptions)
	dir := functionalScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))

	// The Factory scaffold is immutable for this row; all external behavior is
	// supplied by the fixture-backed host and protocol edges.
	writeGenericConformanceCaches(t, home)
	var recorder localAIOMNIProtocolRecorder
	recorder.attempts = totalAttempts

	localEdges, localNetwork, _, localLauncher := localAIOMNIParityEdges(home, fixture, &recorder)
	localProcess := functionalBuildProcess(t, localEdges)
	localCheckpoint := localAIOMNIPathCheckpointAt(&recorder, fixture)
	localResult := executeLocalAIOMNIParityCLI(
		t, localProcess, dir, environment, testCase.row, "",
	)
	localTrace := localAIOMNIPathTraceAt(t, "direct local CLI", &recorder, fixture, localCheckpoint)
	closeRootProcess(t, localProcess, "close local OMNI process")
	assertLocalAIOMNIHostReleased(t, localLauncher, "local")

	hostedEdges, hostedNetwork, _, hostedLauncher := localAIOMNIParityEdges(home, fixture, &recorder)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       environment,
		Edges:                     hostedEdges,
	})

	httpCheckpoint := localAIOMNIPathCheckpointAt(&recorder, fixture)
	httpObservation, httpFailure := executeLocalAIOMNIHTTP(
		t, server.URL()+"/models/invocations", testCase.row,
	)
	httpTrace := localAIOMNIPathTraceAt(t, "direct HTTP", &recorder, fixture, httpCheckpoint)
	serverCheckpoint := localAIOMNIPathCheckpointAt(&recorder, fixture)
	serverCLIResult := executeLocalAIOMNIParityCLI(
		t, localAIOMNIExecutor{fn: func(input root.Input) error {
			return server.Execute(t, input)
		}}, dir, environment, testCase.row, server.URL(),
	)
	serverTrace := localAIOMNIPathTraceAt(t, "configured --server CLI", &recorder, fixture, serverCheckpoint)
	paths := []localAIOMNIPathTrace{localTrace, httpTrace, serverTrace}

	if testCase.malformed {
		assertLocalAIOMNIMalformedParity(
			t, testCase.row, localResult, httpObservation, httpFailure,
			serverCLIResult, fixture, home, dir, server.URL(), paths,
		)
	} else {
		assertLocalAIOMNISuccessParity(t, testCase.row, localResult, httpObservation, serverCLIResult)
	}
	assertLocalAIOMNIProtocolInputs(t, testCase.row, paths)
	assertLocalAIOMNIFixtureCalls(t, testCase.row, paths)

	server.Close(t)
	assertLocalAIOMNIServerReleased(t, server, hostedLauncher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
	if calls := localNetwork.Calls() + hostedNetwork.Calls(); calls != 0 {
		t.Fatalf("LocalAI model-asset network calls = %d, want zero", calls)
	}
}

func localAIOMNIParityEdges(
	home string,
	fixture *localai.Fixture,
	recorder *localAIOMNIProtocolRecorder,
) (serviceedges.Edges, *rejectingModelAssetHTTP, *joinedCompatibilityChecker, *recordingModelHostLauncher) {
	edges, network, compatibility, launcher := localAIConformanceEdges(home, fixture)
	recorder.next = localAIInvocationProtocolClient{fixture: fixture}
	edges.ModelInvocationProtocolClient = recorder
	return edges, network, compatibility, launcher
}

func localAIOMNIEnvironment(home string) []string {
	inherited := functionalHomeEnvironment(home)
	result := make([]string, 0, len(inherited)+3)
	for _, entry := range inherited {
		key := strings.ToLower(strings.TrimSpace(strings.SplitN(entry, "=", 2)[0]))
		switch key {
		case "home", "userprofile", "homedrive", "homepath":
			continue
		default:
			result = append(result, entry)
		}
	}
	switch runtime.GOOS {
	case "windows":
		drive := filepath.VolumeName(home)
		homePath := strings.TrimPrefix(home, drive)
		result = append(result,
			"USERPROFILE="+home,
			"HOMEDRIVE="+drive,
			"HOMEPATH="+homePath,
		)
	case "plan9":
		result = append(result, "home="+home)
	default:
		result = append(result, "HOME="+home)
	}
	return result
}

func (recorder *localAIOMNIProtocolRecorder) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if recorder.attempts != nil {
		recorder.attempts.Add(1)
	}
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, localAIOMNICloneProtocolRequest(request))
	recorder.mu.Unlock()
	response, err := recorder.next.Predict(ctx, request)
	if err != nil {
		recorder.mu.Lock()
		recorder.failures = append(recorder.failures, err)
		recorder.mu.Unlock()
	}
	return response, err
}

func (recorder *localAIOMNIProtocolRecorder) Requests() []models.InvocationProtocolRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	requests := make([]models.InvocationProtocolRequest, len(recorder.requests))
	for index, request := range recorder.requests {
		requests[index] = localAIOMNICloneProtocolRequest(request)
	}
	return requests
}

func (recorder *localAIOMNIProtocolRecorder) Failures() []error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]error(nil), recorder.failures...)
}

func localAIOMNICloneProtocolRequest(request models.InvocationProtocolRequest) models.InvocationProtocolRequest {
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	request.Parameters = append([]models.OperationParameter(nil), request.Parameters...)
	return request
}

func localAIOMNIPathCheckpointAt(
	recorder *localAIOMNIProtocolRecorder,
	fixture *localai.Fixture,
) localAIOMNIPathCheckpoint {
	return localAIOMNIPathCheckpoint{
		protocolRequests: len(recorder.Requests()),
		protocolFailures: len(recorder.Failures()),
		fixtureCalls:     len(fixture.Calls()),
	}
}

func localAIOMNIPathTraceAt(
	t testing.TB,
	label string,
	recorder *localAIOMNIProtocolRecorder,
	fixture *localai.Fixture,
	checkpoint localAIOMNIPathCheckpoint,
) localAIOMNIPathTrace {
	t.Helper()
	requests := recorder.Requests()
	failures := recorder.Failures()
	calls := fixture.Calls()
	if checkpoint.protocolRequests > len(requests) || checkpoint.protocolFailures > len(failures) || checkpoint.fixtureCalls > len(calls) {
		t.Fatalf("%s path trace moved backwards: protocol %d/%d failures %d/%d fixture %d/%d", label, checkpoint.protocolRequests, len(requests), checkpoint.protocolFailures, len(failures), checkpoint.fixtureCalls, len(calls))
		return localAIOMNIPathTrace{label: label}
	}
	predicts := make([]localai.Call, 0, len(calls)-checkpoint.fixtureCalls)
	for _, call := range calls[checkpoint.fixtureCalls:] {
		if call.Method == "Predict" {
			predicts = append(predicts, call)
		}
	}
	return localAIOMNIPathTrace{
		label:            label,
		protocolRequests: requests[checkpoint.protocolRequests:],
		protocolFailures: failures[checkpoint.protocolFailures:],
		fixturePredicts:  predicts,
	}
}

func executeLocalAIOMNIParityCLI(
	t testing.TB,
	process localAIOMNIProcess,
	dir string,
	environment []string,
	row conformance.Row,
	serverURL string,
) localAIOMNICLIResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), localAIOMNICLIArgs(row, serverURL))
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = dir
	err := process.Execute(inputs.Input)
	result := localAIOMNICLIResult{
		Stdout: inputs.Stdout(),
		Stderr: inputs.Stderr(),
		Err:    err,
	}
	if err != nil {
		return result
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &result.Response); err != nil {
		result.Err = fmt.Errorf("%s CLI response was not JSON", row.Label)
		return result
	}
	if err := assertConformanceResponse(row, result.Response); err != nil {
		result.Err = fmt.Errorf("%s CLI semantic response failed", row.Label)
		return result
	}
	result.Observation, result.Err = localAIOMNIObservationFromResponse(result.Response, result.Stderr)
	return result
}

func localAIOMNICLIArgs(row conformance.Row, serverURL string) []string {
	args := []string{"you", "--json"}
	if strings.TrimSpace(serverURL) != "" {
		args = append(args, "--server", strings.TrimSuffix(serverURL, "/"))
	}
	args = append(args, "models", "invoke", conformanceModelName(row.Operation.Name), "--operation", row.Operation.Name)
	for _, input := range row.Inputs {
		value, _ := json.Marshal(struct {
			Name        string `json:"name"`
			Modality    string `json:"modality"`
			ContentType string `json:"contentType"`
			MediaType   string `json:"mediaType"`
			Content     string `json:"content"`
		}{
			Name: input.Name, Modality: string(input.Modality),
			ContentType: input.ContentType, MediaType: input.MediaType,
			Content: input.Content,
		})
		args = append(args, "--input", string(value))
	}
	if strings.TrimSpace(serverURL) == "" {
		for _, parameter := range conformanceParameters() {
			value, _ := json.Marshal(struct {
				Name  string `json:"name"`
				Value any    `json:"value"`
			}{Name: parameter.Name, Value: parameter.Value})
			args = append(args, "--parameter", string(value))
		}
	}
	return args
}

func executeLocalAIOMNIHTTP(
	t testing.TB,
	endpoint string,
	row conformance.Row,
) (localAIOMNIPublicObservation, *localAIOMNIFailureDiagnostic) {
	t.Helper()
	response, failure, status, err := postConformanceInvocation(t.Context(), endpoint, row)
	if err != nil {
		t.Fatalf("%s direct HTTP request failed before a response", row.Label)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		if failure.Message == "" {
			t.Fatalf("%s direct HTTP returned an unstructured failure", row.Label)
		}
		return localAIOMNIPublicObservation{}, &localAIOMNIFailureDiagnostic{
			Code: string(failure.Code), Family: failure.Family, Message: failure.Message,
		}
	}
	observation, err := localAIOMNIObservationFromResponse(response, "")
	if err != nil {
		t.Fatalf("%s direct HTTP returned an invalid success response", row.Label)
	}
	return observation, nil
}

func localAIOMNIObservationFromResponse(
	response factoryapi.GenericModelInvocationResponse,
	diagnostics string,
) (localAIOMNIPublicObservation, error) {
	if response.Failure != nil || len(response.Outputs) != 1 {
		return localAIOMNIPublicObservation{}, errors.New("unexpected generic response shape")
	}
	output := response.Outputs[0]
	if output.Content == nil || output.ContentType == nil || output.MediaType == nil {
		return localAIOMNIPublicObservation{}, errors.New("generic response omitted content metadata")
	}
	return localAIOMNIPublicObservation{
		Text:        *output.Content,
		Modality:    string(output.Modality),
		ContentType: *output.ContentType,
		MediaType:   *output.MediaType,
		OutputName:  output.Name,
		OutputCount: len(response.Outputs),
		HasArtifact: output.Artifact != nil,
		Diagnostics: diagnostics,
	}, nil
}

func assertLocalAIOMNISuccessParity(
	t *testing.T,
	row conformance.Row,
	localResult localAIOMNICLIResult,
	httpObservation localAIOMNIPublicObservation,
	serverResult localAIOMNICLIResult,
) {
	t.Helper()
	if localResult.Err != nil || serverResult.Err != nil || localResult.Stdout == "" || serverResult.Stdout == "" {
		t.Fatalf("%s did not complete successfully on all CLI paths", row.Label)
	}
	if localResult.Observation != serverResult.Observation || localResult.Observation != httpObservation {
		t.Fatalf("%s public observations differ across CLI and HTTP paths", row.Label)
	}
	want := localAIOMNIExpectedText(row)
	if localResult.Observation.Text != want {
		t.Fatalf("%s semantic text differs from the controlled OMNI response", row.Label)
	}
	if localResult.Observation.Modality != string(factoryapi.ModelInvocationContentTypeText) ||
		localResult.Observation.ContentType != "text/plain" || localResult.Observation.MediaType != "text/plain" ||
		localResult.Observation.OutputName != "text" || localResult.Observation.OutputCount != 1 ||
		localResult.Observation.Diagnostics != "" {
		t.Fatalf("%s success metadata is not the normalized text/plain contract", row.Label)
	}
	if row.Variant == conformance.VariantVideo && !strings.Contains(localResult.Observation.Text, "0:30") {
		t.Fatalf("%s video response omitted the requested 0:30 observation", row.Label)
	}
}

func assertLocalAIOMNIMalformedParity(
	t *testing.T,
	row conformance.Row,
	localResult localAIOMNICLIResult,
	httpObservation localAIOMNIPublicObservation,
	httpFailure *localAIOMNIFailureDiagnostic,
	serverResult localAIOMNICLIResult,
	fixture *localai.Fixture,
	home string,
	dir string,
	serverURL string,
	paths []localAIOMNIPathTrace,
) {
	t.Helper()
	if localResult.Err == nil || serverResult.Err == nil || httpFailure == nil {
		t.Fatalf("%s malformed row did not fail on all three public paths", row.Label)
	}
	if localResult.Stdout != "" || serverResult.Stdout != "" || httpObservation.OutputCount != 0 {
		t.Fatalf("%s malformed row published partial output", row.Label)
	}
	localFailure, ok := localAIOMNIFailureFromError(localResult.Err)
	if !ok {
		t.Fatalf("%s local CLI failure was not typed", row.Label)
	}
	// The configured --server CLI receives the public HTTP ErrorResponse, so
	// that transport's returned APIError intentionally ends the Go error chain.
	// Assert its delivered diagnostic above and assert the actual InvocationFailure
	// at the protocol edge below; never infer the class from the wire code.
	serverFailure, ok := localAIOMNIErrorDiagnosticFromError(serverResult.Err)
	if !ok {
		t.Fatalf("%s configured-server CLI failure was not typed: %T %v", row.Label, serverResult.Err, serverResult.Err)
	}
	for _, failure := range []*localAIOMNIFailureDiagnostic{localFailure, httpFailure, serverFailure} {
		if failure.Code != "MODEL_BACKEND_FAILURE" || failure.Family != factoryapi.ErrorFamilyInternalServerError || failure.Message == "" {
			t.Fatalf("%s malformed failure category is not the provider-neutral backend failure", row.Label)
		}
	}
	if localFailure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("%s local CLI failure class = %s, want MALFORMED_RESPONSE", row.Label, localFailure.Class)
	}
	for _, path := range paths {
		if len(path.protocolFailures) != 1 {
			t.Fatalf("%s %s underlying protocol failure count = %d, want exactly 1", row.Label, path.label, len(path.protocolFailures))
		}
		underlying, ok := localAIOMNIInvocationFailureFromError(path.protocolFailures[0])
		if !ok || underlying.Class != models.InvocationFailureClassMalformedResponse {
			t.Fatalf("%s %s underlying protocol failure = %T %v, want MALFORMED_RESPONSE InvocationFailure", row.Label, path.label, path.protocolFailures[0], path.protocolFailures[0])
		}
	}
	if localFailure.Message != httpFailure.Message || localFailure.Message != serverFailure.Message {
		t.Fatalf("%s malformed safe messages differ across public paths", row.Label)
	}
	redacted := []string{localAIOMNIRedactionSentinel, fixture.Endpoint(), home, dir, serverURL}
	for _, value := range redacted {
		if strings.Contains(localFailure.Message, value) || strings.Contains(httpFailure.Message, value) ||
			strings.Contains(serverFailure.Message, value) || strings.Contains(localResult.Stderr, value) ||
			strings.Contains(serverResult.Stderr, value) || strings.Contains(localResult.Err.Error(), value) ||
			strings.Contains(serverResult.Err.Error(), value) {
			t.Fatalf("%s malformed diagnostics exposed redacted data", row.Label)
		}
	}
}

func localAIOMNIFailureFromError(err error) (*localAIOMNIFailureDiagnostic, bool) {
	diagnostic, ok := localAIOMNIErrorDiagnosticFromError(err)
	if !ok {
		return nil, false
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		return nil, false
	}
	diagnostic.Class = failure.Class
	return diagnostic, true
}

func localAIOMNIInvocationFailureFromError(err error) (*models.InvocationFailure, bool) {
	if err == nil {
		return nil, false
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		return nil, false
	}
	return failure, true
}

func localAIOMNIErrorDiagnosticFromError(err error) (*localAIOMNIFailureDiagnostic, bool) {
	if err == nil {
		return nil, false
	}
	var coded clidiag.FamilyCodedError
	if !errors.As(err, &coded) {
		return nil, false
	}
	return &localAIOMNIFailureDiagnostic{
		Code:   coded.CLIErrorCode(),
		Family: coded.CLIErrorFamily(), Message: coded.CLIErrorMessage(),
	}, true
}

func localAIOMNIExpectedText(row conformance.Row) string {
	var prompt string
	var images, audios, videos []string
	for _, input := range row.Inputs {
		switch input.Modality {
		case models.ModalityText:
			prompt = input.Content
		case models.ModalityImage:
			images = append(images, input.Content)
		case models.ModalityAudio:
			audios = append(audios, input.Content)
		case models.ModalityVideo:
			videos = append(videos, input.Content)
		}
	}
	return localai.ExpectedOmniText(prompt, images, audios, videos)
}

func assertLocalAIOMNIProtocolInputs(
	t *testing.T,
	row conformance.Row,
	paths []localAIOMNIPathTrace,
) {
	t.Helper()
	if len(paths) != 3 {
		t.Fatalf("%s protocol path count = %d, want exactly 3", row.Label, len(paths))
	}
	for _, path := range paths {
		if len(path.protocolRequests) != 1 {
			t.Fatalf("%s %s protocol request count = %d, want exactly 1", row.Label, path.label, len(path.protocolRequests))
		}
		request := path.protocolRequests[0]
		if request.Operation != models.OperationOMNI || request.Prompt != localAIOMNIExpectedPrompt(row) || len(request.Inputs) != len(row.Inputs) {
			t.Fatalf("%s %s protocol request shape was not preserved", row.Label, path.label)
		}
		for index, input := range request.Inputs {
			want := row.Inputs[index]
			if input.Slot != want.Name || input.Modality != want.Modality || input.MediaType != want.MediaType || input.Content != want.Content {
				t.Fatalf("%s %s protocol input order, media type, or bytes changed", row.Label, path.label)
			}
		}
	}
}

func localAIOMNIExpectedPrompt(row conformance.Row) string {
	for _, input := range row.Inputs {
		if input.Modality == models.ModalityText {
			return input.Content
		}
	}
	return ""
}

func assertLocalAIOMNIFixtureCalls(
	t *testing.T,
	row conformance.Row,
	paths []localAIOMNIPathTrace,
) {
	t.Helper()
	if len(paths) != 3 {
		t.Fatalf("%s fixture path count = %d, want exactly 3", row.Label, len(paths))
	}
	for _, path := range paths {
		if len(path.fixturePredicts) != 1 {
			t.Fatalf("%s %s controlled LocalAI Predict calls = %d, want exactly 1", row.Label, path.label, len(path.fixturePredicts))
		}
		call := path.fixturePredicts[0]
		var wantImages, wantVideos []string
		for _, input := range row.Inputs {
			switch input.Modality {
			case models.ModalityImage:
				wantImages = append(wantImages, input.Content)
			case models.ModalityVideo:
				wantVideos = append(wantVideos, input.Content)
			}
		}
		if call.Prompt != localAIOMNIExpectedPrompt(row) || !equalLocalAIOMNIStrings(call.Images, wantImages) || !equalLocalAIOMNIStrings(call.Videos, wantVideos) {
			t.Fatalf("%s %s controlled LocalAI bytes or repeated-input order changed", row.Label, path.label)
		}
	}
	if row.Variant == conformance.VariantVideo && !strings.Contains(localAIOMNIExpectedText(row), "0:30") {
		t.Fatalf("%s video fixture answer omitted the requested 0:30 observation", row.Label)
	}
}

func equalLocalAIOMNIStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertLocalAIOMNIHostReleased(t *testing.T, launcher *recordingModelHostLauncher, label string) {
	t.Helper()
	launcher.mu.Lock()
	active := launcher.active
	launcher.mu.Unlock()
	if active {
		t.Fatalf("%s managed LocalAI host remained active after process close", label)
	}
	if launcher.Calls() == 0 {
		t.Fatalf("%s managed LocalAI host was never exercised", label)
	}
}

func assertLocalAIOMNIServerReleased(
	t *testing.T,
	server *support.FunctionalAPIServer,
	launcher *recordingModelHostLauncher,
) {
	t.Helper()
	select {
	case <-server.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("configured server process did not close")
	}
	assertLocalAIOMNIHostReleased(t, launcher, "configured server")
	client := &http.Client{Timeout: 250 * time.Millisecond}
	response, err := client.Get(server.URL() + "/status")
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("configured server listener remained reachable after close")
	}
}

func assertLocalAIOMNIFixtureListenerReleased(t *testing.T, endpoint string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", endpoint, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("LocalAI fixture listener remained reachable after close")
	}
}
