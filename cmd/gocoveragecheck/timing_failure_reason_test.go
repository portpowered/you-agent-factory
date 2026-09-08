package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFunctionalFailureReasonRanksTerminalSignals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "panic beats setup and stack chatter",
			output: "setup progress\ngoroutine 1 [running]:\nruntime/panic.go:123 +0x1\npanic: backend crashed",
			want:   "panic: backend crashed",
		},
		{
			name:   "timeout beats progress",
			output: "waiting for backend\ntest timed out after 30s\ngoroutine 1 [running]:",
			want:   "test timed out after 30s",
		},
		{
			name:   "fail marker is the safe fallback",
			output: "=== RUN   TestBroken\n--- FAIL: TestBroken (0.01s)",
			want:   "--- FAIL: TestBroken (0.01s)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := compactFunctionalFailureReason(tc.output)
			if got != tc.want {
				t.Fatalf("compactFunctionalFailureReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFunctionalFailureReasonUsesStableScopeAndOrdering(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/tests/functional/diagnostics/alpha"
	beta := modulePath + "/tests/functional/diagnostics/beta"
	stream := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: beta, Test: "TestBetaGreen", Output: "successful sibling chatter\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "pass", Package: beta, Test: "TestBetaGreen", Elapsed: 0.01}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: alpha, Test: "TestAlphaSecond", Output: "expected alpha second failure\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: alpha, Test: "TestAlphaSecond", Elapsed: 0.02}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: beta, Test: "TestBetaBroken", Output: "expected beta failure\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: beta, Test: "TestBetaBroken", Elapsed: 0.03}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: alpha, Elapsed: 0.04}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: beta, Elapsed: 0.05}),
	}, "\n")

	detail := renderFunctionalFailureDetail(stream)
	alphaRow := "functional test failure: package=" + alpha + " test=TestAlphaSecond reason=expected alpha second failure"
	betaRow := "functional test failure: package=" + beta + " test=TestBetaBroken reason=expected beta failure"
	if detail != alphaRow+"\n"+betaRow {
		t.Fatalf("failure detail = %q, want stable scoped rows %q", detail, alphaRow+"\n"+betaRow)
	}
	if strings.Contains(detail, "successful sibling chatter") || strings.Contains(detail, "TestBetaGreen") {
		t.Fatalf("failure detail retained successful sibling: %q", detail)
	}

	summary := buildFunctionalTimingSummary(stream, []string{beta, alpha}, 0.05)
	if len(summary.Packages) != 2 || summary.Packages[0].Package != alpha || summary.Packages[1].Package != beta {
		t.Fatalf("package timing order = %+v, want alpha then beta", summary.Packages)
	}
	if len(summary.Tests) != 3 {
		t.Fatalf("test timing rows = %+v, want successful sibling plus two failures", summary.Tests)
	}
	for _, row := range summary.Tests {
		if row.Outcome == timingOutcomeFail && row.Reason == "" {
			t.Fatalf("failed timing row has no reason: %+v", row)
		}
		if row.Test == "TestBetaGreen" && row.Reason != "" {
			t.Fatalf("successful timing row invented a reason: %+v", row)
		}
	}
}

func TestFunctionalFailureReasonUsesNoDiagnosticFallback(t *testing.T) {
	t.Parallel()

	packageName := modulePath + "/tests/functional/diagnostics/empty"
	stream := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: packageName, Test: "TestEmpty", Elapsed: 0.01}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: packageName, Elapsed: 0.02}),
	}, "\n")

	detail := renderFunctionalFailureDetail(stream)
	if !strings.Contains(detail, "reason="+functionalFailureNoDiagnosticReason) {
		t.Fatalf("failure detail = %q, want deterministic fallback", detail)
	}
	summary := buildFunctionalTimingSummary(stream, []string{packageName}, 0.02)
	if summary.Packages[0].Reason != functionalFailureNoDiagnosticReason || summary.Tests[0].Reason != functionalFailureNoDiagnosticReason {
		t.Fatalf("timing fallback reasons = package=%q test=%q, want %q", summary.Packages[0].Reason, summary.Tests[0].Reason, functionalFailureNoDiagnosticReason)
	}
}

func TestFunctionalFailureReasonHandlesMalformedAndIncompleteStreams(t *testing.T) {
	t.Parallel()

	packageName := modulePath + "/tests/functional/diagnostics/partial"
	malformed := strings.Join([]string{
		"{truncated",
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: packageName, Test: "TestBroken", Output: "expected partial failure\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: packageName, Test: "TestBroken", Elapsed: 0.01}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: packageName, Elapsed: 0.02}),
	}, "\n")
	summary := buildFunctionalTimingSummary(malformed, []string{packageName}, 0.02)
	if summary.Complete {
		t.Fatal("malformed stream was reported complete")
	}
	if summary.Packages[0].Reason != "expected partial failure" || summary.Tests[0].Reason != "expected partial failure" {
		t.Fatalf("partial reasons = package=%q test=%q, want preserved safe diagnostic", summary.Packages[0].Reason, summary.Tests[0].Reason)
	}

	missingTerminal := marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: packageName, Test: "TestBroken", Output: "expected incomplete failure\n"})
	partial := buildFunctionalTimingSummary(missingTerminal, []string{packageName}, 0.02)
	if partial.Complete || len(partial.Packages) != 0 {
		t.Fatalf("missing terminal summary = %+v, want incomplete without a false package pass", partial)
	}
}

func TestFunctionalFailureReasonBoundsUTF8AndRedactsSecretCandidates(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("界", 100)
	bounded := compactFunctionalFailureReason(long)
	if !utf8.ValidString(bounded) || len([]byte(bounded)) > maxTimingFailureReasonLength || !strings.HasSuffix(bounded, "...") {
		t.Fatalf("bounded reason = %q, valid=%t bytes=%d, want valid <=%d with ellipsis", bounded, utf8.ValidString(bounded), len([]byte(bounded)), maxTimingFailureReasonLength)
	}

	candidates := []functionalFailureReasonCandidate{
		{reason: "failure token=synthetic-secret-sentinel", rank: functionalFailureReasonAssertion, eventPosition: 3},
		{reason: "<redacted>", rank: functionalFailureReasonFallback, eventPosition: 2},
		{reason: "fatal error: safe terminal signal", rank: functionalFailureReasonTerminal, eventPosition: 1},
	}
	selected := selectFunctionalFailureReasonCandidates(candidates)
	if selected != "fatal error: safe terminal signal" || strings.Contains(selected, "synthetic-secret-sentinel") {
		t.Fatalf("selected reason = %q, want safe terminal signal without raw secret", selected)
	}
	if redacted := compactFunctionalFailureReason("failure: <redacted>"); redacted != "failure: <redacted>" {
		t.Fatalf("redacted reason = %q, want marker preserved", redacted)
	}
}

func TestFunctionalFailureReasonTieBreakIsDeterministic(t *testing.T) {
	t.Parallel()

	candidates := []functionalFailureReasonCandidate{
		{reason: "zeta failure", rank: functionalFailureReasonAssertion, eventPosition: 4, linePosition: 2},
		{reason: "alpha failure", rank: functionalFailureReasonAssertion, eventPosition: 4, linePosition: 2},
		{reason: "alpha failure", rank: functionalFailureReasonAssertion, eventPosition: 4, linePosition: 2},
	}
	for attempt := 0; attempt < 10; attempt++ {
		if got := selectFunctionalFailureReasonCandidates(candidates); got != "alpha failure" {
			t.Fatalf("attempt %d selected %q, want lexical stable tie-break", attempt, got)
		}
	}
}
