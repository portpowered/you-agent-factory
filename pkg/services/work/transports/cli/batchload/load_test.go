package batchload

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestLoadFromFile_ForwardsToInjectedWorkLoader(t *testing.T) {
	want := work.WorkRequest{RequestID: "request-1", Type: work.WorkRequestTypeFactoryRequestBatch}
	got, err := LoadFromFile(func(path string) (work.WorkRequest, error) {
		if path != "work.json" {
			t.Fatalf("path = %q", path)
		}
		return want, nil
	}, "work.json")
	if err != nil || got.RequestID != want.RequestID {
		t.Fatalf("LoadFromFile = %#v, %v", got, err)
	}
}

func TestLoadFromFile_RequiresAndPreservesWorkLoader(t *testing.T) {
	if _, err := LoadFromFile(nil, "work.json"); err == nil {
		t.Fatal("missing loader error = nil")
	}
	want := errors.New("read work.json: denied")
	_, err := LoadFromFile(func(string) (work.WorkRequest, error) {
		return work.WorkRequest{}, want
	}, "work.json")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
