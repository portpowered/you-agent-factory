package compat_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type compatibilityRequest struct {
	Known  string                `json:"known"`
	Nested *compatibilityNested  `json:"nested"`
	Items  []compatibilityNested `json:"items"`
	Opaque json.RawMessage       `json:"opaque"`
}

type compatibilityNested struct {
	Allowed string `json:"allowed"`
}

func TestDecodeReportsSortedNestedUnknownPathsAndPreservesKnownFields(t *testing.T) {
	result, err := httpcompat.DecodeBytes[compatibilityRequest]([]byte(`{
		"unknownZ": "secret-z",
		"known": "known-value",
		"nested": {"unknownNested": true, "allowed": "nested-value"},
		"items": [{"unknownItem": 1, "allowed": "item-value"}],
		"opaque": {"futureOpaque": "not a decoded field"},
		"unknownA": "secret-a"
	}`))
	if err != nil {
		t.Fatalf("DecodeBytes: %v", err)
	}
	if result.Value.Known != "known-value" || result.Value.Nested == nil || result.Value.Nested.Allowed != "nested-value" ||
		len(result.Value.Items) != 1 || result.Value.Items[0].Allowed != "item-value" {
		t.Fatalf("known value = %#v, want decoded known fields", result.Value)
	}
	want := []string{"$.items[0].unknownItem", "$.nested.unknownNested", "$.unknownA", "$.unknownZ"}
	if got := result.Diagnostics.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown paths = %#v, want %#v", got, want)
	}
}

func TestDecodeRetainsStrictMalformedKnownAndTrailingDocumentFailures(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantTrailing bool
	}{
		{name: "malformed", body: `{"known":`, wantTrailing: false},
		{name: "known type", body: `{"known":17}`, wantTrailing: false},
		{name: "trailing document", body: `{"known":"ok"}{}`, wantTrailing: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := httpcompat.DecodeBytes[compatibilityRequest]([]byte(testCase.body))
			if err == nil {
				t.Fatal("DecodeBytes error = nil, want failure")
			}
			var validationErr httpcompat.RequestFieldValidationError
			if testCase.wantTrailing && !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want RequestFieldValidationError", err, err)
			}
			if !testCase.wantTrailing && errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want malformed/known-field failure", err)
			}
		})
	}
}

func TestApplyWarningSetsRFC9110HeaderAndPathOnlyStructuredLog(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	recorder := httptest.NewRecorder()
	httpcompat.ApplyWarning(
		recorder,
		logger,
		"worker_sessions.http",
		"start_worker_session",
		[]string{"$.z", "$.a", "$.z"},
	)

	warning := recorder.Header().Get("Warning")
	wantWarning := `299 - "ignored unknown request fields at $.a, $.z"`
	if warning != wantWarning {
		t.Fatalf("Warning = %q, want %q", warning, wantWarning)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) || fields["boundary"] != "worker_sessions.http" ||
		fields["operation"] != "start_worker_session" {
		t.Fatalf("warning fields = %#v, want compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{"$.a", "$.z"}) {
		t.Fatalf("json_paths = %#v, want sorted unique paths", fields["json_paths"])
	}
	if strings.Contains(entries[0].Message, "secret") {
		t.Fatal("structured warning message contains a payload value")
	}
}

func TestApplyWarningDoesNothingWithoutIgnoredPaths(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	recorder := httptest.NewRecorder()
	httpcompat.ApplyWarning(recorder, zap.New(core), "factory_sessions.http", "open_factory_session", nil)
	if recorder.Header().Get("Warning") != "" {
		t.Fatalf("Warning = %q, want absent", recorder.Header().Get("Warning"))
	}
	if len(logs.All()) != 0 {
		t.Fatalf("warning logs = %#v, want none", logs.All())
	}
}
