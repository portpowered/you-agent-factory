package chatsessions

import (
	"errors"
	"fmt"
)

// Sentinel Factory target-catalog failure categories. Callers use errors.Is
// against these sentinels (typically through a returned
// *FactoryTargetCatalogError) instead of parsing Error() text.
var (
	// ErrFactoryTargetProfileUnavailable reports that the effective Operator
	// Settings ACP Agent profile could not be resolved, including a missing,
	// empty, malformed, or otherwise invalid stored profile (including a
	// configured default absent from its own effective allowlist, which
	// Operator Settings rejects while resolving the profile).
	ErrFactoryTargetProfileUnavailable = errors.New("chat sessions: ACP Agent profile is unavailable")
	// ErrFactoryTargetCatalogUnavailable reports that the installed Factory
	// catalog, or a Factory's canonical cross-root resolution, could not be
	// read from Factory Definitions.
	ErrFactoryTargetCatalogUnavailable = errors.New("chat sessions: installed Factory catalog is unavailable")
	// ErrFactoryTargetReferenceMalformed reports that a caller-supplied
	// current target is not a well-formed unversioned factory:<ref>
	// reference: it is missing the factory: namespace, is otherwise
	// lexically invalid, or carries a version or digest pin that this
	// unversioned picker contract never accepts.
	ErrFactoryTargetReferenceMalformed = errors.New("chat sessions: Factory target reference is malformed")
	// ErrFactoryTargetCatalogEmpty reports that the effective profile
	// allowlist and the installed Factory catalog share no target in common,
	// so no current/default target can ever be selected.
	ErrFactoryTargetCatalogEmpty = errors.New("chat sessions: no Factory target is both allowed and installed")
	// ErrFactoryTargetNotInstalled reports that a well-formed current/default
	// target is unknown to, or not currently installed in, the Factory
	// Definitions catalog.
	ErrFactoryTargetNotInstalled = errors.New("chat sessions: Factory target is unknown or not installed")
	// ErrFactoryTargetNotAllowed reports that a well-formed, installed
	// current/default target is outside the effective profile's allowlist.
	ErrFactoryTargetNotAllowed = errors.New("chat sessions: Factory target is outside the allowed target list")
	// ErrFactoryTargetWorkingRootIncompatible reports that the resolved
	// current/default target is pinned to a project-local root that differs
	// from the ACP client's supplied working root.
	ErrFactoryTargetWorkingRootIncompatible = errors.New("chat sessions: Factory target working root is incompatible with the client working root")
)

// FactoryTargetCatalogError reports one Factory target-catalog resolution
// failure. Target is the offending canonical target reference, empty when
// the failure applies before any target is considered. Err is one of the
// package sentinel errors so callers can use errors.Is without parsing
// Error() text. Fields carry only safe identity facts: never a credential,
// prompt, raw provider command, filesystem path, or private topology detail.
type FactoryTargetCatalogError struct {
	Target string
	Err    error
}

func (e *FactoryTargetCatalogError) Error() string {
	if e.Target == "" {
		return fmt.Sprintf("chat sessions: factory target catalog: %v", e.Err)
	}
	return fmt.Sprintf("chat sessions: factory target catalog %q: %v", e.Target, e.Err)
}

// Unwrap exposes the underlying sentinel so errors.Is/errors.As can classify
// the failure.
func (e *FactoryTargetCatalogError) Unwrap() error {
	return e.Err
}
