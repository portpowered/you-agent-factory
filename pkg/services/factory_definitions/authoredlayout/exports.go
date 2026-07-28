// Package authoredlayout is a transitional compile shim that re-exports the
// authoring_layout-owned split-layout implementation. Peers should depend on
// factory_definitions contracts; baseline deletion of this path is owned by
// DEL-DEF.
package authoredlayout

import (
	internalauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
)

// Writer materializes prepared Factory Definitions into their split authored
// filesystem representation.
type Writer = internalauthoredlayout.Writer

// WorkerParser parses one authored worker definition from disk.
type WorkerParser = internalauthoredlayout.WorkerParser

// WorkstationParser parses one authored workstation definition from disk.
type WorkstationParser = internalauthoredlayout.WorkstationParser

// BodyParser parses one authored body file from disk.
type BodyParser = internalauthoredlayout.BodyParser

// Reader loads split-layout authored files from disk.
type Reader = internalauthoredlayout.Reader

// NewWriter constructs a split-layout writer from flat representation adapters.
var NewWriter = internalauthoredlayout.NewWriter

// NewAgentsFileWriter materializes one rendered AGENTS.md file.
var NewAgentsFileWriter = internalauthoredlayout.NewAgentsFileWriter

// NewReader constructs a split-layout reader from representation adapters.
var NewReader = internalauthoredlayout.NewReader

// NewFactorySourceLoader constructs the authored factory source loader.
var NewFactorySourceLoader = internalauthoredlayout.NewFactorySourceLoader
