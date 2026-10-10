// Package contracts owns implementation-facing Factory Sessions capability
// types that must be shared across private implementation packages.
package contracts

import (
	"regexp"
	"strings"
)

var sessionIdentityPattern = regexp.MustCompile(`^(dur-sess-[a-f0-9]{32}|~default|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)

// SessionIdentity is a Factory Session selector compatible with durable storage.
type SessionIdentity string

// Valid checks the normalized identity without allocating or opening a session.
func (id SessionIdentity) Valid() bool {
	return sessionIdentityPattern.MatchString(strings.TrimSpace(string(id)))
}

// SessionIdentityForm describes the existing accepted identity grammar.
const SessionIdentityForm = "~default, a lowercase hyphenated UUID, or dur-sess- followed by 32 lowercase hexadecimal digits"

// SessionIDGenerator supplies one opaque Factory Session identity.
type SessionIDGenerator func() string

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}
