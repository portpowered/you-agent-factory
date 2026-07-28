// Package portableconfig is a transitional compile shim that re-exports the
// snapshots_portability-owned portable configuration implementation. Peers
// should depend on factory_definitions contracts; baseline deletion of this
// path is owned by DEL-DEF.
package portableconfig

import (
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
)

// NewPortableBundledFilesApplier binds the filesystem selected by Wire to
// portable authored-file discovery.
var NewPortableBundledFilesApplier = internalportableconfig.NewPortableBundledFilesApplier

// NewFactoryStarterWorkApplier binds the filesystem traversal selected by Wire
// to starter-Work discovery.
var NewFactoryStarterWorkApplier = internalportableconfig.NewFactoryStarterWorkApplier

// NewPortableBundledDocsPruner binds the filesystem traversal selected by Wire
// to obsolete authored-document cleanup.
var NewPortableBundledDocsPruner = internalportableconfig.NewPortableBundledDocsPruner

// NewFilesCopier binds the filesystem selected by Wire to portable file copy.
var NewFilesCopier = internalportableconfig.NewFilesCopier

// NewMaterializer binds the filesystem selected by Wire to portable file writes.
var NewMaterializer = internalportableconfig.NewMaterializer

// NewWritesValidator binds the filesystem selected by Wire to portable write checks.
var NewWritesValidator = internalportableconfig.NewWritesValidator

// MaterializeFiles writes portable bundled files into targetDir.
var MaterializeFiles = internalportableconfig.MaterializeFiles

// ValidateWrites checks portable bundled file writes for targetDir.
var ValidateWrites = internalportableconfig.ValidateWrites

// CloneReplacements clones portable bundled file replacement metadata.
var CloneReplacements = internalportableconfig.CloneReplacements

// PruneRemovedDocs removes obsolete authored documents from targetDir.
var PruneRemovedDocs = internalportableconfig.PruneRemovedDocs

// CopySupportedFiles copies supported portable files from sourceDir to targetDir.
var CopySupportedFiles = internalportableconfig.CopySupportedFiles

// NewSupportedSourceResolver binds the filesystem selected by Wire to portable
// source resolution.
var NewSupportedSourceResolver = internalportableconfig.NewSupportedSourceResolver
