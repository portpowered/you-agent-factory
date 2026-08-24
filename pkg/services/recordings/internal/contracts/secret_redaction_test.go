package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

func TestRedactDeclaredSecretsReplacesNestedClassifiedValues(t *testing.T) {
	const equalLiteral = "declared-secret-equal-literal"
	payload := json.RawMessage(`{
  "name": "visible-name",
  "credentials": {"token": "` + equalLiteral + `", "retry": 2.50},
  "items": [{"token": "` + equalLiteral + `", "label": "first"}, {"token": "ordinary-lookalike"}],
  "repeated": ["` + equalLiteral + `", "` + equalLiteral + `"],
  "sameLiteralUnmarked": "` + equalLiteral + `"
}`)

	result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
		Payload: payload,
		Secrets: []RecordingSecret{
			{JSONPointer: "/credentials/token", Provenance: RecordingSecretProvenanceDeclared},
			{JSONPointer: "/items/0/token", Provenance: RecordingSecretProvenanceDeclared},
			{JSONPointer: "/repeated/0", Provenance: RecordingSecretProvenanceDeclared},
			{JSONPointer: "/repeated/1", Provenance: RecordingSecretProvenanceDeclared},
		},
	})
	if err != nil {
		t.Fatalf("RedactDeclaredSecrets: %v", err)
	}
	if result.RedactedCount != 4 {
		t.Fatalf("RedactedCount = %d, want 4", result.RedactedCount)
	}

	assertRedactedRecordingValue(t, result.Payload, "/credentials/token")
	assertRedactedRecordingValue(t, result.Payload, "/items/0/token")
	assertRedactedRecordingValue(t, result.Payload, "/repeated/0")
	assertRedactedRecordingValue(t, result.Payload, "/repeated/1")

	if got := recordingJSONValue(t, result.Payload, "/sameLiteralUnmarked"); got != equalLiteral {
		t.Fatalf("unmarked equal literal = %#v, want %q", got, equalLiteral)
	}
	if got := recordingJSONValue(t, result.Payload, "/items/1/token"); got != "ordinary-lookalike" {
		t.Fatalf("unmarked lookalike = %#v, want ordinary-lookalike", got)
	}
	if got := recordingJSONValue(t, result.Payload, "/name"); got != "visible-name" {
		t.Fatalf("adjacent name = %#v, want visible-name", got)
	}
	if got := recordingJSONValue(t, result.Payload, "/credentials/retry"); got != json.Number("2.50") {
		t.Fatalf("adjacent number = %#v, want JSON number 2.50", got)
	}
}

func TestRedactDeclaredSecretsSupportsEscapedObjectKeysAndRootValues(t *testing.T) {
	result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
		Payload: json.RawMessage(`{"a/b":{"~key":"value"}}`),
		Secrets: []RecordingSecret{{
			JSONPointer: "/a~1b/~0key",
			Provenance:  RecordingSecretProvenanceDeclared,
		}},
	})
	if err != nil {
		t.Fatalf("RedactDeclaredSecrets escaped key: %v", err)
	}
	assertRedactedRecordingValue(t, result.Payload, "/a~1b/~0key")

	root, err := RedactDeclaredSecrets(RecordingRedactionRequest{
		Payload: json.RawMessage(`"root-secret"`),
		Secrets: []RecordingSecret{{Provenance: RecordingSecretProvenanceDeclared}},
	})
	if err != nil {
		t.Fatalf("RedactDeclaredSecrets root: %v", err)
	}
	assertRedactedRecordingValue(t, root.Payload, "")
}

func TestRedactDeclaredSecretsRejectsInvalidClassificationsWithoutResult(t *testing.T) {
	const secret = "classified-value-must-not-be-returned"
	tests := []struct {
		name   string
		secret RecordingSecret
		want   error
	}{
		{
			name:   "missing provenance",
			secret: RecordingSecret{JSONPointer: "/credential"},
			want:   ErrInvalidRecordingSecretProvenance,
		},
		{
			name:   "unknown provenance",
			secret: RecordingSecret{JSONPointer: "/credential", Provenance: "HEURISTIC"},
			want:   ErrInvalidRecordingSecretProvenance,
		},
		{
			name:   "missing classified location",
			secret: RecordingSecret{JSONPointer: "/missing", Provenance: RecordingSecretProvenanceDeclared},
			want:   ErrRecordingSecretPathNotFound,
		},
		{
			name:   "invalid pointer",
			secret: RecordingSecret{JSONPointer: "credential", Provenance: RecordingSecretProvenanceDeclared},
			want:   ErrInvalidRecordingSecretPath,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
				Payload: json.RawMessage(`{"credential":"` + secret + `"}`),
				Secrets: []RecordingSecret{test.secret},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, test.want)
			}
			if result.Payload != nil || result.RedactedCount != 0 {
				t.Fatalf("failed result = %#v, want no destination payload", result)
			}
		})
	}
}

func TestRedactDeclaredSecretsRejectsDuplicateClassifications(t *testing.T) {
	result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
		Payload: json.RawMessage(`{"credential":"value"}`),
		Secrets: []RecordingSecret{
			{JSONPointer: "/credential", Provenance: RecordingSecretProvenanceDeclared},
			{JSONPointer: "/credential", Provenance: RecordingSecretProvenanceDeclared},
		},
	})
	if !errors.Is(err, ErrDuplicateRecordingSecretPath) {
		t.Fatalf("error = %v, want duplicate path", err)
	}
	if result.Payload != nil {
		t.Fatalf("failed result payload = %s, want nil", result.Payload)
	}
}

func TestRecordingRedactedValueSerializationIsClosedAndTyped(t *testing.T) {
	value := RecordingRedactedValue{
		Redacted:   true,
		Provenance: RecordingSecretProvenanceDeclared,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode marker: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("marker fields = %#v, want exactly redacted and provenance", fields)
	}
	var redacted bool
	if err := json.Unmarshal(fields["redacted"], &redacted); err != nil || !redacted {
		t.Fatalf("redacted field = %s, want true", fields["redacted"])
	}
	var provenance RecordingSecretProvenance
	if err := json.Unmarshal(fields["provenance"], &provenance); err != nil || provenance != RecordingSecretProvenanceDeclared {
		t.Fatalf("provenance field = %s, want declared-secret", fields["provenance"])
	}
	if _, ok := fields["value"]; ok {
		t.Fatal("marker contains an original value field")
	}

	if _, err := json.Marshal(RecordingRedactedValue{Provenance: RecordingSecretProvenanceDeclared}); !errors.Is(err, ErrInvalidRecordingSecretProvenance) {
		t.Fatalf("invalid marker marshal error = %v, want invalid provenance", err)
	}
}

func TestRedactDeclaredSecretsWithoutClassificationsReturnsDetachedBytes(t *testing.T) {
	payload := json.RawMessage("  {\"value\":\"unchanged\"}  ")
	result, err := RedactDeclaredSecrets(RecordingRedactionRequest{Payload: payload})
	if err != nil {
		t.Fatalf("RedactDeclaredSecrets: %v", err)
	}
	if !reflect.DeepEqual(result.Payload, payload) {
		t.Fatalf("payload = %q, want byte-preserved payload %q", result.Payload, payload)
	}
	result.Payload[0] = 'x'
	if payload[0] != ' ' {
		t.Fatal("result payload aliases request payload")
	}
}

func assertRedactedRecordingValue(t *testing.T, payload json.RawMessage, pointer string) {
	t.Helper()
	raw := recordingJSONRawAt(t, payload, pointer)
	var marker RecordingRedactedValue
	if err := json.Unmarshal(raw, &marker); err != nil {
		t.Fatalf("decode redacted value at %q: %v", pointer, err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("marker at %q: %v", pointer, err)
	}
}

func recordingJSONValue(t *testing.T, payload json.RawMessage, pointer string) any {
	t.Helper()
	var value any
	decoder := json.NewDecoder(bytesReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for _, segment := range mustRecordingJSONPointer(t, pointer) {
		switch typed := value.(type) {
		case map[string]any:
			value = typed[segment]
		case []any:
			index := mustRecordingJSONArrayIndex(t, segment)
			value = typed[index]
		default:
			t.Fatalf("pointer %q crossed non-container value", pointer)
		}
	}
	return value
}

func recordingJSONRawAt(t *testing.T, payload json.RawMessage, pointer string) json.RawMessage {
	t.Helper()
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for _, segment := range mustRecordingJSONPointer(t, pointer) {
		switch typed := value.(type) {
		case map[string]any:
			value = typed[segment]
		case []any:
			index := mustRecordingJSONArrayIndex(t, segment)
			value = typed[index]
		default:
			t.Fatalf("pointer %q crossed non-container value", pointer)
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value at %q: %v", pointer, err)
	}
	return raw
}

func mustRecordingJSONPointer(t *testing.T, pointer string) []string {
	t.Helper()
	path, err := parseRecordingJSONPointer(pointer)
	if err != nil {
		t.Fatalf("parse pointer %q: %v", pointer, err)
	}
	return path
}

func mustRecordingJSONArrayIndex(t *testing.T, segment string) int {
	t.Helper()
	index, err := strconv.Atoi(segment)
	if err != nil {
		t.Fatalf("array segment %q: %v", segment, err)
	}
	return index
}

func bytesReader(payload []byte) *bytes.Reader {
	return bytes.NewReader(payload)
}
