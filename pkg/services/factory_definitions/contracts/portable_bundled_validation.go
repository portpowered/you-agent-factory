package factorycontracts

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const (
	portableBundledScriptRoot = "factory/scripts/"
	portableBundledDocRoot    = "factory/docs/"
	portableBundledInputRoot  = "factory/inputs/"
)

// PortableBundledFileValidationKind identifies the Factory Definition field or
// policy rejected by portable bundled-file validation.
type PortableBundledFileValidationKind string

const (
	PortableBundledFileValidationType             PortableBundledFileValidationKind = "type"
	PortableBundledFileValidationTargetPath       PortableBundledFileValidationKind = "target-path"
	PortableBundledFileValidationTargetRoot       PortableBundledFileValidationKind = "target-root"
	PortableBundledFileValidationTargetRootHelper PortableBundledFileValidationKind = "target-root-helper"
)

// PortableBundledFileValidationError preserves structured validation meaning
// while allowing transports and authored-format adapters to choose their own
// finding representation.
type PortableBundledFileValidationError struct {
	Kind    PortableBundledFileValidationKind
	Message string
}

func (e *PortableBundledFileValidationError) Error() string {
	return e.Message
}

func ValidatePortableBundledFileType(bundledFile BundledFileConfig) error {
	if strings.TrimSpace(bundledFile.Type) == "" {
		return portableBundledValidationError(
			PortableBundledFileValidationType,
			"missing required 'type' field",
		)
	}
	switch bundledFile.Type {
	case BundledFileTypeScript, BundledFileTypeDoc, BundledFileTypeInput,
		BundledFileTypeRootHelper:
		return nil
	default:
		return portableBundledValidationError(
			PortableBundledFileValidationType,
			"type %q must be one of %q, %q, %q, or %q",
			bundledFile.Type,
			BundledFileTypeScript,
			BundledFileTypeDoc,
			BundledFileTypeInput,
			BundledFileTypeRootHelper,
		)
	}
}

func ValidatePortableBundledFileTarget(bundledFile BundledFileConfig) error {
	targetPath := strings.TrimSpace(bundledFile.TargetPath)
	if targetPath == "" {
		return portableBundledValidationError(
			PortableBundledFileValidationTargetPath,
			"missing required 'targetPath' field",
		)
	}
	if err := validatePortableBundledFileTargetPath(targetPath); err != nil {
		return portableBundledValidationError(
			PortableBundledFileValidationTargetPath,
			"%s",
			err,
		)
	}
	if bundledFile.Type == BundledFileTypeRootHelper &&
		targetPath != "Makefile" &&
		targetPath != "factory/portable-dependencies.json" {
		return portableBundledValidationError(
			PortableBundledFileValidationTargetRootHelper,
			"targetPath %q must be one of the supported root helper files",
			targetPath,
		)
	}
	expectedRoot := portableBundledFileRootForType(bundledFile.Type)
	if expectedRoot != "" && !strings.HasPrefix(targetPath, expectedRoot) {
		return portableBundledValidationError(
			PortableBundledFileValidationTargetRoot,
			"targetPath %q must stay under %q for %s bundled files",
			targetPath,
			expectedRoot,
			bundledFile.Type,
		)
	}
	if bundledFile.Type == BundledFileTypeInput &&
		!isSupportedPortableBundledInputTarget(targetPath) {
		return portableBundledValidationError(
			PortableBundledFileValidationTargetRoot,
			"targetPath %q must use factory/inputs/<work-type>/<channel>/<file> for INPUT bundled files",
			targetPath,
		)
	}
	return nil
}

func ShouldOmitSupportedPortableBundledInline(bundledFile BundledFileConfig) bool {
	if ValidatePortableBundledFileType(bundledFile) != nil ||
		ValidatePortableBundledFileTarget(bundledFile) != nil {
		return false
	}
	switch bundledFile.Type {
	case BundledFileTypeScript, BundledFileTypeDoc, BundledFileTypeInput:
		return true
	default:
		return false
	}
}

func portableBundledValidationError(
	kind PortableBundledFileValidationKind,
	format string,
	args ...any,
) error {
	return &PortableBundledFileValidationError{
		Kind:    kind,
		Message: fmt.Sprintf(format, args...),
	}
}

func portableBundledFileRootForType(fileType string) string {
	switch fileType {
	case BundledFileTypeScript:
		return portableBundledScriptRoot
	case BundledFileTypeDoc:
		return portableBundledDocRoot
	case BundledFileTypeInput:
		return portableBundledInputRoot
	default:
		return ""
	}
}

func isSupportedPortableBundledInputTarget(targetPath string) bool {
	if !strings.HasPrefix(targetPath, portableBundledInputRoot) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(targetPath, portableBundledInputRoot), "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func validatePortableBundledFileTargetPath(targetPath string) error {
	if filepath.IsAbs(targetPath) || path.IsAbs(targetPath) ||
		filepath.VolumeName(targetPath) != "" {
		return fmt.Errorf("targetPath %q must be factory-relative, not absolute", targetPath)
	}
	if strings.Contains(targetPath, "\\") {
		return fmt.Errorf("targetPath %q must use forward slashes", targetPath)
	}
	cleaned := path.Clean(targetPath)
	if cleaned == "." {
		return fmt.Errorf(
			"targetPath %q must point to a file inside the factory root",
			targetPath,
		)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("targetPath %q cannot escape the factory root", targetPath)
	}
	if cleaned != targetPath {
		return fmt.Errorf(
			"targetPath %q must already be canonical and must not contain '.' or '..' segments",
			targetPath,
		)
	}
	if strings.HasSuffix(targetPath, "/") {
		return fmt.Errorf("targetPath %q must point to a file, not a directory", targetPath)
	}
	return nil
}
