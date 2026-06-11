package result_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	jsresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

func TestValidateTypedValue_AcceptsStructuredJSON(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	result := jsresult.ValidateTypedValue(jsresult.TypedValue{JSON: raw})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none", result.Issues)
	}
}

func TestValidateTypedValue_RejectsPromiseFunctionCycleAndBinary(t *testing.T) {
	cases := []struct {
		name  string
		value jsresult.TypedValue
		code  string
	}{
		{
			name:  "promise",
			value: jsresult.TypedValue{Unresolved: true},
			code:  jsresult.CodeUnresolvedPromise,
		},
		{
			name:  "function",
			value: jsresult.TypedValue{Function: true},
			code:  jsresult.CodeUnsupportedType,
		},
		{
			name:  "host handle",
			value: jsresult.TypedValue{HostHandle: "fs.readFile"},
			code:  jsresult.CodeHostHandle,
		},
		{
			name:  "binary",
			value: jsresult.TypedValue{RawBinary: []byte{0x01}},
			code:  jsresult.CodeUnsupportedBinary,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := jsresult.ValidateTypedValue(tc.value)
			if !result.HasIssues() {
				t.Fatal("expected validation issues")
			}
			if result.Issues[0].Code != tc.code {
				t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, tc.code)
			}
		})
	}

	cycle := map[string]any{}
	cycle["self"] = cycle
	result := jsresult.ValidateTypedValue(jsresult.TypedValue{HostObject: cycle})
	if !result.HasIssues() {
		t.Fatal("expected cyclic value issue")
	}
	if result.Issues[0].Code != jsresult.CodeCyclicValue {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, jsresult.CodeCyclicValue)
	}
}

func TestParseArtifactURI_AcceptsCanonicalForm(t *testing.T) {
	uri := jsresult.FormatArtifactURI("session-1", "artifact-log-1")
	parsed, issues := jsresult.ParseArtifactURI(uri)
	if len(issues) > 0 {
		t.Fatalf("parse issues = %#v", issues)
	}
	if parsed.SessionID != "session-1" || parsed.ArtifactID != "artifact-log-1" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseArtifactURI_RejectsMalformedTraversalAndHostPaths(t *testing.T) {
	cases := []string{
		"",
		"file:///tmp/secret",
		"you-artifact://sessions/../artifacts/x",
		"you-artifact://sessions/session-1/artifacts/../escape",
		"you-artifact://sessions//artifacts/x",
	}
	for _, raw := range cases {
		if _, issues := jsresult.ParseArtifactURI(raw); len(issues) == 0 {
			t.Fatalf("parse(%q) = no issues, want rejection", raw)
		}
	}

	mismatch := jsresult.ValidateArtifactURIForSession(
		jsresult.FormatArtifactURI("session-a", "artifact-1"),
		"session-b",
	)
	if len(mismatch) == 0 || mismatch[0].Code != jsresult.CodeArtifactURISessionMismatch {
		t.Fatalf("session mismatch issues = %#v", mismatch)
	}
}

func TestProjectPrimaryResult_MapsJSONAndArtifactBackedOutputs(t *testing.T) {
	sessionID := "session-fixture"
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	parts, validation := jsresult.ProjectPrimaryResult(sessionID, jsresult.TypedValue{JSON: raw}, nil)
	if validation.HasIssues() {
		t.Fatalf("projection validation = %#v", validation.Issues)
	}
	if len(parts) != 1 || parts[0].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("json parts = %#v", parts)
	}

	imageURI := jsresult.FormatArtifactURI(sessionID, "artifact-image-1")
	imageRaw, err := json.Marshal(imageURI)
	if err != nil {
		t.Fatalf("marshal image uri: %v", err)
	}
	imageArtifacts := []interfaces.FactorySessionArtifactState{{
		ID:              "artifact-image-1",
		Kind:            "IMAGE",
		Visibility:      "PUBLIC",
		CaptureMetadata: map[string]string{"contentType": "image/png"},
	}}
	imageParts, imageValidation := jsresult.ProjectPrimaryResult(sessionID, jsresult.TypedValue{JSON: imageRaw}, imageArtifacts)
	if imageValidation.HasIssues() {
		t.Fatalf("image validation = %#v", imageValidation.Issues)
	}
	if len(imageParts) != 1 || imageParts[0].Type != interfaces.WorkContentPartTypeImage {
		t.Fatalf("image parts = %#v", imageParts)
	}
	if imageParts[0].URL != imageURI || imageParts[0].ArtifactID != "artifact-image-1" {
		t.Fatalf("image part = %#v", imageParts[0])
	}
}

func TestProjectPrimaryResult_RejectsCrossSessionArtifactURI(t *testing.T) {
	sessionID := "session-fixture"
	foreignURI := jsresult.FormatArtifactURI("session-other", "artifact-image-1")
	raw, err := json.Marshal(foreignURI)
	if err != nil {
		t.Fatalf("marshal foreign uri: %v", err)
	}
	artifacts := []interfaces.FactorySessionArtifactState{{
		ID:         "artifact-image-1",
		Kind:       "IMAGE",
		Visibility: "PUBLIC",
	}}
	parts, validation := jsresult.ProjectPrimaryResult(sessionID, jsresult.TypedValue{JSON: raw}, artifacts)
	if !validation.HasIssues() {
		t.Fatalf("parts = %#v, expected cross-session artifact URI rejection", parts)
	}
	if validation.Issues[0].Code != jsresult.CodeArtifactURISessionMismatch {
		t.Fatalf("issue code = %q, want %q", validation.Issues[0].Code, jsresult.CodeArtifactURISessionMismatch)
	}

	sessionResult := jsresult.BuildSessionResult(jsresult.SessionResultInput{
		SessionID:    sessionID,
		Status:       factoryapi.FactorySessionStatusFINISHED,
		PrimaryValue: jsresult.TypedValue{JSON: raw},
		Artifacts:    artifacts,
	})
	if sessionResult.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil for cross-session artifact URI", sessionResult.PrimaryResult)
	}
}

func TestBuildSessionResultAndEventPayload_ProjectSameResultAndArtifactIDs(t *testing.T) {
	sessionID := "session-fixture"
	raw, err := json.Marshal(map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	resultArtifact := &factoryapi.FactoryArtifactRef{
		Id:         "artifact-final-1",
		Kind:       factoryapi.FactoryArtifactKindFINALRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
	}
	input := jsresult.SessionResultInput{
		SessionID:      sessionID,
		Status:         factoryapi.FactorySessionStatusFINISHED,
		PrimaryValue:   jsresult.TypedValue{JSON: raw},
		ResultArtifact: resultArtifact,
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-final-1",
			Kind:       "FINAL_RESULT",
			Visibility: "PUBLIC",
		}},
	}
	sessionResult := jsresult.BuildSessionResult(input)
	eventPayload := jsresult.BuildSessionResultUpdatedPayload(input)

	if sessionResult.PrimaryResult == nil {
		t.Fatal("expected primaryResult on session result")
	}
	if eventPayload.ResultSummary == nil {
		t.Fatal("expected resultSummary on event payload")
	}
	if sessionResult.ArtifactIds == nil || len(*sessionResult.ArtifactIds) == 0 || eventPayload.ArtifactIds == nil || len(*eventPayload.ArtifactIds) == 0 {
		t.Fatal("expected result artifact ids")
	}
	if (*sessionResult.ArtifactIds)[0] != (*eventPayload.ArtifactIds)[0] {
		t.Fatalf("artifact ids differ: %q vs %q", (*sessionResult.ArtifactIds)[0], (*eventPayload.ArtifactIds)[0])
	}

	sessionParts := workcontent.PartsFromGenerated(sessionResult.PrimaryResult)
	eventParts := workcontent.PartsFromGenerated(eventPayload.ResultSummary)
	if len(sessionParts) != len(eventParts) {
		t.Fatalf("primary result part counts differ: %d vs %d", len(sessionParts), len(eventParts))
	}
	if string(sessionParts[0].JSON) != string(eventParts[0].JSON) {
		t.Fatalf("primary result JSON differs: %s vs %s", sessionParts[0].JSON, eventParts[0].JSON)
	}

	encodedSession, err := json.Marshal(sessionResult)
	if err != nil {
		t.Fatalf("marshal session result: %v", err)
	}
	if strings.Contains(string(encodedSession), "/tmp/") {
		t.Fatalf("session result leaked host path: %s", encodedSession)
	}
}
