package analyzers

import (
	"errors"
	"strings"
	"testing"
)

func TestBaselineGrowthFollowsDetectedTestFileRename(t *testing.T) {
	t.Parallel()
	base := "testsleep-sleep-test|tests/functional/old_test|tests/functional/old/helper_test.go::awaitOutput::sleep::1"
	head := "testsleep-sleep-test|tests/functional/customer_test|tests/functional/customer/old_helper_test.go::awaitOutput::sleep::1"
	renames := "R090\ttests/functional/old/helper_test.go\ttests/functional/customer/old_helper_test.go"
	relocated, err := relocateTestSleepBaseline(base, renames, func(path string) (string, error) {
		if path != "tests/functional/customer/old_helper_test.go" {
			t.Fatalf("unexpected source %q", path)
		}
		return "customer_test", nil
	})
	if err != nil || relocated != head {
		t.Fatalf("relocation = %q, %v", relocated, err)
	}
	if _, err := CompareBaselineGrowth(relocated, head); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{
		strings.Replace(head, "awaitOutput", "awaitDifferentOutput", 1),
		head + "\n" + strings.Replace(head, "sleep::1", "sleep::2", 1),
	} {
		if _, err := CompareBaselineGrowth(relocated, changed); err == nil {
			t.Fatal("relocation admitted new timer debt")
		}
	}
}

func TestBaselineGrowthRelocationRequiresGitEvidenceAndReadableSource(t *testing.T) {
	t.Parallel()
	base := "testsleep-sleep-test|old_test|old/helper_test.go::awaitOutput::sleep::1"
	for _, status := range []string{"A\tnew/helper_test.go", "C090\told/helper_test.go\tnew/helper_test.go"} {
		got, err := relocateTestSleepBaseline(base, status, func(string) (string, error) {
			t.Fatal("source read without rename evidence")
			return "", nil
		})
		if err != nil || got != base {
			t.Fatalf("non-rename changed allowance: %q, %v", got, err)
		}
	}
	_, err := relocateTestSleepBaseline(base, "R090\told/helper_test.go\tnew/helper_test.go", func(string) (string, error) {
		return "", errors.New("unreadable")
	})
	if err == nil {
		t.Fatal("unreadable renamed source passed")
	}
}
