// Package namedpaths is a transitional compile shim that re-exports the
// catalog-owned named-path implementation. Peers should depend on
// factory_definitions contracts; baseline deletion of this path is owned by
// DEL-DEF.
package namedpaths

import (
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

var (
	ErrInvalidName    = catalognamedpaths.ErrInvalidName
	ErrNotFound       = catalognamedpaths.ErrNotFound
	ErrLayoutNotFound = catalognamedpaths.ErrLayoutNotFound
)

// CandidatePaths contains detached candidate paths for cross-root lookup.
type CandidatePaths = catalognamedpaths.CandidatePaths

// Resolver owns canonical named-Factory filesystem resolution.
type Resolver = catalognamedpaths.Resolver

// New constructs a named-path resolver from the exact filesystem port.
var New = catalognamedpaths.New

// ValidateName validates one canonical named Factory display name.
var ValidateName = catalognamedpaths.ValidateName

// PathSegments maps a canonical name to its safe hierarchical layout.
var PathSegments = catalognamedpaths.PathSegments

// NameFromPathSegments reconstructs the canonical display name from segments.
var NameFromPathSegments = catalognamedpaths.NameFromPathSegments

// MapDir maps a canonical named-factory display name to its directory.
var MapDir = catalognamedpaths.MapDir
