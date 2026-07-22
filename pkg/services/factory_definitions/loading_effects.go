package factorydefinitions

import (
	"io/fs"
	"time"
)

// Clock is the exact wall-clock effect used to version editable Factory
// Definitions. Wire selects the policy-free implementation; definition and
// session hosts never select a process-global clock.
type Clock interface {
	Now() time.Time
}

// VersionFileSystem is the exact filesystem effect used only when an older
// Factory Definition has no persisted logical/physical version metadata.
type VersionFileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

// LoadingFileSystem is the exact filesystem effect used to resolve and read
// effective Factory Definition sources. Authored-layout traversal has a
// separate contract because it is a distinct operation.
type LoadingFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
}

// RequiredToolPathLookup resolves one declared external tool without owning
// process PATH policy inside Factory Definitions.
type RequiredToolPathLookup func(string) (string, error)

// RequiredToolVersionProbe invokes one already-resolved executable solely to
// validate the version arguments declared by a Factory Definition.
type RequiredToolVersionProbe func(string, ...string) ([]byte, error)
