package workflowresult_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

func TestValidateTypedValue_AcceptsStructuredJSON(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	result := workflowresult.ValidateTypedValue(workflowresult.TypedValue{JSON: raw})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none", result.Issues)
	}
}

func TestValidateTypedValue_RejectsPromiseFunctionCycleAndBinary(t *testing.T) {
	cases := []struct {
		name  string
		value workflowresult.TypedValue
		code  string
	}{
		{
			name:  "promise",
			value: workflowresult.TypedValue{Unresolved: true},
			code:  workflowresult.CodeUnresolvedPromise,
		},
		{
			name:  "function",
			value: workflowresult.TypedValue{Function: true},
			code:  workflowresult.CodeUnsupportedType,
		},
		{
			name:  "host handle",
			value: workflowresult.TypedValue{HostHandle: "fs.readFile"},
			code:  workflowresult.CodeHostHandle,
		},
		{
			name:  "binary",
			value: workflowresult.TypedValue{RawBinary: []byte{0x01}},
			code:  workflowresult.CodeUnsupportedBinary,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := workflowresult.ValidateTypedValue(tc.value)
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
	result := workflowresult.ValidateTypedValue(workflowresult.TypedValue{HostObject: cycle})
	if !result.HasIssues() {
		t.Fatal("expected cyclic value issue")
	}
	if result.Issues[0].Code != workflowresult.CodeCyclicValue {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, workflowresult.CodeCyclicValue)
	}
}

func TestParseArtifactURI_AcceptsCanonicalForm(t *testing.T) {
	uri := workflowresult.FormatArtifactURI("session-1", "artifact-log-1")
	parsed, issues := workflowresult.ParseArtifactURI(uri)
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
		if _, issues := workflowresult.ParseArtifactURI(raw); len(issues) == 0 {
			t.Fatalf("parse(%q) = no issues, want rejection", raw)
		}
	}

	mismatch := workflowresult.ValidateArtifactURIForSession(
		workflowresult.FormatArtifactURI("session-a", "artifact-1"),
		"session-b",
	)
	if len(mismatch) == 0 || mismatch[0].Code != workflowresult.CodeArtifactURISessionMismatch {
		t.Fatalf("session mismatch issues = %#v", mismatch)
	}
}

func TestProjectPrimaryResult_MapsJSONAndArtifactBackedOutputs(t *testing.T) {
	sessionID := "session-fixture"
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, workflowresult.TypedValue{JSON: raw}, nil)
	if validation.HasIssues() {
		t.Fatalf("projection validation = %#v", validation.Issues)
	}
	if len(parts) != 1 || parts[0].Type != work.WorkContentPartTypeJSON {
		t.Fatalf("json parts = %#v", parts)
	}

	imageURI := workflowresult.FormatArtifactURI(sessionID, "artifact-image-1")
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
	imageParts, imageValidation := workflowresult.ProjectPrimaryResult(sessionID, workflowresult.TypedValue{JSON: imageRaw}, imageArtifacts)
	if imageValidation.HasIssues() {
		t.Fatalf("image validation = %#v", imageValidation.Issues)
	}
	if len(imageParts) != 1 || imageParts[0].Type != work.WorkContentPartTypeImage {
		t.Fatalf("image parts = %#v", imageParts)
	}
	if imageParts[0].URL != imageURI || imageParts[0].ArtifactID != "artifact-image-1" {
		t.Fatalf("image part = %#v", imageParts[0])
	}
}

func TestProjectPrimaryResult_RejectsCrossSessionArtifactURI(t *testing.T) {
	sessionID := "session-fixture"
	foreignURI := workflowresult.FormatArtifactURI("session-other", "artifact-image-1")
	raw, err := json.Marshal(foreignURI)
	if err != nil {
		t.Fatalf("marshal foreign uri: %v", err)
	}
	artifacts := []interfaces.FactorySessionArtifactState{{
		ID:         "artifact-image-1",
		Kind:       "IMAGE",
		Visibility: "PUBLIC",
	}}
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, workflowresult.TypedValue{JSON: raw}, artifacts)
	if !validation.HasIssues() {
		t.Fatalf("parts = %#v, expected cross-session artifact URI rejection", parts)
	}
	if validation.Issues[0].Code != workflowresult.CodeArtifactURISessionMismatch {
		t.Fatalf("issue code = %q, want %q", validation.Issues[0].Code, workflowresult.CodeArtifactURISessionMismatch)
	}

	sessionResult := workflowresult.BuildSessionResult(workflowresult.SessionResultInput{
		SessionID:    sessionID,
		Status:       interfaces.RuntimeStatusFinished,
		PrimaryValue: workflowresult.TypedValue{JSON: raw},
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
	resultArtifact := &interfaces.FactoryArtifactRef{
		ID:         "artifact-final-1",
		Kind:       "FINAL_RESULT",
		Visibility: "PUBLIC",
	}
	input := workflowresult.SessionResultInput{
		SessionID:      sessionID,
		Status:         interfaces.RuntimeStatusFinished,
		PrimaryValue:   workflowresult.TypedValue{JSON: raw},
		ResultArtifact: resultArtifact,
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-final-1",
			Kind:       "FINAL_RESULT",
			Visibility: "PUBLIC",
		}},
	}
	sessionResult := workflowresult.BuildSessionResult(input)
	eventPayload := workflowresult.BuildSessionResultUpdatedPayload(input)

	if len(sessionResult.PrimaryResult) == 0 {
		t.Fatal("expected primaryResult on session result")
	}
	if len(eventPayload.ResultSummary) == 0 {
		t.Fatal("expected resultSummary on event payload")
	}
	if len(sessionResult.ArtifactIDs) == 0 || len(eventPayload.ArtifactIDs) == 0 {
		t.Fatal("expected result artifact ids")
	}
	if sessionResult.ArtifactIDs[0] != eventPayload.ArtifactIDs[0] {
		t.Fatalf("artifact ids differ: %q vs %q", sessionResult.ArtifactIDs[0], eventPayload.ArtifactIDs[0])
	}

	sessionParts := sessionResult.PrimaryResult
	eventParts := eventPayload.ResultSummary
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
