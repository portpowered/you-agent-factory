package factorydefinitions

import (
	"errors"
	"testing"
)

func TestValidatePortableBundledFileTargetReturnsStructuredPolicyKind(t *testing.T) {
	err := ValidatePortableBundledFileTarget(BundledFileConfig{
		Type:       BundledFileTypeScript,
		TargetPath: "factory/docs/run.sh",
	})
	var validationErr *PortableBundledFileValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want PortableBundledFileValidationError", err)
	}
	if validationErr.Kind != PortableBundledFileValidationTargetRoot {
		t.Fatalf("kind = %q, want %q", validationErr.Kind, PortableBundledFileValidationTargetRoot)
	}
}
