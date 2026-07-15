package tts

import (
	"encoding/json"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestMetadataContentFromWorkerOutput_ReturnsTextMetadataWithoutRawAudio(t *testing.T) {
	output := `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`

	got, err := MetadataContentFromWorkerOutput(output, "trace-tts", "~default", "")
	if err != nil {
		t.Fatalf("MetadataContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("content = %#v, want one text metadata part", got)
	}
	if strings.Contains(got[0].Text, "AUDIO") {
		t.Fatalf("metadata text = %q, want JSON metadata not raw audio content type marker", got[0].Text)
	}

	var metadata InvocationMetadata
	if err := json.Unmarshal([]byte(got[0].Text), &metadata); err != nil {
		t.Fatalf("metadata is not JSON: %v\n%s", err, got[0].Text)
	}
	if metadata.ArtifactPath != "/tmp/speech.wav" {
		t.Fatalf("artifactPath = %q", metadata.ArtifactPath)
	}
	if metadata.MediaType != "audio/wav" {
		t.Fatalf("mediaType = %q, want audio/wav", metadata.MediaType)
	}
	if metadata.Backend != "OMNIVOICE_Q4_K_M/LLAMACPP" {
		t.Fatalf("backend = %q", metadata.Backend)
	}
	if metadata.TraceID != "trace-tts" || metadata.SessionID != "~default" {
		t.Fatalf("metadata correlation = %#v", metadata)
	}
}

func TestShouldFormatInvocationMetadata_MatchesPackagedInvokeWorkstation(t *testing.T) {
	for _, workstationType := range []string{
		interfaces.WorkstationTypeInvoke,
		interfaces.WorkstationTypeInference,
	} {
		workstation := &interfaces.FactoryWorkstationConfig{
			Name:      PackagedInvokeWorkstationName,
			Type:      workstationType,
			Operation: "TTS",
		}
		if !ShouldFormatInvocationMetadata(workstation) {
			t.Fatalf("expected packaged invoke workstation type %q to require metadata formatting", workstationType)
		}
	}
}
