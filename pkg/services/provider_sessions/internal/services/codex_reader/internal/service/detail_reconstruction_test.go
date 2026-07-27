package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func representativeCodexJSONL() string {
	return strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"]}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":"go test ./pkg/api"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
	}, "\n")
}

func TestParseDetailsRepeatedRunsAreStructurallyEquivalent(t *testing.T) {
	session := representativeCodexJSONL()
	first, err := ParseDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("first ParseDetails: %v", err)
	}
	second, err := ParseDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("second ParseDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated parse results differ:\nfirst = %#v\nsecond = %#v", first, second)
	}
}

func TestParseDetailsDetachesNestedPointerFields(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":"go test"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Summary.FunctionCalls) != 1 || len(parsed.Transcript) != 2 {
		t.Fatalf("parsed = %#v, want one call and two transcript entries", parsed)
	}
	if parsed.Transcript[0].Arguments == parsed.Summary.FunctionCalls[0].Arguments {
		t.Fatal("tool_call transcript arguments alias function-call summary arguments")
	}
	*parsed.Transcript[0].Arguments = "mutated"
	if *parsed.Summary.FunctionCalls[0].Arguments == "mutated" {
		t.Fatalf("mutating transcript arguments changed summary arguments = %#v", parsed.Summary.FunctionCalls[0])
	}
}

func TestLoadDetailsReturnsDetachedResultsAcrossInspections(t *testing.T) {
	root, sessionID := writeCodexJSONLFixture(t, representativeCodexJSONL())
	first, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, sessionID)
	if err != nil {
		t.Fatalf("first LoadDetails: %v", err)
	}
	second, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, sessionID)
	if err != nil {
		t.Fatalf("second LoadDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated inspections differ before mutation:\nfirst = %#v\nsecond = %#v", first, second)
	}
	if len(first.Transcript) == 0 || first.Transcript[0].Text == nil {
		t.Fatalf("first detail missing transcript text: %#v", first)
	}
	*first.Transcript[0].Text = "mutated transcript"
	if reflect.DeepEqual(first, second) {
		t.Fatal("mutating first inspection transcript also changed second inspection")
	}
}

func TestParseDetailsAttachesToolOutputToMatchingCallOnly(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"first_tool","arguments":"a"}}`,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"call-2","name":"second_tool","arguments":"b"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"call-2","output":"second-output"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"first-output"}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Summary.FunctionCalls) != 2 {
		t.Fatalf("function calls = %#v, want two calls", parsed.Summary.FunctionCalls)
	}
	first := parsed.Summary.FunctionCalls[0]
	second := parsed.Summary.FunctionCalls[1]
	if stringValue(first.CallID) != "call-1" || stringValue(first.Output) != "first-output" {
		t.Fatalf("first call = %#v, want call-1 with first-output", first)
	}
	if stringValue(second.CallID) != "call-2" || stringValue(second.Output) != "second-output" {
		t.Fatalf("second call = %#v, want call-2 with second-output", second)
	}
}

func TestParseDetailsPreservesLineOrderWhenTimestampsAreAbsent(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"turn_context"}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"first"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"second"}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	want := []providersessions.TranscriptEntryType{
		providersessions.TranscriptUserMessage,
		providersessions.TranscriptAssistantMessage,
	}
	if len(parsed.Transcript) != len(want) {
		t.Fatalf("transcript = %#v, want %d entries", parsed.Transcript, len(want))
	}
	for index, wantType := range want {
		if parsed.Transcript[index].Type != wantType {
			t.Fatalf("transcript[%d].Type = %q, want %q", index, parsed.Transcript[index].Type, wantType)
		}
	}
	if len(parsed.Summary.Turns) != 1 || parsed.Summary.Turns[0].EventCount != 3 {
		t.Fatalf("turns = %#v, want one turn with three counted events", parsed.Summary.Turns)
	}
}

func writeCodexJSONLFixture(t *testing.T, content string) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "session-reconstruct-1"
	dir := filepath.Join(root, "2026", "07", "27")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-"+sessionID+".jsonl"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return root, sessionID
}
