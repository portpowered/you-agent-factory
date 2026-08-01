// Package wire constructs the Factory Definitions authoring_layout subservice
// from exact injected layout-parse, transform, and durable-write ports.
package wire

import (
	"context"
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/authoredlayout"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/persist"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/prepare"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

type Writer = authoredlayout.Writer
type Reader = authoredlayout.Reader

func NewWriter(
	renderWorker func(factorydefinitions.FactoryWorkerConfig) ([]byte, error),
	renderWorkstation func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error),
	renderBody func(string) []byte,
	writeAgents func(string, []byte) error,
	safeSegment func(string, string) (string, error),
	safePromptPath func(string, string) (string, error),
	fileSystem factoryeffects.AuthoredLayoutWriterFileSystem,
	ensureInbox factoryeffects.InputInboxSentinelEnsurer,
	normalizeWorkstation ...func(*factorydefinitions.FactoryWorkstationConfig),
) *Writer {
	return authoredlayout.NewWriter(
		renderWorker, renderWorkstation, renderBody, writeAgents,
		safeSegment, safePromptPath, fileSystem, ensureInbox, normalizeWorkstation...,
	)
}

func NewAgentsFileWriter(
	fileSystem factoryeffects.AuthoredLayoutWriterFileSystem,
) func(string, []byte) error {
	return authoredlayout.NewAgentsFileWriter(fileSystem)
}

func NewReader(
	parseWorker func([]byte, string) (*factorydefinitions.FactoryWorkerConfig, error),
	parseWorkstation func([]byte, string) (*factorydefinitions.FactoryWorkstationConfig, error),
	parseBody func([]byte, string) (string, error),
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
) *Reader {
	return authoredlayout.NewReader(parseWorker, parseWorkstation, parseBody, fileSystem)
}

func NewFactorySourceLoader(
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return authoredlayout.NewFactorySourceLoader(fileSystem)
}

func PrepareFactoryLayout(
	ctx context.Context,
	segment string,
	payload []byte,
	validator factorydefinitions.Validator,
	decodeFactory factorydefinitions.FactoryConfigJSONDecoder,
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return authoringlayoutprepare.FactoryLayout(
		ctx, segment, payload, validator, decodeFactory,
		normalizeAuthored, encodeFactory,
	)
}

// NamedFactory exposes the authoring-owned atomic layout operation through its
// owner wire package for sibling catalog composition.
func NamedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	replaceExisting bool,
	ports authoringlayout.PersistPorts,
) (string, error) {
	return authoringlayoutpersist.NamedFactory(ctx, rootDir, name, prepared, replaceExisting, ports)
}

// NewService constructs the private authoring_layout subservice from exact
// injected authoring ports. Callers must supply Dependencies; this constructor
// does not choose host filesystem adapters or take Wire/root construction
// ownership.
func NewService(deps authoringlayout.Dependencies) (authoringlayout.Service, error) {
	if deps.Validator == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: validator is required")
	}
	if deps.MapInput == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: payload mapper is required")
	}
	if deps.DecodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory decoder is required")
	}
	if deps.NormalizeAuthored == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: authored normalizer is required")
	}
	if deps.EncodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory encoder is required")
	}
	if deps.Write == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout writer is required")
	}
	if deps.Validate == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout validator is required")
	}
	if deps.Flatten == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout flattener is required")
	}
	if deps.Expand == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout expander is required")
	}
	if deps.FileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: persistence filesystem is required")
	}
	if deps.RequireDefinitionDir == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: definition directory validator is required")
	}
	if deps.Directories == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: directory replacement store is required")
	}
	service := authoringlayoutservice.New(deps)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: implementation rejected its dependencies")
	}
	return service, nil
}
