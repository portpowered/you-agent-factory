package requestadmission

import (
	"errors"
	"strings"
	"testing"
)

type fileSourceFunc func(string) ([]byte, error)

func (read fileSourceFunc) ReadFile(path string) ([]byte, error) { return read(path) }

func TestRequestFileLoader_OwnsReadAndCanonicalParsing(t *testing.T) {
	loader := NewRequestFileLoader(fileSourceFunc(func(path string) ([]byte, error) {
		if path != "work.json" {
			t.Fatalf("path = %q", path)
		}
		return []byte(`{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"story","workTypeName":"task"}]}`), nil
	}))
	request, err := loader("work.json")
	if err != nil || request.RequestID != "request-1" || len(request.Works) != 1 {
		t.Fatalf("request = %#v, error = %v", request, err)
	}
}

func TestRequestFileLoader_PreservesReadAndParseContext(t *testing.T) {
	readErr := errors.New("denied")
	_, err := NewRequestFileLoader(fileSourceFunc(func(string) ([]byte, error) { return nil, readErr }))("work.json")
	if !errors.Is(err, readErr) || !strings.Contains(err.Error(), "read work.json") {
		t.Fatalf("read error = %v", err)
	}
	_, err = NewRequestFileLoader(fileSourceFunc(func(string) ([]byte, error) { return []byte(`{bad`), nil }))("work.json")
	if err == nil || !strings.Contains(err.Error(), "parse work.json") {
		t.Fatalf("parse error = %v", err)
	}
}

func TestPayloadFileReader_OwnsReadContext(t *testing.T) {
	want := []byte("payload")
	got, err := NewPayloadFileReader(fileSourceFunc(func(string) ([]byte, error) { return want, nil }))("payload.txt")
	if err != nil || string(got) != string(want) {
		t.Fatalf("payload = %q, error = %v", got, err)
	}
}
