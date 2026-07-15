package progress_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/codex/progress"
)

func TestIsCommand_AcceptsNativeExecutableShapes(t *testing.T) {
	for _, command := range []string{"codex", "codex.exe", `C:\tools\codex.cmd`, "/usr/local/bin/codex"} {
		if !progress.IsCommand(command) {
			t.Fatalf("IsCommand(%q) = false, want true", command)
		}
	}
	if progress.IsCommand("codex-helper.exe") {
		t.Fatal("codex-helper.exe must not be classified as the Codex provider command")
	}
}

func TestProgressStream_NormalizesStructuredLifecycleEvents(t *testing.T) {
	req := workerprocess.CommandRequest{
		Command:         "codex",
		DispatchID:      "dispatch-codex-json-1",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-1"},
		},
	}
	stdout := "{\"event\":\"session.created\",\"session_id\":\"sess-codex-1\"}\n" +
		"{\"type\":\"response.output_text.delta\",\"delta\":\"hello from delta\"}\n" +
		"{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello final\"}]}]}}\n"

	var published []progress.ProgressFragment
	stream := progress.NewProgressStream(req, func(fragment progress.ProgressFragment) {
		published = append(published, fragment)
	})
	stream.Observe(workerprocess.OutputStreamStdout, []byte(stdout))
	stream.Observe(workerprocess.OutputStreamStderr, []byte("planning update\n"))
	stream.Flush()

	if len(published) != 4 {
		t.Fatalf("published fragments = %#v, want 4 normalized fragments", published)
	}

	startedFragment := fragmentByType(published, "STARTED")
	deltaFragment := fragmentByType(published, "TEXT_DELTA")
	finalFragment := fragmentByType(published, "FINAL_TEXT")
	progressFragment := fragmentByType(published, "PROGRESS")

	assertStartedFragment(t, startedFragment, "sess-codex-1", "work-codex-json-1")
	assertResponseFragment(t, deltaFragment, "TEXT_DELTA", "hello from delta")
	assertResponseFragment(t, finalFragment, "FINAL_TEXT", "hello final")
	assertProgressFragment(t, progressFragment, "planning update")
	if finalFragment.ProviderSessionRef == nil || finalFragment.ProviderSessionRef.ID != "sess-codex-1" {
		t.Fatalf("final provider session = %#v, want session propagated", finalFragment.ProviderSessionRef)
	}
}

func TestProgressStream_MapsUnknownAndMalformedEventsToBoundedDiagnostics(t *testing.T) {
	req := workerprocess.CommandRequest{
		Command:         "codex",
		DispatchID:      "dispatch-codex-json-2",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-2"},
		},
	}
	stdout := "{\"event\":\"session.created\",\"session_id\":\"sess-codex-2\"}\n" +
		"{\"type\":\"response.mystery\",\"message\":\"secret-token-123 should never be retained\"}\n" +
		"{\"type\":\"response.progress\"\n" +
		"event: response.output_text.delta\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"delta\":\"hello after malformed frames\"}\n\n"

	var published []progress.ProgressFragment
	stream := progress.NewProgressStream(req, func(fragment progress.ProgressFragment) {
		published = append(published, fragment)
	})
	stream.Observe(workerprocess.OutputStreamStdout, []byte(stdout))
	stream.Flush()

	if len(published) != 5 {
		t.Fatalf("published fragments = %#v, want 5 fragments with bounded unknown diagnostics", published)
	}

	assertStartedFragment(t, &published[0], "sess-codex-2", "work-codex-json-2")
	assertUnknownDiagnostic(t, &published[1], "response.mystery", progress.ProgressDiagnosticUnknownEvent)
	assertUnknownDiagnostic(t, &published[2], "", progress.ProgressDiagnosticMalformedJSON)
	assertUnknownDiagnostic(t, &published[3], "response.output_text.delta", progress.ProgressDiagnosticIncompleteSSE)
	assertResponseFragment(t, &published[4], "TEXT_DELTA", "hello after malformed frames")
	if published[4].ProviderSessionRef == nil || published[4].ProviderSessionRef.ID != "sess-codex-2" {
		t.Fatalf("final provider session = %#v, want session carried across malformed frames", published[4].ProviderSessionRef)
	}
}

func TestProgressStream_MapsFailureCancelAndTruncation(t *testing.T) {
	progressPayload := strings.Repeat("p", progress.ProgressRetainedProgressBytes+73)
	deltaPayload := strings.Repeat("d", progress.ProgressRetainedTextBytes+29)
	finalPayload := strings.Repeat("f", progress.ProgressRetainedTextBytes+41)
	failurePayload := strings.Repeat("e", progress.ProgressRetainedProgressBytes+17)
	cancelPayload := strings.Repeat("c", progress.ProgressRetainedProgressBytes+9)

	req := workerprocess.CommandRequest{
		Command:         "codex",
		DispatchID:      "dispatch-codex-json-3",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-3"},
		},
	}
	stdout := "{\"event\":\"session.created\",\"session_id\":\"sess-codex-3\"}\n" +
		"{\"type\":\"response.progress\",\"message\":\"" + progressPayload + "\"}\n" +
		"{\"type\":\"response.output_text.delta\",\"delta\":\"" + deltaPayload + "\"}\n" +
		"{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"" + finalPayload + "\"}]}]}}\n" +
		"{\"type\":\"response.failed\",\"error\":\"" + failurePayload + "\"}\n" +
		"{\"type\":\"response.canceled\",\"status\":\"" + cancelPayload + "\"}\n"

	var published []progress.ProgressFragment
	stream := progress.NewProgressStream(req, func(fragment progress.ProgressFragment) {
		published = append(published, fragment)
	})
	stream.Observe(workerprocess.OutputStreamStdout, []byte(stdout))
	stream.Flush()

	if len(published) != 6 {
		t.Fatalf("published fragments = %#v, want 6 normalized fragments", published)
	}

	assertStartedFragment(t, &published[0], "sess-codex-3", "work-codex-json-3")
	assertBoundedFragment(t, &published[1], "PROGRESS_FRAGMENT", "PROGRESS", "response.progress", progressPayload, progress.ProgressRetainedProgressBytes)
	assertBoundedFragment(t, &published[2], "RESPONSE_FRAGMENT", "TEXT_DELTA", "response.output_text.delta", deltaPayload, progress.ProgressRetainedTextBytes)
	assertBoundedFragment(t, &published[3], "RESPONSE_FRAGMENT", "FINAL_TEXT", "response.completed", finalPayload, progress.ProgressRetainedTextBytes)
	assertBoundedFragment(t, &published[4], "PROGRESS_FRAGMENT", "FAILED", "response.failed", failurePayload, progress.ProgressRetainedProgressBytes)
	assertBoundedFragment(t, &published[5], "PROGRESS_FRAGMENT", "CANCELED", "response.canceled", cancelPayload, progress.ProgressRetainedProgressBytes)

	if published[5].ProviderSessionRef == nil || published[5].ProviderSessionRef.ID != "sess-codex-3" {
		t.Fatalf("cancel provider session = %#v, want session propagated", published[5].ProviderSessionRef)
	}
}

func fragmentByType(published []progress.ProgressFragment, fragmentType string) *progress.ProgressFragment {
	for i := range published {
		if published[i].Type == fragmentType {
			return &published[i]
		}
	}
	return nil
}

func assertStartedFragment(t *testing.T, fragment *progress.ProgressFragment, sessionID string, workID string) {
	t.Helper()
	if fragment == nil || fragment.ExternalEventType != "session.created" {
		t.Fatalf("started fragment = %#v, want session.created", fragment)
	}
	if fragment.ProviderSessionRef == nil || fragment.ProviderSessionRef.ID != sessionID {
		t.Fatalf("start provider session = %#v, want %s", fragment.ProviderSessionRef, sessionID)
	}
	if got := fragment.Metadata[progress.ProgressMetadataRunnerIDKey]; got != "codex" {
		t.Fatalf("start metadata runner_id = %q, want codex", got)
	}
	if got := fragment.Metadata[progress.ProgressMetadataWorkstationKey]; got != "review" {
		t.Fatalf("start metadata workstation_name = %q, want review", got)
	}
	if got := fragment.Metadata[progress.ProgressMetadataWorkIDKey]; got != workID {
		t.Fatalf("start metadata work_id = %q, want %q", got, workID)
	}
}

func assertResponseFragment(t *testing.T, fragment *progress.ProgressFragment, fragmentType string, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != "RESPONSE_FRAGMENT" || fragment.Type != fragmentType || fragment.Payload != payload {
		t.Fatalf("response fragment = %#v, want %s payload %q", fragment, fragmentType, payload)
	}
}

func assertProgressFragment(t *testing.T, fragment *progress.ProgressFragment, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != "PROGRESS_FRAGMENT" || fragment.Payload != payload {
		t.Fatalf("progress fragment = %#v, want progress payload %q", fragment, payload)
	}
}

func assertUnknownDiagnostic(t *testing.T, fragment *progress.ProgressFragment, externalEventType string, diagnosticClass string) {
	t.Helper()
	if fragment == nil || fragment.Type != "UNKNOWN" || fragment.Kind != "PROGRESS_FRAGMENT" {
		t.Fatalf("unknown fragment = %#v, want UNKNOWN progress fragment", fragment)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("unknown external event = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if fragment.Payload != "codex event omitted" || strings.Contains(fragment.Payload, "secret-token-123") {
		t.Fatalf("unknown payload = %q, want bounded omitted diagnostic", fragment.Payload)
	}
	if got := fragment.Metadata[progress.ProgressMetadataDiagnosticKey]; got != diagnosticClass {
		t.Fatalf("diagnostic_class = %q, want %q", got, diagnosticClass)
	}
	if diagnosticClass == progress.ProgressDiagnosticUnknownEvent && (fragment.Metadata[progress.ProgressMetadataRawSHA256Key] == "" || fragment.Metadata[progress.ProgressMetadataRawBytesKey] == "") {
		t.Fatalf("unknown metadata = %#v, want raw digest metadata", fragment.Metadata)
	}
}

func assertBoundedFragment(
	t *testing.T,
	fragment *progress.ProgressFragment,
	kind string,
	fragmentType string,
	externalEventType string,
	originalPayload string,
	retainedBytes int,
) {
	t.Helper()
	if fragment == nil {
		t.Fatalf("fragment = nil, want %s %s", kind, fragmentType)
	}
	if fragment.Kind != kind || fragment.Type != fragmentType {
		t.Fatalf("fragment kind/type = (%q, %q), want (%q, %q)", fragment.Kind, fragment.Type, kind, fragmentType)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("external event type = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if len([]byte(fragment.Payload)) != retainedBytes {
		t.Fatalf("payload bytes = %d, want %d", len([]byte(fragment.Payload)), retainedBytes)
	}
	if fragment.Payload != originalPayload[:retainedBytes] {
		t.Fatalf("payload retained wrong prefix length: got %d bytes", len([]byte(fragment.Payload)))
	}
	if got := fragment.Metadata[progress.ProgressMetadataTextBytesKey]; got != strconv.Itoa(len([]byte(originalPayload))) {
		t.Fatalf("text_bytes = %q, want %d", got, len([]byte(originalPayload)))
	}
	if got := fragment.Metadata[progress.ProgressMetadataTruncatedKey]; got != "true" {
		t.Fatalf("payload_truncated = %q, want true", got)
	}
}
