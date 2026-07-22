package baseline

import (
	"strings"
	"testing"
)

func TestNormalizeFixtureText_StripsCRLF(t *testing.T) {
	const want = "alpha\tbeta\n"
	got := NormalizeFixtureText("alpha\tbeta\r\n")
	if got != want {
		t.Fatalf("NormalizeFixtureText() = %q, want %q", got, want)
	}
}

func TestReadFixtureText_NormalizesLineEndings(t *testing.T) {
	got, err := ReadFixtureText(testSourceStore(), "testdata/command_tree.txt")
	if err != nil {
		t.Fatalf("ReadFixtureText: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty fixture text")
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("ReadFixtureText() still contains carriage returns")
	}
}
