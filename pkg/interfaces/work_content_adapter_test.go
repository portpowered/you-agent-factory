package interfaces

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestWorkContentFromGeneratedPreservesParts(t *testing.T) {
	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(`[
		{"type":"text","text":"Inspect this"},
		{"type":"image","file":"fixtures/inventory.png"}
	]`), &content); err != nil {
		t.Fatalf("unmarshal generated content: %v", err)
	}

	got, err := WorkContentFromGenerated(&content)
	if err != nil {
		t.Fatalf("WorkContentFromGenerated returned error: %v", err)
	}

	want := []WorkContentPart{
		{Type: WorkContentPartTypeText, Text: "Inspect this"},
		{Type: WorkContentPartTypeImage, File: "fixtures/inventory.png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WorkContentFromGenerated() = %#v, want %#v", got, want)
	}
}

func TestWorkContentFromGeneratedNilAndEmptyContent(t *testing.T) {
	got, err := WorkContentFromGenerated(nil)
	if err != nil {
		t.Fatalf("WorkContentFromGenerated(nil) returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("WorkContentFromGenerated(nil) = %#v, want nil", got)
	}

	empty := factoryapi.WorkContent{}
	got, err = WorkContentFromGenerated(&empty)
	if err != nil {
		t.Fatalf("WorkContentFromGenerated(empty) returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("WorkContentFromGenerated(empty) = %#v, want nil", got)
	}
}

func TestWorkContentFromGeneratedRejectsUnsupportedPartWithFieldPath(t *testing.T) {
	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(`[{"type":"audio","file":"sound.wav"}]`), &content); err != nil {
		t.Fatalf("unmarshal generated content: %v", err)
	}

	got, err := WorkContentFromGeneratedAtPath(&content, "works[0].content")
	if err == nil {
		t.Fatalf("WorkContentFromGeneratedAtPath() error = nil, got parts %#v", got)
	}

	var validationErr WorkContentValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want WorkContentValidationError", err)
	}
	if validationErr.FieldPath != "works[0].content[0].type" {
		t.Fatalf("FieldPath = %q, want %q", validationErr.FieldPath, "works[0].content[0].type")
	}
	if err.Error() != "works[0].content[0].type must be one of text or image" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestGeneratedWorkContentPtrPreservesPartsAndOmissionBehavior(t *testing.T) {
	if got := GeneratedWorkContentPtr(nil); got != nil {
		t.Fatalf("GeneratedWorkContentPtr(nil) = %#v, want nil", got)
	}
	if got := GeneratedWorkContentPtr([]WorkContentPart{}); got != nil {
		t.Fatalf("GeneratedWorkContentPtr(empty) = %#v, want nil", got)
	}
	if got := GeneratedWorkContentPtr([]WorkContentPart{{Type: WorkContentPartType("unknown")}}); got != nil {
		t.Fatalf("GeneratedWorkContentPtr(unknown) = %#v, want nil", got)
	}

	got := GeneratedWorkContentPtr([]WorkContentPart{
		{Type: WorkContentPartTypeText, Text: "Inspect this"},
		{Type: WorkContentPartTypeImage, File: "fixtures/inventory.png"},
	})
	if got == nil {
		t.Fatal("GeneratedWorkContentPtr(valid) = nil")
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal generated content: %v", err)
	}
	want := `[{"text":"Inspect this","type":"text"},{"file":"fixtures/inventory.png","type":"image"}]`
	if string(data) != want {
		t.Fatalf("generated JSON = %s, want %s", data, want)
	}
}
