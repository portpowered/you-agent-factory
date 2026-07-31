package support

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestCompareProviderSessionGoldens_WhitespaceOnlyDoesNotFail(t *testing.T) {
	manifest := ProviderSessionGoldenManifest{
		ID:               "harness-compare-whitespace",
		NormalizedFields: []string{"eventId", "recordedAt"},
	}
	expected := ProviderSessionExpectedGoldens{
		ProviderSession: json.RawMessage(`{
  "provider": "harness",
  "status": "completed",
  "eventId": "evt_expected"
}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage(`{"type":"message.completed","eventId":"evt_1","itemId":"item_1"}`),
		},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello"}`),
	}
	observed := ProviderSessionObservedGoldens{
		ProviderSession: json.RawMessage(`{"provider":"harness","status":"completed","eventId":"evt_observed"}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage("{\n  \"type\": \"message.completed\",\n  \"eventId\": \"evt_other\",\n  \"itemId\": \"item_1\"\n}\n"),
		},
		InvocationResult: json.RawMessage("{\n\t\"ok\": true,\n\t\"content\": \"Hello\"\n}\n"),
	}

	if err := CompareProviderSessionGoldens(manifest, expected, observed); err != nil {
		t.Fatalf("CompareProviderSessionGoldens: %v", err)
	}
}

func TestCompareProviderSessionGoldens_DeliberateFieldMismatchFails(t *testing.T) {
	manifest := ProviderSessionGoldenManifest{
		ID:               "harness-compare-mismatch",
		NormalizedFields: []string{"eventId", "recordedAt"},
	}
	expected := ProviderSessionExpectedGoldens{
		ProviderSession:  json.RawMessage(`{"provider":"harness","status":"completed","eventId":"evt_a"}`),
		ResponseEvents:   []json.RawMessage{json.RawMessage(`{"type":"message.completed","itemId":"item_1"}`)},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello"}`),
	}
	observed := ProviderSessionObservedGoldens{
		ProviderSession:  json.RawMessage(`{"provider":"harness","status":"failed","eventId":"evt_b"}`),
		ResponseEvents:   []json.RawMessage{json.RawMessage(`{"type":"message.completed","itemId":"item_1"}`)},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello"}`),
	}

	err := CompareProviderSessionGoldens(manifest, expected, observed)
	if err == nil {
		t.Fatal("CompareProviderSessionGoldens: expected mismatch error")
	}
	var compareErr *ProviderSessionCompareError
	if !errors.As(err, &compareErr) {
		t.Fatalf("error type = %T, want *ProviderSessionCompareError", err)
	}
	if compareErr.CaseID != "harness-compare-mismatch" {
		t.Fatalf("CaseID = %q, want harness-compare-mismatch", compareErr.CaseID)
	}
	if compareErr.Role != "expected-provider-session" {
		t.Fatalf("Role = %q, want expected-provider-session", compareErr.Role)
	}
	if len(compareErr.Differences) == 0 || compareErr.Differences[0].Path != "status" {
		t.Fatalf("differences = %#v, want path status", compareErr.Differences)
	}
	if !strings.Contains(err.Error(), `path "status"`) {
		t.Fatalf("error = %q, want status path diagnostic", err)
	}
}

func TestCompareProviderSessionGoldens_NormalizedFieldDifferencePasses(t *testing.T) {
	manifest := ProviderSessionGoldenManifest{
		ID:               "harness-compare-normalized",
		NormalizedFields: []string{"eventId", "recordedAt", "factorySessionId", "runId"},
	}
	expected := ProviderSessionExpectedGoldens{
		ProviderSession: json.RawMessage(`{
			"provider":"harness",
			"status":"completed",
			"eventId":"evt_expected",
			"recordedAt":"2026-01-01T00:00:00Z",
			"factorySessionId":"fs_expected",
			"runId":"run_expected"
		}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage(`{"type":"message.delta","eventId":"evt_1","recordedAt":"2026-01-01T00:00:01Z","itemId":"item_1","text":"Hello"}`),
			json.RawMessage(`{"type":"message.completed","eventId":"evt_2","recordedAt":"2026-01-01T00:00:02Z","itemId":"item_1","finishReason":"stop"}`),
		},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello","runId":"run_expected"}`),
	}
	observed := ProviderSessionObservedGoldens{
		ProviderSession: json.RawMessage(`{
			"provider":"harness",
			"status":"completed",
			"eventId":"evt_live",
			"recordedAt":"2026-07-23T12:00:00Z",
			"factorySessionId":"fs_live",
			"runId":"run_live"
		}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage(`{"type":"message.delta","eventId":"evt_live_1","recordedAt":"2026-07-23T12:00:01Z","itemId":"item_1","text":"Hello"}`),
			json.RawMessage(`{"type":"message.completed","eventId":"evt_live_2","recordedAt":"2026-07-23T12:00:02Z","itemId":"item_1","finishReason":"stop"}`),
		},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello","runId":"run_live"}`),
	}

	if err := CompareProviderSessionGoldens(manifest, expected, observed); err != nil {
		t.Fatalf("CompareProviderSessionGoldens: %v", err)
	}
}

func TestCompareProviderSessionGoldens_UndeclaredDifferingFieldFails(t *testing.T) {
	manifest := ProviderSessionGoldenManifest{
		ID:               "harness-compare-undeclared",
		NormalizedFields: []string{"eventId"},
	}
	expected := ProviderSessionExpectedGoldens{
		ProviderSession:  json.RawMessage(`{"provider":"harness","status":"completed","eventId":"evt_a","runId":"run_expected"}`),
		ResponseEvents:   []json.RawMessage{json.RawMessage(`{"type":"message.completed","eventId":"evt_1","itemId":"item_1"}`)},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello"}`),
	}
	observed := ProviderSessionObservedGoldens{
		// eventId is normalized (should pass); runId is NOT declared (must fail).
		ProviderSession:  json.RawMessage(`{"provider":"harness","status":"completed","eventId":"evt_b","runId":"run_observed"}`),
		ResponseEvents:   []json.RawMessage{json.RawMessage(`{"type":"message.completed","eventId":"evt_9","itemId":"item_1"}`)},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Hello"}`),
	}

	err := CompareProviderSessionGoldens(manifest, expected, observed)
	if err == nil {
		t.Fatal("CompareProviderSessionGoldens: expected undeclared field mismatch")
	}
	var compareErr *ProviderSessionCompareError
	if !errors.As(err, &compareErr) {
		t.Fatalf("error type = %T, want *ProviderSessionCompareError", err)
	}
	if compareErr.Role != "expected-provider-session" {
		t.Fatalf("Role = %q, want expected-provider-session", compareErr.Role)
	}
	if len(compareErr.Differences) == 0 || compareErr.Differences[0].Path != "runId" {
		t.Fatalf("differences = %#v, want path runId", compareErr.Differences)
	}
}

func TestCompareProviderSessionGoldens_LoadsTrackedFixtureWithoutCallingMapper(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(ProviderSessionFixturePath("harness", "load-smoke")))

	loaded, err := LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}

	// Observed values are supplied by the caller; comparison never synthesizes
	// expected output via a mapper/adapter under test.
	observed := ProviderSessionObservedGoldens{
		ProviderSession:  append(json.RawMessage(nil), loaded.Expected.ProviderSession...),
		ResponseEvents:   append([]json.RawMessage(nil), loaded.Expected.ResponseEvents...),
		InvocationResult: append(json.RawMessage(nil), loaded.Expected.InvocationResult...),
	}
	if err := CompareProviderSessionGoldens(loaded.Manifest, loaded.Expected, observed); err != nil {
		t.Fatalf("CompareProviderSessionGoldens: %v", err)
	}
}

func TestCompareProviderSessionNDJSON_ResponseEventMismatchFails(t *testing.T) {
	err := CompareProviderSessionNDJSON(
		"harness-compare-events",
		"expected-response-events",
		[]string{"eventId"},
		[]json.RawMessage{json.RawMessage(`{"type":"message.completed","eventId":"evt_1","itemId":"item_1"}`)},
		[]json.RawMessage{json.RawMessage(`{"type":"message.completed","eventId":"evt_9","itemId":"item_2"}`)},
	)
	if err == nil {
		t.Fatal("CompareProviderSessionNDJSON: expected mismatch")
	}
	var compareErr *ProviderSessionCompareError
	if !errors.As(err, &compareErr) {
		t.Fatalf("error type = %T, want *ProviderSessionCompareError", err)
	}
	if len(compareErr.Differences) == 0 || compareErr.Differences[0].Path != "[0].itemId" {
		t.Fatalf("differences = %#v, want path [0].itemId", compareErr.Differences)
	}
}
