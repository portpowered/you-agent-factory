package provider

import "testing"

func TestContainsStopToken_Found(t *testing.T) {
	output := "Some output text\n<promise>COMPLETE</promise>\nMore text"
	if !ContainsStopToken(output, "<promise>COMPLETE</promise>") {
		t.Error("expected stop token to be found")
	}
}

func TestContainsStopToken_NotFound(t *testing.T) {
	output := "Some output text without the token"
	if ContainsStopToken(output, "<promise>COMPLETE</promise>") {
		t.Error("expected stop token NOT to be found")
	}
}

func TestContainsStopToken_EmptyToken(t *testing.T) {
	if ContainsStopToken("any output", "") {
		t.Error("empty stop token should never match")
	}
}

func TestContainsStopToken_EmptyOutput(t *testing.T) {
	if ContainsStopToken("", "COMPLETE") {
		t.Error("empty output should never match")
	}
}

func TestContainsStopToken_CaseSensitive(t *testing.T) {
	output := "The task is complete"
	if ContainsStopToken(output, "COMPLETE") {
		t.Error("stop token check should be case-sensitive")
	}
}

func TestContainsStopToken_PartialMatch(t *testing.T) {
	output := "This is COMPLETED now"
	if !ContainsStopToken(output, "COMPLETE") {
		t.Error("substring match should work — COMPLETE is in COMPLETED")
	}
}
