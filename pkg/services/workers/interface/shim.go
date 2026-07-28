// Package mockworkers is a transitional compile shim that re-exports mock-worker
// contract helpers from the private workers/internal destination. Baseline
// deletion of this path is owned by DEL-WRK.
package mockworkers

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/interface"
)

type (
	MockWorkerRunType                 = private.MockWorkerRunType
	MockWorkerUnmatchedDispatchPolicy = private.MockWorkerUnmatchedDispatchPolicy
	MockWorkersConfig                 = private.MockWorkersConfig
	MockWorkerConfig                  = private.MockWorkerConfig
	MockWorkInputSelector             = private.MockWorkInputSelector
	MockWorkerScriptConfig            = private.MockWorkerScriptConfig
	MockWorkerRejectConfig            = private.MockWorkerRejectConfig
	MockWorkersConfigFileSystem       = private.MockWorkersConfigFileSystem
	MockWorkersConfigLoader           = private.MockWorkersConfigLoader
	Inventory                         = private.Inventory
	RunTypeUnion                      = private.RunTypeUnion
	RunTypeRecord                     = private.RunTypeRecord
	UnmatchedDispatchPolicy           = private.UnmatchedDispatchPolicy
	ValidationBoundary                = private.ValidationBoundary
	NotAcceptedCapability             = private.NotAcceptedCapability
	FieldRecord                       = private.FieldRecord
	ContractFieldDocumentation        = private.ContractFieldDocumentation
	InputInventory                    = private.InputInventory
	InputCase                         = private.InputCase
	MockWorkersConfigExpectation      = private.MockWorkersConfigExpectation
	MockWorkerExpectation             = private.MockWorkerExpectation
)

const (
	MockWorkerRunTypeAccept                      = private.MockWorkerRunTypeAccept
	MockWorkerRunTypeScript                      = private.MockWorkerRunTypeScript
	MockWorkerRunTypeReject                      = private.MockWorkerRunTypeReject
	MockWorkerUnmatchedDispatchPolicyAccept      = private.MockWorkerUnmatchedDispatchPolicyAccept
	MockWorkerUnmatchedDispatchPolicyPassthrough = private.MockWorkerUnmatchedDispatchPolicyPassthrough
	FormatVersion                                = private.FormatVersion
	SchemaRelativePath                           = private.SchemaRelativePath
	SchemaID                                     = private.SchemaID
	ContractFormatVersion                        = private.ContractFormatVersion
	DocumentationIDPrefix                        = private.DocumentationIDPrefix
	TopologyBaselineRelativePath                 = private.TopologyBaselineRelativePath
	InputInventoryFormatVersion                  = private.InputInventoryFormatVersion
	InputIndexBaselineRelativePath               = private.InputIndexBaselineRelativePath
)

var (
	NewEmptyMockWorkersConfig  = private.NewEmptyMockWorkersConfig
	NewMockWorkersConfigLoader = private.NewMockWorkersConfigLoader
	ParseMockWorkersConfig     = private.ParseMockWorkersConfig
	DocumentationItemID        = private.DocumentationItemID
	ProjectContractFieldDocumentation = private.ProjectContractFieldDocumentation
	ProjectTopologyInventory   = private.ProjectTopologyInventory
	ProjectInputInventory      = private.ProjectInputInventory
	MarshalInputInventoryJSON  = private.MarshalInputInventoryJSON
	MarshalCanonicalJSON       = private.MarshalCanonicalJSON
	NormalizeFixtureBytes      = private.NormalizeFixtureBytes
	NormalizeSourceBytes       = private.NormalizeSourceBytes
)
