package blockingload

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/factoryerrors"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestBlockingFactoryLoadError_ErrorNilAndEmptyTargets(t *testing.T) {
	var nilErr *BlockingFactoryLoadError
	if got := nilErr.Error(); got != factoryerrors.ErrInvalidNamedFactory.Error() {
		t.Fatalf("nil error = %q, want invalid named factory", got)
	}

	empty := &BlockingFactoryLoadError{}
	if !strings.Contains(empty.Error(), "invalid graph references") {
		t.Fatalf("empty targets error = %q, want invalid graph references", empty.Error())
	}
	if strings.Contains(empty.Error(), "blocking validation targets") {
		t.Fatalf("empty targets error = %q, did not want target count summary", empty.Error())
	}
	if !errors.Is(empty, factoryerrors.ErrInvalidNamedFactory) {
		t.Fatal("expected ErrInvalidNamedFactory via Is")
	}
}

func TestNewBlockingFactoryLoadError_EmptyTargetsReturnsNil(t *testing.T) {
	if err := NewBlockingFactoryLoadError(factoryvalidation.Result{}); err != nil {
		t.Fatalf("empty targets = %v, want nil", err)
	}
}

func TestBlockingFactoryLoadOperatorError_UnwrapAndNil(t *testing.T) {
	var nilOp *BlockingFactoryLoadOperatorError
	if nilOp.Unwrap() != nil {
		t.Fatal("nil operator error unwrap should be nil")
	}
	if nilOp.Is(factoryerrors.ErrInvalidNamedFactory) {
		t.Fatal("nil operator error Is should be false")
	}
	if nilOp.Error() != "" {
		t.Fatalf("nil operator error = %q, want empty", nilOp.Error())
	}

	base := NewBlockingFactoryLoadError(factoryvalidation.Result{
		Targets: []factoryvalidation.Target{{Code: "RULE", Message: "broken"}},
	})
	op := &BlockingFactoryLoadOperatorError{FactoryPath: t.TempDir(), Err: base}
	if op.Unwrap() == nil {
		t.Fatal("expected unwrap of base error")
	}
	if !op.Is(factoryerrors.ErrInvalidNamedFactory) {
		t.Fatal("expected operator error to match ErrInvalidNamedFactory")
	}
}

func TestFactoryConfigValidateRecoveryCommandForCLI_EdgeCases(t *testing.T) {
	if got := FactoryConfigValidateRecoveryCommandForCLI("", ""); got != "you factory config validate" {
		t.Fatalf("empty path command = %q", got)
	}

	spaced := filepath.Join("C:", "path with spaces", "factory")
	got := FactoryConfigValidateRecoveryCommandForCLI("you", spaced)
	if !strings.Contains(got, "'") {
		t.Fatalf("spaced path command = %q, want shell quoting", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("command = %q, want exactly one validate invocation", got)
	}
}

func TestQuoteFactoryPathForCLI(t *testing.T) {
	if got := quoteFactoryPathForCLI("simple-path"); got != "simple-path" {
		t.Fatalf("simple path = %q", got)
	}
	if got := quoteFactoryPathForCLI("has space"); got == "has space" {
		t.Fatal("expected quoted path for spaces")
	}
	if got := quoteFactoryPathForCLI(""); got != "." {
		t.Fatalf("empty path = %q, want cleaned dot path", got)
	}
}

func TestFormatBlockingFactoryLoadFindingVariants(t *testing.T) {
	cases := []struct {
		in   blockingFinding
		want string
	}{
		{blockingFinding{rule: "RULE", path: "path", message: "msg"}, "- [RULE] path: msg"},
		{blockingFinding{rule: "RULE", message: "msg"}, "- [RULE] msg"},
		{blockingFinding{path: "path", message: "msg"}, "- path: msg"},
		{blockingFinding{message: "msg"}, "- msg"},
	}
	for _, tc := range cases {
		if got := formatBlockingFactoryLoadFinding(tc.in); got != tc.want {
			t.Fatalf("finding %#v = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWrapBlockingFactoryLoadOperatorErrorBranches(t *testing.T) {
	if got := WrapBlockingFactoryLoadOperatorError("p", nil); got != nil {
		t.Fatalf("nil err = %v, want nil", got)
	}

	ioErr := errors.New("io failure")
	if got := WrapBlockingFactoryLoadOperatorError("p", ioErr); got != ioErr {
		t.Fatalf("non-blocking err = %v, want passthrough", got)
	}

	base := NewBlockingFactoryLoadError(factoryvalidation.Result{
		Targets: []factoryvalidation.Target{{Code: "RULE", Message: "broken"}},
	})
	wrapped := WrapBlockingFactoryLoadOperatorError("/factory", base)
	if _, ok := AsBlockingFactoryLoadOperatorError(wrapped); !ok {
		t.Fatalf("wrapped = %T, want BlockingFactoryLoadOperatorError", wrapped)
	}
	if got := WrapBlockingFactoryLoadOperatorError("/factory", wrapped); got != wrapped {
		t.Fatal("already wrapped error should pass through unchanged")
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	base := NewBlockingFactoryLoadError(factoryvalidation.Result{
		Targets: []factoryvalidation.Target{
			{Code: "RULE", Message: "broken", Subject: factoryvalidation.Subject{ID: "subject"}},
		},
	})

	wrapped := MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(
		base,
		projectRoot,
		globalRoot,
		"@you/goal",
	)
	if _, ok := AsBlockingFactoryLoadOperatorError(wrapped); !ok {
		t.Fatalf("wrapped = %T, want BlockingFactoryLoadOperatorError", wrapped)
	}
	if got := MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(
		wrapped,
		projectRoot,
		globalRoot,
		"@you/goal",
	); got != wrapped {
		t.Fatal("already wrapped error should pass through unchanged")
	}
}

func TestNamedFactoryDirPath(t *testing.T) {
	root := t.TempDir()
	got, err := namedFactoryDirPath(root, "@you/goal")
	if err != nil {
		t.Fatalf("namedFactoryDirPath: %v", err)
	}
	if !strings.Contains(got, "@you") || !strings.Contains(got, "goal") {
		t.Fatalf("path = %q, want named @you/goal factory segment", got)
	}
}

func TestBlockingFactoryLoadFindingsUsesSubjectIDWhenPathEmpty(t *testing.T) {
	err := NewBlockingFactoryLoadError(factoryvalidation.Result{
		Targets: []factoryvalidation.Target{
			{Code: "RULE", Message: "broken", Subject: factoryvalidation.Subject{ID: "subject-id"}},
		},
	})
	findings := blockingFactoryLoadFindings(err)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].path != "subject-id" {
		t.Fatalf("finding path = %q, want subject-id", findings[0].path)
	}
}
