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
	ErrInvalidNamedFactoryName   = config.ErrInvalidNamedFactoryName
	ErrNamedFactoryAlreadyExists = config.ErrNamedFactoryAlreadyExists
	ErrNamedFactoryNotFound      = config.ErrNamedFactoryNotFound
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

// PersistNamedFactoryWithPrepared materializes a named factory from a
// pre-normalized payload shared by validation and split layout writes.
func PersistNamedFactoryWithPrepared(rootDir, name string, prepared *PreparedFactoryLayoutPayload) (string, error) {
	return config.PersistNamedFactoryWithPrepared(rootDir, name, prepared)
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

// DefaultFactoryLayoutReplaceOptions returns split-layout replace options.
func DefaultFactoryLayoutReplaceOptions(targetDir string) FactoryLayoutReplaceOptions {
	return config.DefaultFactoryLayoutReplaceOptions(targetDir)
}

// ReplaceFactoryLayoutAtDir atomically replaces targetDir from payload using the
// shared split-layout persist pipeline.
func ReplaceFactoryLayoutAtDir(targetDir string, payload []byte, opts FactoryLayoutReplaceOptions) (restore func(), err error) {
	return config.ReplaceFactoryLayoutAtDir(targetDir, payload, opts)
}

// FactorySplitLayoutReplaceResult holds rollback and backup-discard callbacks after
// a successful split-layout replace commit.
type FactorySplitLayoutReplaceResult = config.FactorySplitLayoutReplaceResult

// PreparedFactoryLayoutPayload holds normalized factory state for split-layout persist.
type PreparedFactoryLayoutPayload = config.PreparedFactoryLayoutPayload

// PrepareFactoryLayoutPayload normalizes factory JSON for split-layout persist.
func PrepareFactoryLayoutPayload(segment string, payload []byte) (*PreparedFactoryLayoutPayload, error) {
	return config.PrepareFactoryLayoutPayload(segment, payload)
}

// ReplaceFactoryLayoutAtDirWithPreparedWithResult replaces targetDir using a
// pre-normalized payload shared by validation and layout writes.
func ReplaceFactoryLayoutAtDirWithPreparedWithResult(
	targetDir string,
	prepared *PreparedFactoryLayoutPayload,
	opts FactoryLayoutReplaceOptions,
) (*FactorySplitLayoutReplaceResult, error) {
	return config.ReplaceFactoryLayoutAtDirWithPreparedWithResult(targetDir, prepared, opts)
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

// NamedFactoryPathSegments returns the validated hierarchical path segments for
// a canonical named-factory display name.
func NamedFactoryPathSegments(name string) ([]string, error) {
	return config.NamedFactoryPathSegments(name)
}

// NamedFactoryNameFromPathSegments reconstructs the canonical named-factory
// display name from validated hierarchical path segments.
func NamedFactoryNameFromPathSegments(segments []string) (string, error) {
	return config.NamedFactoryNameFromPathSegments(segments)
}

// MapNamedFactoryDir maps a canonical named-factory display name to its
// hierarchical on-disk directory under factoriesRoot.
func MapNamedFactoryDir(factoriesRoot, name string) (string, error) {
	return config.MapNamedFactoryDir(factoriesRoot, name)
}

// GlobalNamedFactoryRootForHome builds the customer-owned global named-factory
// root for a resolved home directory.
func GlobalNamedFactoryRootForHome(homeDir string) (string, error) {
	return config.GlobalNamedFactoryRootForHome(homeDir)
}

// DefaultGlobalNamedFactoryRoot returns the default global named-factory root
// under the current user's home directory.
func DefaultGlobalNamedFactoryRoot() (string, error) {
	return config.DefaultGlobalNamedFactoryRoot()
}

// DefaultProjectNamedFactoryRoot returns the default project-local named
// factory root for a caller working directory.
func DefaultProjectNamedFactoryRoot(cwd string) (string, error) {
	return config.DefaultProjectNamedFactoryRoot(cwd)
}

// IsInvalidNamedFactory reports whether err wraps ErrInvalidNamedFactory.
func IsInvalidNamedFactory(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactory)
}

// IsInvalidNamedFactoryName reports whether err wraps ErrInvalidNamedFactoryName.
func IsInvalidNamedFactoryName(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactoryName)
}

// IsNamedFactoryAlreadyExists reports whether err wraps ErrNamedFactoryAlreadyExists.
func IsNamedFactoryAlreadyExists(err error) bool {
	return errors.Is(err, ErrNamedFactoryAlreadyExists)
}

// IsNamedFactoryNotFound reports whether err wraps ErrNamedFactoryNotFound.
func IsNamedFactoryNotFound(err error) bool {
	return errors.Is(err, ErrNamedFactoryNotFound)
}
