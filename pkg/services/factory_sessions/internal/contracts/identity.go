// Package contracts owns implementation-facing Factory Sessions capability
// types that must be shared across private implementation packages.
package contracts

// SessionIDGenerator supplies one opaque Factory Session identity.
type SessionIDGenerator func() string
