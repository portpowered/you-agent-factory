package factory_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestBuildLiveSessionResult_ProjectsTerminalReadShape(t *testing.T) {
	t.Parallel()
	timestamp := time.Date(2026, 7, 16, 3, 15, 0, 0, time.UTC)
	checkpointRefs := []interfaces.FactorySessionJavaScriptCheckpointEventRef{{
		ID:        "checkpoint-1",
		Timestamp: &timestamp,
		ArtifactRef: &interfaces.FactoryArtifactRef{
			ID: "artifact-checkpoint-1", Kind: "CHECKPOINT", Visibility: "INTERNAL_CHECKPOINT",
		},
	}}
	resultArtifact := &interfaces.FactoryArtifactRef{
		ID: "artifact-result-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC",
	}

	result := factory.NewSessionResultProjectionOperation().ProjectSessionResults(factory.SessionResultInput{
		SessionID:      " session-js-1 ",
		Status:         interfaces.RuntimeStatusFinished,
		CheckpointRefs: checkpointRefs,
		ResultArtifact: resultArtifact,
	}).Live
	checkpointRefs[0].ID = "mutated"
	resultArtifact.ID = "mutated"

	if result.SessionID != "session-js-1" || result.Status != interfaces.RuntimeStatusFinished {
		t.Fatalf("identity = %#v", result)
	}
	if len(result.CheckpointRefs) != 1 || result.CheckpointRefs[0].ID != "checkpoint-1" {
		t.Fatalf("checkpoint refs = %#v, want detached projection", result.CheckpointRefs)
	}
	if result.ResultArtifactRef == nil || result.ResultArtifactRef.ID != "artifact-result-1" {
		t.Fatalf("result artifact = %#v, want detached projection", result.ResultArtifactRef)
	}
}

func TestValidateTypedValue_AcceptsStructuredJSON(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	result := factory.ValidateTypedValue(factory.TypedValue{JSON: raw})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none", result.Issues)
	}
}

func TestValidateTypedValue_RejectsPromiseFunctionCycleAndBinary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value factory.TypedValue
		code  string
	}{
		{
			name:  "promise",
			value: factory.TypedValue{Unresolved: true},
			code:  factory.CodeUnresolvedPromise,
		},
		{
			name:  "function",
			value: factory.TypedValue{Function: true},
			code:  factory.CodeUnsupportedType,
		},
		{
			name:  "host handle",
			value: factory.TypedValue{HostHandle: "fs.readFile"},
			code:  factory.CodeHostHandle,
		},
		{
			name:  "binary",
			value: factory.TypedValue{RawBinary: []byte{0x01}},
			code:  factory.CodeUnsupportedBinary,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := factory.ValidateTypedValue(tc.value)
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
	result := factory.ValidateTypedValue(factory.TypedValue{HostObject: cycle})
	if !result.HasIssues() {
		t.Fatal("expected cyclic value issue")
	}
	if result.Issues[0].Code != factory.CodeCyclicValue {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, factory.CodeCyclicValue)
	}
}

func TestParseArtifactURI_AcceptsCanonicalForm(t *testing.T) {
	t.Parallel()
	uri := factory.FormatArtifactURI("session-1", "artifact-log-1")
	parsed, issues := factory.ParseArtifactURI(uri)
	if len(issues) > 0 {
		t.Fatalf("parse issues = %#v", issues)
	}
	if parsed.SessionID != "session-1" || parsed.ArtifactID != "artifact-log-1" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseArtifactURI_RejectsMalformedTraversalAndHostPaths(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"file:///tmp/secret",
		"you-artifact://sessions/../artifacts/x",
		"you-artifact://sessions/session-1/artifacts/../escape",
		"you-artifact://sessions//artifacts/x",
	}
	for _, raw := range cases {
		if _, issues := factory.ParseArtifactURI(raw); len(issues) == 0 {
			t.Fatalf("parse(%q) = no issues, want rejection", raw)
		}
	}

	mismatch := factory.ValidateArtifactURIForSession(
		factory.FormatArtifactURI("session-a", "artifact-1"),
		"session-b",
	)
	if len(mismatch) == 0 || mismatch[0].Code != factory.CodeArtifactURISessionMismatch {
		t.Fatalf("session mismatch issues = %#v", mismatch)
	}
}

func TestProjectPrimaryResult_MapsJSONAndArtifactBackedOutputs(t *testing.T) {
	t.Parallel()
	sessionID := "session-fixture"
	raw, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	parts, validation := factory.ProjectPrimaryResult(sessionID, factory.TypedValue{JSON: raw}, nil)
	if validation.HasIssues() {
		t.Fatalf("projection validation = %#v", validation.Issues)
	}
	if len(parts) != 1 || parts[0].Type != work.WorkContentPartTypeJSON {
		t.Fatalf("json parts = %#v", parts)
	}

	imageURI := factory.FormatArtifactURI(sessionID, "artifact-image-1")
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
	imageParts, imageValidation := factory.ProjectPrimaryResult(sessionID, factory.TypedValue{JSON: imageRaw}, imageArtifacts)
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
	t.Parallel()
	sessionID := "session-fixture"
	foreignURI := factory.FormatArtifactURI("session-other", "artifact-image-1")
	raw, err := json.Marshal(foreignURI)
	if err != nil {
		t.Fatalf("marshal foreign uri: %v", err)
	}
	artifacts := []interfaces.FactorySessionArtifactState{{
		ID:         "artifact-image-1",
		Kind:       "IMAGE",
		Visibility: "PUBLIC",
	}}
	parts, validation := factory.ProjectPrimaryResult(sessionID, factory.TypedValue{JSON: raw}, artifacts)
	if !validation.HasIssues() {
		t.Fatalf("parts = %#v, expected cross-session artifact URI rejection", parts)
	}
	if validation.Issues[0].Code != factory.CodeArtifactURISessionMismatch {
		t.Fatalf("issue code = %q, want %q", validation.Issues[0].Code, factory.CodeArtifactURISessionMismatch)
	}

	sessionResult := factory.NewSessionResultProjectionOperation().ProjectSessionResults(factory.SessionResultInput{
		SessionID:    sessionID,
		Status:       interfaces.RuntimeStatusFinished,
		PrimaryValue: factory.TypedValue{JSON: raw},
		Artifacts:    artifacts,
	}).Durable
	if sessionResult.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil for cross-session artifact URI", sessionResult.PrimaryResult)
	}
}

func TestBuildSessionResultAndEventPayload_ProjectSameResultAndArtifactIDs(t *testing.T) {
	t.Parallel()
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
	input := factory.SessionResultInput{
		SessionID:      sessionID,
		Status:         interfaces.RuntimeStatusFinished,
		PrimaryValue:   factory.TypedValue{JSON: raw},
		ResultArtifact: resultArtifact,
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-final-1",
			Kind:       "FINAL_RESULT",
			Visibility: "PUBLIC",
		}},
	}
	projection := factory.NewSessionResultProjectionOperation().ProjectSessionResults(input)
	sessionResult := projection.Durable
	eventPayload := projection.Updated

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
	sessionParts[0].JSON[0] = '['
	sessionResult.ArtifactIDs[0] = "mutated"
	if string(eventParts[0].JSON) != `{"ok":true}` {
		t.Fatalf("event result JSON changed through durable projection: %s", eventParts[0].JSON)
	}
	if eventPayload.ArtifactIDs[0] != "artifact-final-1" {
		t.Fatalf("event artifact id changed through durable projection: %q", eventPayload.ArtifactIDs[0])
	}
	sessionParts[0].JSON[0] = '{'
	sessionResult.ArtifactIDs[0] = "artifact-final-1"

	encodedSession, err := json.Marshal(sessionResult)
	if err != nil {
		t.Fatalf("marshal session result: %v", err)
	}
	if strings.Contains(string(encodedSession), "/tmp/") {
		t.Fatalf("session result leaked host path: %s", encodedSession)
	}
}
