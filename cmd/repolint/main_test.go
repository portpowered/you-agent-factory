package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineMode(t *testing.T) {
	var out bytes.Buffer
	if handled, err := baselineMode([]string{"-flags"}, &out); handled || err != nil {
		t.Fatalf("normal dispatch: %v %v", handled, err)
	}
	for _, args := range [][]string{{"-baseline-growth"}, {"-baseline-growth="}, {"-baseline-growth=missing"}, {"-baseline-growth=missing", "extra"}} {
		if handled, err := baselineMode(args, &out); !handled || err == nil {
			t.Fatalf("invalid args %v: %v %v", args, handled, err)
		}
	}
	path := filepath.Join(t.TempDir(), "base")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if handled, err := baselineMode([]string{"-baseline-growth=" + path}, &out); !handled || err != nil || !strings.Contains(out.String(), "accepted") {
		t.Fatalf("comparison: %v %v %s", handled, err, out.String())
	}
	if err := os.WriteFile(path, []byte("application-graph-test|other|target"), 0600); err != nil {
		t.Fatal(err)
	}
	if handled, err := baselineMode([]string{"-baseline-growth=" + path}, &out); !handled || err == nil || !strings.Contains(err.Error(), "established") {
		t.Fatalf("growth: %v %v", handled, err)
	}

}
