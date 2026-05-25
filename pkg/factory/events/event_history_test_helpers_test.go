package events

import "testing"

func assertJSONField(t *testing.T, object map[string]any, field string, want any) {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON field %q in %#v", field, object)
	}
	if got != want {
		t.Fatalf("JSON field %q = %#v, want %#v", field, got, want)
	}
}

func assertJSONObject(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON object field %q in %#v", field, object)
	}
	value, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("JSON field %q = %#v, want object", field, got)
	}
	return value
}
