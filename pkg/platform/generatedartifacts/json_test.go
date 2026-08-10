package generatedartifacts

import (
	"strings"
	"testing"
)

func TestDecodeJSONRejectingDuplicateKeysDecodesNestedValues(t *testing.T) {
	var target struct {
		Items []map[string]string `json:"items"`
	}
	if err := DecodeJSONRejectingDuplicateKeys([]byte(`{"items":[{"name":"first"},{"name":"second"}]}`), &target); err != nil {
		t.Fatalf("DecodeJSONRejectingDuplicateKeys() error = %v", err)
	}
	if len(target.Items) != 2 || target.Items[1]["name"] != "second" {
		t.Fatalf("decoded target = %#v, want two nested items", target)
	}
}

func TestDecodeJSONRejectingDuplicateKeysRejectsMalformedAndTrailingInput(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "duplicate nested key", payload: `{"items":{"name":"first","name":"second"}}`, want: `duplicate object key "name"`},
		{name: "trailing value", payload: `{"ok":true}{"again":true}`, want: "unexpected trailing JSON value"},
		{name: "invalid trailing token", payload: `{"ok":true}oops`, want: "decode trailing JSON"},
		{name: "truncated object value", payload: `{"ok":`, want: "EOF"},
		{name: "truncated object key", payload: `{"ok":true,`, want: "EOF"},
		{name: "truncated array value", payload: `{"items":[`, want: "EOF"},
		{name: "wrong object delimiter", payload: `{"ok":true]`, want: "invalid character"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := DecodeJSONRejectingDuplicateKeys([]byte(test.payload), &map[string]any{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDecodeJSONRejectingDuplicateKeysAcceptsScalarAndEmptyContainers(t *testing.T) {
	for _, payload := range []string{`true`, `[]`, `{}`} {
		var target any
		if err := DecodeJSONRejectingDuplicateKeys([]byte(payload), &target); err != nil {
			t.Fatalf("DecodeJSONRejectingDuplicateKeys(%s) error = %v", payload, err)
		}
	}
}
