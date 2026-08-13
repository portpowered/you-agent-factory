// Package runtimesnapshot exposes the private Definitions-side snapshot
// capability. Its implementation lives under internal so construction and
// mutable source handling cannot leak through the subservice boundary.
package runtimesnapshot

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service resolves authored or canonical Factory sources into detached
// Runtime snapshots.
type Service interface {
	ResolveRuntimeSnapshot(
		context.Context,
		factorydefinitions.ResolveRuntimeSnapshotRequest,
	) (factorydefinitions.ResolveRuntimeSnapshotResult, error)
}
