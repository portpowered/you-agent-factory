package api

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type strictJSONTestPayload struct {
	Name string `json:"name"`
}

func TestDecodeStrictJSON_ValidObject(t *testing.T) {
	got, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}`))
	if err != nil {
		t.Fatalf("decodeStrictJSON() error = %v, want nil", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("decoded name = %q, want alpha", got.Name)
	}
}

func TestDecodeStrictJSON_UnknownField(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha","extra":1}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `json: unknown field "extra"`) {
		t.Fatalf("decodeStrictJSON() error = %v, want unknown field message", err)
	}
}

func TestDecodeStrictJSON_MalformedJSON(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want malformed JSON error")
	}
}

func TestDecodeStrictJSON_EmptyBody(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("decodeStrictJSON() error = %v, want io.EOF", err)
	}
}

func TestDecodeStrictJSON_MultiObjectPayload(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}{}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want single-object validation error")
	}
	message, ok := requestFieldValidationMessage(err)
	if !ok {
		t.Fatalf("decodeStrictJSON() error = %T(%v), want requestFieldValidationError", err, err)
	}
	if message != "request payload must contain one JSON object" {
		t.Fatalf("validation message = %q, want single-object payload message", message)
	}
}
