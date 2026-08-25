package internal

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRecordingSecretsForEventPayloadKeepsOnlyExistingDeclaredPaths(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"factory":{"credential":"secret","items":[{"token":"one"}],"a/b":{"~key":true}}}`)
	secrets := recordingSecretsForEventPayload(payload, []string{
		"",
		"/credential",
		"/items/0/token",
		"/a~1b/~0key",
		"/missing",
		"/items/not-an-index",
	})

	wantPointers := []string{"/factory", "/factory/credential", "/factory/items/0/token", "/factory/a~1b/~0key"}
	if len(secrets) != len(wantPointers) {
		t.Fatalf("recordingSecretsForEventPayload returned %d secrets, want %d: %#v", len(secrets), len(wantPointers), secrets)
	}
	for index, wantPointer := range wantPointers {
		if secrets[index].JSONPointer != wantPointer {
			t.Errorf("secret[%d].JSONPointer = %q, want %q", index, secrets[index].JSONPointer, wantPointer)
		}
		if secrets[index].Provenance != recordings.RecordingSecretProvenanceDeclared {
			t.Errorf("secret[%d].Provenance = %q, want declared", index, secrets[index].Provenance)
		}
	}
}

func TestRecordingSecretsForEventPayloadRejectsUnavailableInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  string
		pointers []string
	}{
		{name: "empty payload", payload: "", pointers: []string{"/credential"}},
		{name: "empty pointers", payload: `{"factory":{"credential":"secret"}}`},
		{name: "malformed input", payload: `{"factory":`, pointers: []string{"/credential"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := recordingSecretsForEventPayload([]byte(test.payload), test.pointers); got != nil {
				t.Fatalf("recordingSecretsForEventPayload(%s) = %#v, want nil", test.name, got)
			}
		})
	}
}

func TestRecordingJSONPointerExistsHandlesObjectsArraysAndEscapes(t *testing.T) {
	t.Parallel()

	document := map[string]any{
		"factory": map[string]any{
			"items":  []any{"first", map[string]any{"value": true}},
			"a/b":    map[string]any{"~key": true},
			"scalar": "leaf",
		},
	}
	tests := map[string]struct {
		pointer string
		want    bool
	}{
		"empty pointer":            {pointer: "", want: true},
		"missing leading slash":    {pointer: "factory", want: false},
		"object value":             {pointer: "/factory/items", want: true},
		"array element":            {pointer: "/factory/items/0", want: true},
		"nested array object":      {pointer: "/factory/items/1/value", want: true},
		"escaped object keys":      {pointer: "/factory/a~1b/~0key", want: true},
		"missing object key":       {pointer: "/factory/missing", want: false},
		"invalid array index":      {pointer: "/factory/items/nope", want: false},
		"negative array index":     {pointer: "/factory/items/-1", want: false},
		"out of range array index": {pointer: "/factory/items/2", want: false},
		"descend into scalar":      {pointer: "/factory/scalar/child", want: false},
		"truncated escape":         {pointer: "/factory/a~", want: false},
		"unknown escape":           {pointer: "/factory/a~2", want: false},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := recordingJSONPointerExists(document, test.pointer); got != test.want {
				t.Errorf("recordingJSONPointerExists(%q) = %v, want %v", test.pointer, got, test.want)
			}
		})
	}
}

func TestDecodeRecordingJSONPointerTokenDecodesRFC6901Escapes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		token string
		want  string
		ok    bool
	}{
		"plain":           {token: "plain", want: "plain", ok: true},
		"tilde and slash": {token: "~0~1", want: "~/", ok: true},
		"mixed":           {token: "a~1b~0c", want: "a/b~c", ok: true},
		"truncated":       {token: "~", ok: false},
		"unknown escape":  {token: "~2", ok: false},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := decodeRecordingJSONPointerToken(test.token)
			if got != test.want || ok != test.ok {
				t.Errorf("decodeRecordingJSONPointerToken(%q) = (%q, %v), want (%q, %v)", test.token, got, ok, test.want, test.ok)
			}
		})
	}
}
