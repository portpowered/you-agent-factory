// Package identity owns the exact external identity effect used by Factory
// Sessions. Wire selects the process implementation; session policy decides
// how generated values are formatted and where they are applied.
package identity

// Generator supplies one opaque Factory Session identity.
type Generator func() string
