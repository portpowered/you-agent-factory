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
	// malformed, or otherwise invalid stored profile.
	ErrFactoryTargetProfileUnavailable = errors.New("chat sessions: ACP Agent profile is unavailable")
	// ErrFactoryTargetCatalogUnavailable reports that the installed Factory
	// catalog could not be read from Factory Definitions.
	ErrFactoryTargetCatalogUnavailable = errors.New("chat sessions: installed Factory catalog is unavailable")
	// ErrFactoryTargetCurrentUnavailable reports that neither a
	// caller-supplied current target nor the profile's configured default
	// target belongs to the allowed, installed Factory target intersection.
	ErrFactoryTargetCurrentUnavailable = errors.New("chat sessions: no allowed, installed current Factory target is available")
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
