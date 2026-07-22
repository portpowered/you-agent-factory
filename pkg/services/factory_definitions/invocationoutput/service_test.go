package invocationoutput

import (
	"encoding/json"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestPackagedInvocationOutputRoleSelectionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workstation *factorydefinitions.FactoryWorkstationConfig
		summary     bool
		response    bool
		tts         bool
	}{
		{name: "nil"},
		{
			name: "goal model",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: "execute-goal", Type: factorydefinitions.WorkstationTypeModel,
			},
			summary: true,
		},
		{
			name: "subagent agent",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: "run-subagent", Type: factorydefinitions.WorkstationTypeAgent,
			},
			response: true,
		},
		{
			name: "tts inference",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name:      factorydefinitions.PackagedTTSInvokeWorkstationName,
				Type:      factorydefinitions.WorkstationTypeInvoke,
				Operation: "tts",
			},
			tts: true,
		},
		{
			name: "customer workstation with packaged-like operation",
			workstation: &factorydefinitions.FactoryWorkstationConfig{
				Name: "customer-tts", Type: factorydefinitions.WorkstationTypeInvoke, Operation: "TTS",
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldFormatInvocationSummary(testCase.workstation); got != testCase.summary {
				t.Fatalf("ShouldFormatInvocationSummary() = %t, want %t", got, testCase.summary)
			}
			if got := ShouldFormatInvocationResponse(testCase.workstation); got != testCase.response {
				t.Fatalf("ShouldFormatInvocationResponse() = %t, want %t", got, testCase.response)
			}
			if got := ShouldFormatTTSInvocationMetadata(testCase.workstation); got != testCase.tts {
				t.Fatalf("ShouldFormatTTSInvocationMetadata() = %t, want %t", got, testCase.tts)
			}
		})
	}
}

func TestPackagedTextOutputShapingNormalizesStopTokens(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		shape func(string, string) ([]work.WorkContentPart, error)
		input string
		want  string
	}{
		{
			name:  "goal summary",
			shape: SummaryContentFromWorkerOutput,
			input: "Final goal summary.\nCOMPLETE",
			want:  "Final goal summary.",
		},
		{
			name:  "subagent response",
			shape: ResponseContentFromWorkerOutput,
			input: "mock worker accepted\nCOMPLETE",
			want:  "mock worker accepted",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			content, err := testCase.shape(testCase.input, "COMPLETE")
			if err != nil {
				t.Fatalf("shape output: %v", err)
			}
			if len(content) != 1 ||
				content[0].Type != work.WorkContentPartTypeText ||
				content[0].Text != testCase.want {
				t.Fatalf("content = %#v, want one text part %q", content, testCase.want)
			}
		})
	}
}

func TestPackagedTTSOutputShapingUsesAudioAndEditedWorkerBackend(t *testing.T) {
	t.Parallel()

	backend := TTSBackendLabelFromWorker(&factorydefinitions.FactoryWorkerConfig{
		Model: "CUSTOMER_EDITED_TTS_MODEL",
	})
	if backend != "CUSTOMER_EDITED_TTS_MODEL/LLAMACPP" {
		t.Fatalf("backend = %q, want edited model with default backend", backend)
	}

	content, err := TTSMetadataContentFromWorkerOutput(
		`[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`,
		"trace-1",
		"session-1",
		backend,
	)
	if err != nil {
		t.Fatalf("TTSMetadataContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("content = %#v, want one metadata text part", content)
	}
	var metadata factorydefinitions.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(content[0].Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.ArtifactPath != "/tmp/speech.wav" ||
		metadata.MediaType != "audio/wav" ||
		metadata.Backend != backend {
		t.Fatalf("metadata = %#v, want audio artifact and edited backend", metadata)
	}
}
