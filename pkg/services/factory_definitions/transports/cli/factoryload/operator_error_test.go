package factoryload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestBlockingFactoryLoadError_ErrorNilAndEmptyTargets(t *testing.T) {
	var nilErr *factorydefinitions.BlockingFactoryLoadError
	if got := nilErr.Error(); got != factorydefinitions.ErrInvalidNamedFactory.Error() {
		t.Fatalf("nil error = %q, want invalid named factory", got)
	}

	empty := &factorydefinitions.BlockingFactoryLoadError{}
	if !strings.Contains(empty.Error(), "invalid graph references") {
		t.Fatalf("empty targets error = %q, want invalid graph references", empty.Error())
	}
	if strings.Contains(empty.Error(), "blocking validation targets") {
		t.Fatalf("empty targets error = %q, did not want target count summary", empty.Error())
	}
	if !errors.Is(empty, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatal("expected ErrInvalidNamedFactory via Is")
	}
}

func TestFormatOperatorDiagnosticPreservesWrappedSourceContext(t *testing.T) {
	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{
			Code: "RULE", Message: "missing worker",
		}},
	})
	sourceErr := fmt.Errorf(
		"validate factory config /factory/factory.yaml (YAML): %w",
		base,
	)

	got := FormatOperatorDiagnostic("/factory", sourceErr)
	if !strings.Contains(
		got,
		"validate factory config /factory/factory.yaml (YAML)",
	) {
		t.Fatalf("error = %q, want source path and format context", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	if !errors.Is(&OperatorError{FactoryPath: "/factory", Err: sourceErr}, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatal("wrapped source error should retain ErrInvalidNamedFactory")
	}
}

func TestNewBlockingFactoryLoadError_EmptyTargetsReturnsNil(t *testing.T) {
	if err := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{}); err != nil {
		t.Fatalf("empty targets = %v, want nil", err)
	}
}

func TestBlockingFactoryLoadOperatorError_UnwrapAndNil(t *testing.T) {
	var nilOp *OperatorError
	if nilOp.Unwrap() != nil {
		t.Fatal("nil operator error unwrap should be nil")
	}
	if nilOp.Is(factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatal("nil operator error Is should be false")
	}
	if nilOp.Error() != "" {
		t.Fatalf("nil operator error = %q, want empty", nilOp.Error())
	}

	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	op := &OperatorError{FactoryPath: t.TempDir(), Err: base}
	if op.Unwrap() == nil {
		t.Fatal("expected unwrap of base error")
	}
	if !op.Is(factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatal("expected operator error to match ErrInvalidNamedFactory")
	}
}

func TestFactoryConfigValidateRecoveryCommandForCLI_EdgeCases(t *testing.T) {
	if got := ConfigValidateRecoveryCommandForCLI("", ""); got != "you factory config validate" {
		t.Fatalf("empty path command = %q", got)
	}

	spaced := filepath.Join("C:", "path with spaces", "factory")
	got := ConfigValidateRecoveryCommandForCLI("you", spaced)
	if !strings.Contains(got, "'") {
		t.Fatalf("spaced path command = %q, want shell quoting", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("command = %q, want exactly one validate invocation", got)
	}
}

func TestQuoteFactoryPathForCLI(t *testing.T) {
	if got := quoteFactoryPath("simple-path"); got != "simple-path" {
		t.Fatalf("simple path = %q", got)
	}
	if got := quoteFactoryPath("has space"); got == "has space" {
		t.Fatal("expected quoted path for spaces")
	}
	if got := quoteFactoryPath(""); got != "." {
		t.Fatalf("empty path = %q, want cleaned dot path", got)
	}
}

func TestFormatBlockingFactoryLoadFindingVariants(t *testing.T) {
	cases := []struct {
		in   finding
		want string
	}{
		{finding{rule: "RULE", path: "path", message: "msg"}, "- [RULE] path: msg"},
		{finding{rule: "RULE", message: "msg"}, "- [RULE] msg"},
		{finding{path: "path", message: "msg"}, "- path: msg"},
		{finding{message: "msg"}, "- msg"},
	}
	for _, tc := range cases {
		if got := formatFinding(tc.in); got != tc.want {
			t.Fatalf("finding %#v = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWrapBlockingFactoryLoadOperatorErrorBranches(t *testing.T) {
	if got := WrapOperatorError("p", nil); got != nil {
		t.Fatalf("nil err = %v, want nil", got)
	}

	ioErr := errors.New("io failure")
	if got := WrapOperatorError("p", ioErr); got != ioErr {
		t.Fatalf("non-blocking err = %v, want passthrough", got)
	}

	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	wrapped := WrapOperatorError("/factory", base)
	if _, ok := AsOperatorError(wrapped); !ok {
		t.Fatalf("wrapped = %T, want BlockingFactoryLoadOperatorError", wrapped)
	}
	if got := WrapOperatorError("/factory", wrapped); got != wrapped {
		t.Fatal("already wrapped error should pass through unchanged")
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{
			{Code: "RULE", Message: "broken", Subject: factorydefinitions.ValidationSubject{ID: "subject"}},
		},
	})

	wrapped := MaybeFormatOperatorErrorForNamedFactory(
		base,
		factorydefinitions.NamedFactoryCandidatePaths{
			Project: filepath.Join(projectRoot, "@you", "goal"),
			Global:  filepath.Join(globalRoot, "@you", "goal"),
		},
	)
	if _, ok := AsOperatorError(wrapped); !ok {
		t.Fatalf("wrapped = %T, want BlockingFactoryLoadOperatorError", wrapped)
	}
	if got := MaybeFormatOperatorErrorForNamedFactory(
		wrapped,
		factorydefinitions.NamedFactoryCandidatePaths{},
	); got != wrapped {
		t.Fatal("already wrapped error should pass through unchanged")
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorErrorUsesDetachedCandidatePrecedence(t *testing.T) {
	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	candidates := factorydefinitions.NamedFactoryCandidatePaths{
		Project: "/project/factory/alpha",
		Global:  "/global/factories/alpha",
	}

	wrapped, ok := AsOperatorError(MaybeFormatOperatorErrorForNamedFactory(base, candidates))
	if !ok {
		t.Fatal("formatted error is not an OperatorError")
	}
	if wrapped.FactoryPath != candidates.Project {
		t.Fatalf("FactoryPath = %q, want project-precedence candidate %q", wrapped.FactoryPath, candidates.Project)
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorErrorUsesDetachedGlobalFallback(t *testing.T) {
	base := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	candidates := factorydefinitions.NamedFactoryCandidatePaths{Global: "/global/factories/alpha"}

	wrapped, ok := AsOperatorError(MaybeFormatOperatorErrorForNamedFactory(base, candidates))
	if !ok {
		t.Fatal("formatted error is not an OperatorError")
	}
	if wrapped.FactoryPath != candidates.Global {
		t.Fatalf("FactoryPath = %q, want global fallback candidate %q", wrapped.FactoryPath, candidates.Global)
	}
}

func TestNamedFactoryDirPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "@you", "goal")
	base := &factorydefinitions.BlockingFactoryLoadError{
		Targets: []factorydefinitions.ValidationTarget{{Code: "RULE", Message: "broken"}},
	}
	wrapped, ok := AsOperatorError(WrapOperatorError(want, base))
	if !ok {
		t.Fatal("wrapped error is not an OperatorError")
	}
	if wrapped.FactoryPath != want {
		t.Fatalf("path = %q, want detached named Factory path %q", wrapped.FactoryPath, want)
	}
}

func TestBlockingFactoryLoadFindingsUsesSubjectIDWhenPathEmpty(t *testing.T) {
	err := factorydefinitions.NewBlockingFactoryLoadError(factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{
			{Code: "RULE", Message: "broken", Subject: factorydefinitions.ValidationSubject{ID: "subject-id"}},
		},
	})
	findings := blockingFindings(err)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].path != "subject-id" {
		t.Fatalf("finding path = %q, want subject-id", findings[0].path)
	}
}
