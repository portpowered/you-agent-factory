package root_composition_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsASRBoundedInvalidInputsAreTypedAndEffectFree proves the remaining
// public CLI preflight forms for the ASR operation. Each invalid request uses
// an isolated root composition and observes the typed failure, rendered
// diagnostic, empty stdout, and absence of model lifecycle or asset effects.
func TestModelsASRBoundedInvalidInputsAreTypedAndEffectFree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		arguments   func(*testing.T) []string
		wantCode    factoryapi.ErrorResponseCode
		wantClass   models.InvocationFailureClass
		wantSlot    string
		wantMessage string
	}{
		{
			name: "missing file",
			arguments: func(t *testing.T) []string {
				return []string{"--operation", "ASR", "--input", "audio=@" + filepath.Join(t.TempDir(), "missing.wav")}
			},
			wantCode:    factoryapi.ErrorResponseCode("CLI_LOCAL_INPUT_FAILED"),
			wantMessage: "failed to load --input",
		},
		{
			name: "wrong media",
			arguments: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "notes.txt")
				if err := os.WriteFile(path, []byte("not audio"), 0o600); err != nil {
					t.Fatalf("write wrong-media ASR input: %v", err)
				}
				return []string{"--operation", "ASR", "--input", "audio=@" + path}
			},
			wantCode:    factoryapi.ErrorResponseCode("BAD_REQUEST"),
			wantClass:   models.InvocationFailureClassMediaCapability,
			wantSlot:    "audio",
			wantMessage: "does not accept media type",
		},
		{
			name: "duplicate audio",
			arguments: func(t *testing.T) []string {
				_, inputPath, _, _, _ := loadASRStoryFixture(t)
				return []string{
					"--operation", "ASR",
					"--input", "audio=@" + inputPath,
					"--input", "audio=@" + inputPath,
				}
			},
			wantCode:    factoryapi.ErrorResponseCode("BAD_REQUEST"),
			wantClass:   models.InvocationFailureClassSlotArity,
			wantSlot:    "audio",
			wantMessage: "accepts at most one value",
		},
		{
			name: "malformed parameter",
			arguments: func(t *testing.T) []string {
				_, inputPath, _, _, _ := loadASRStoryFixture(t)
				return []string{
					"--operation", "ASR",
					"--input", "audio=@" + inputPath,
					"--parameter", `{"name":"language","value":}`,
				}
			},
			wantCode:    factoryapi.ErrorResponseCode("BAD_REQUEST"),
			wantClass:   models.InvocationFailureClassInvalidParameter,
			wantMessage: "parse --parameter 1",
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newInvalidGenericCLIProcess(t, genericConformanceFactoryConfig)
			defer fixture.close(t)
			arguments := testCase.arguments(t)
			beforeEffects := fixture.effectSnapshot()

			for _, jsonMode := range []bool{false, true} {
				args := []string{"you"}
				if jsonMode {
					args = append(args, "--json")
				}
				args = append(args, "models", "invoke", models.BuiltInModelNameASR)
				args = append(args, arguments...)
				if !jsonMode {
					outputDir := t.TempDir()
					args = append(args,
						"--output-map", "transcript="+filepath.Join(outputDir, "transcript.txt"),
						"--output-map", "segments="+filepath.Join(outputDir, "segments.json"),
					)
				}

				var stdout, stderr bytes.Buffer
				inputs := support.FakeInputs(t.Context(), args)
				inputs.Input.Env = fixture.environment
				inputs.Input.WorkingDirectory = fixture.directory
				inputs.Input.Stdout = &stdout
				inputs.Input.Stderr = &stderr
				err := fixture.process.Execute(inputs.Input)
				if err == nil {
					t.Fatalf("ASR %s invocation returned nil, want bounded failure", map[bool]string{false: "human", true: "JSON"}[jsonMode])
				}
				if stdout.Len() != 0 {
					t.Fatalf("ASR %s invalid-input stdout = %q, want empty", map[bool]string{false: "human", true: "JSON"}[jsonMode], stdout.String())
				}

				if testCase.wantClass != "" {
					var failure *models.InvocationFailure
					if !errors.As(err, &failure) || failure == nil || failure.Class != testCase.wantClass || (testCase.wantSlot != "" && failure.Slot != testCase.wantSlot) {
						t.Fatalf("ASR %s typed failure = %v, failure = %#v, want class %s slot %q", map[bool]string{false: "human", true: "JSON"}[jsonMode], err, failure, testCase.wantClass, testCase.wantSlot)
					}
				}

				diagnostic := decodeFirstDiagnostic(t, stderr.String())
				if diagnostic.Code != testCase.wantCode || diagnostic.Family != factoryapi.ErrorFamilyBadRequest || !strings.Contains(diagnostic.Message, testCase.wantMessage) {
					t.Fatalf("ASR %s invalid-input diagnostic = %#v, want code %s and message containing %q", map[bool]string{false: "human", true: "JSON"}[jsonMode], diagnostic, testCase.wantCode, testCase.wantMessage)
				}
				if nonEmptyDiagnosticLines(stderr.String()) != 1 {
					t.Fatalf("ASR %s invalid-input diagnostic lines = %d, want one", map[bool]string{false: "human", true: "JSON"}[jsonMode], nonEmptyDiagnosticLines(stderr.String()))
				}
			}

			fixture.assertNoEffectsSince(t, beforeEffects)
			t.Logf("ASR bounded invalid-input proof: human and JSON requests returned %s with no downstream model or asset effects", testCase.wantCode)
		})
	}
}

// TestModelsASRBoundedRepeatOnReusableRootProcessReleasesEachHostOnce proves
// stable semantic values across repeated exact-fixture requests. The same
// public root process is reused while each request-scoped managed host is
// released exactly once.
func TestModelsASRBoundedRepeatOnReusableRootProcessReleasesEachHostOnce(t *testing.T) {
	t.Parallel()

	story := setupASRStory(t)
	first := runASRJSONInvocation(t, story)
	firstObservation, err := localAIASRObservationFromResponse(first)
	if err != nil {
		t.Fatalf("normalize first repeated ASR response: %v", err)
	}
	assertASRStoryBackendRequest(t, story, *story.received)

	second := runASRJSONInvocation(t, story)
	secondObservation, err := localAIASRObservationFromResponse(second)
	if err != nil {
		t.Fatalf("normalize second repeated ASR response: %v", err)
	}
	if !reflect.DeepEqual(firstObservation, secondObservation) {
		t.Fatalf("repeated ASR observations differ: first=%#v second=%#v", firstObservation, secondObservation)
	}
	assertASRStoryBackendRequest(t, story, *story.received)

	transcriptionCalls := 0
	for _, call := range story.fixture.Calls() {
		if call.Method == "AudioTranscription" {
			transcriptionCalls++
			if call.Prompt != base64ASRStoryInput(story.inputBytes) {
				t.Fatalf("repeated ASR fixture prompt = %q, want exact fixture bytes", call.Prompt)
			}
		}
	}
	if transcriptionCalls != 2 {
		t.Fatalf("repeated ASR fixture transcription calls = %d, want one per invocation", transcriptionCalls)
	}
	if story.hostLauncher.Calls() != 2 {
		t.Fatalf("repeated ASR managed host starts = %d, want one request-scoped host per invocation on the reused root process", story.hostLauncher.Calls())
	}
	if story.hostLauncher.StopCalls() != 2 {
		t.Fatalf("repeated ASR managed host stops = %d, want exactly one release per started host", story.hostLauncher.StopCalls())
	}
	if story.rejectingNetwork.Calls() != 0 {
		t.Fatalf("repeated ASR asset network calls = %d, want zero", story.rejectingNetwork.Calls())
	}

	closeRootProcess(t, story.process, "close repeated ASR root process")
	assertLocalAIOMNIHostReleased(t, story.hostLauncher, "repeated ASR")
	if story.hostLauncher.StopCalls() != 2 {
		t.Fatalf("repeated ASR managed host stops after root close = %d, want no duplicate release", story.hostLauncher.StopCalls())
	}
	if err := story.fixture.Close(); err != nil {
		t.Fatalf("close repeated ASR protocol fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, story.fixture.Endpoint())
	t.Logf("ASR repeat/release proof: two exact-fixture invocations returned stable observations on one reused root process, released each managed host exactly once, and released the fixture listener")
}

func base64ASRStoryInput(input []byte) string {
	return base64.StdEncoding.EncodeToString(input)
}
