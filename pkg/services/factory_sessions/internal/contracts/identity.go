// Package contracts owns implementation-facing Factory Sessions capability
// aliases shared across private implementation packages. Type ownership lives
// on the Factory Sessions root.
package contracts

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// SessionIDGenerator supplies one opaque Factory Session identity.
type SessionIDGenerator = factorysessions.SessionIDGenerator

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric = factorysessions.InvocationMetric
