// Package persist is the canonical entrypoint for persisting factory definitions
// to disk: named-factory create/replace, split-layout replace, and
// current-factory pointer read/write.
package persist

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/config"
)

// Re-export stable persist error values for callers that import pkg/config/persist only.
var (
	ErrInvalidNamedFactory       = config.ErrInvalidNamedFactory
	ErrNamedFactoryAlreadyExists = config.ErrNamedFactoryAlreadyExists
)

// NamedFactoryPersistResult reports the staged named-factory directory together
// with any bundled files that were overwritten while restoring inline portable
// content into the thin persisted layout.
type NamedFactoryPersistResult = config.NamedFactoryPersistResult

// PortableBundledFileReplacement reports a bundled file path replaced on disk.
type PortableBundledFileReplacement = config.PortableBundledFileReplacement

// PersistNamedFactory materializes a compact canonical factory payload under a
// named subdirectory rooted at rootDir.
func PersistNamedFactory(rootDir, name string, canonicalFactoryJSON []byte) (string, error) {
	return config.PersistNamedFactory(rootDir, name, canonicalFactoryJSON)
}

// PersistNamedFactoryWithReport materializes a compact canonical factory payload
// under a named subdirectory rooted at rootDir and reports portable bundled
// file replacements.
func PersistNamedFactoryWithReport(rootDir, name string, canonicalFactoryJSON []byte) (*NamedFactoryPersistResult, error) {
	return config.PersistNamedFactoryWithReport(rootDir, name, canonicalFactoryJSON)
}

// ReplaceNamedFactory materializes a compact canonical factory payload and
// atomically replaces an existing named factory directory rooted at rootDir.
func ReplaceNamedFactory(rootDir, name string, canonicalFactoryJSON []byte) (string, error) {
	return config.ReplaceNamedFactory(rootDir, name, canonicalFactoryJSON)
}

// ReplaceNamedFactoryWithReport is the replacement equivalent of
// PersistNamedFactoryWithReport.
func ReplaceNamedFactoryWithReport(rootDir, name string, canonicalFactoryJSON []byte) (*NamedFactoryPersistResult, error) {
	return config.ReplaceNamedFactoryWithReport(rootDir, name, canonicalFactoryJSON)
}

// FactoryLayoutReplaceOptions configures ReplaceFactoryLayoutAtDir.
type FactoryLayoutReplaceOptions = config.FactoryLayoutReplaceOptions

// DefaultFactoryLayoutReplaceOptions returns persist-from-save layout options.
func DefaultFactoryLayoutReplaceOptions(targetDir string) FactoryLayoutReplaceOptions {
	return config.DefaultFactoryLayoutReplaceOptions(targetDir)
}

// ReplaceFactoryLayoutAtDir atomically replaces targetDir from payload using the
// shared split-layout persist pipeline.
func ReplaceFactoryLayoutAtDir(targetDir string, payload []byte, opts FactoryLayoutReplaceOptions) (restore func(), err error) {
	return config.ReplaceFactoryLayoutAtDir(targetDir, payload, opts)
}

// ReplaceFactorySplitLayout atomically replaces an existing factory directory
// with a split-layout materialization of canonicalFactoryJSON.
func ReplaceFactorySplitLayout(targetDir string, canonicalFactoryJSON []byte) (*config.FactorySplitLayoutReplaceResult, error) {
	return config.ReplaceFactorySplitLayout(targetDir, canonicalFactoryJSON)
}

// ReadCurrentFactoryPointer returns the current named factory selected for the
// root directory's named-factory layout.
func ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return config.ReadCurrentFactoryPointer(rootDir)
}

// WriteCurrentFactoryPointer persists the selected named factory for later
// restart-time resolution.
func WriteCurrentFactoryPointer(rootDir, name string) error {
	return config.WriteCurrentFactoryPointer(rootDir, name)
}

// ValidateNamedFactoryName applies the canonical safe directory-segment rules
// used by the named-factory on-disk layout.
func ValidateNamedFactoryName(name string) error {
	return config.ValidateNamedFactoryName(name)
}

// IsInvalidNamedFactory reports whether err wraps ErrInvalidNamedFactory.
func IsInvalidNamedFactory(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactory)
}

// IsNamedFactoryAlreadyExists reports whether err wraps ErrNamedFactoryAlreadyExists.
func IsNamedFactoryAlreadyExists(err error) bool {
	return errors.Is(err, ErrNamedFactoryAlreadyExists)
}
