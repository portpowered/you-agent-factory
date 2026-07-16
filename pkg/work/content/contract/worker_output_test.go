package contentcontract

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestContentFromWorkerOutput_ReturnsPlainTextResponse(t *testing.T) {
	got, err := ContentFromWorkerOutput("worker response")
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeText || got[0].Text != "worker response" {
		t.Fatalf("content = %#v, want one text response part", got)
	}
}

func TestContentFromWorkerOutput_ParsesCanonicalPartArray(t *testing.T) {
	raw, err := json.Marshal([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "structured response",
	}})
	if err != nil {
		t.Fatalf("marshal parts: %v", err)
	}

	got, err := ContentFromWorkerOutput(string(raw))
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Text != "structured response" {
		t.Fatalf("content = %#v, want structured response part", got)
	}
}

func TestContentFromWorkerOutput_NormalizesAliasesAndOmitsUnknownParts(t *testing.T) {
	raw := `{"content":[{"type":"TEXT","text":"structured response"},{"type":"unknown","text":"ignored"}]}`

	got, err := ContentFromWorkerOutput(raw)
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeText || got[0].Text != "structured response" {
		t.Fatalf("content = %#v, want one normalized text response part", got)
	}
}

func TestContentFromWorkerOutput_ReturnsNilForEmptyOutput(t *testing.T) {
	got, err := ContentFromWorkerOutput("   ")
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if got != nil {
		t.Fatalf("content = %#v, want nil", got)
	}
}
