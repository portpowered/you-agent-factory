// Package loading owns effective Factory Definition loading from authored
// directories and canonical JSON inputs.
package loading

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig"
)

// Loader coordinates Factory Definition filesystem loading while representation
// parsing remains an adapter selected by Wire.
type Loader struct {
	fileSystem               factorydefinitions.LoadingFileSystem
	loadAuthoredSource       factorydefinitions.AuthoredFactorySourceLoader
	resolveCurrentDir        factorydefinitions.CurrentFactoryDirectoryResolver
	newSource                factorydefinitions.LoadedFactorySourceFactory
	decodeFactory            func([]byte) (*factorydefinitions.FactoryConfig, error)
	decodeAuthoredLayout     func([]byte) (*factorydefinitions.FactoryConfig, error)
	encodeFactory            func(*factorydefinitions.FactoryConfig) ([]byte, error)
	normalizeAuthored        func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
	normalizeCanonical       func([]byte) (*factorydefinitions.FactoryConfig, error)
	validateManifest         func(string, *factorydefinitions.FactoryConfig) error
	validateCanonicalFiles   func(string, *factorydefinitions.FactoryConfig) error
	validateBlockingLoad     func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult
	applyPortableFiles       factorydefinitions.PortableBundledFilesApplier
	applyStarterWork         factorydefinitions.FactoryStarterWorkApplier
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer
	loadWorker               func(string) (*factorydefinitions.FactoryWorkerConfig, error)
	loadWorkstation          func(string) (*factorydefinitions.FactoryWorkstationConfig, error)
	loadWorkerBody           func(string) (string, bool, error)
	loadWorkstationBody      func(string) (string, bool, error)
	loadWorkstationPrompt    func(string, string) (string, error)
	safeLayoutSegment        func(string, string) (string, error)
	splitRuntimeEntityExists func(string) bool
}

// New constructs the Factory Definitions loader from flat representation and
// filesystem capabilities.
func New(
	fileSystem factorydefinitions.LoadingFileSystem,
	loadAuthoredSource factorydefinitions.AuthoredFactorySourceLoader,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	newSource factorydefinitions.LoadedFactorySourceFactory,
	decodeFactory func([]byte) (*factorydefinitions.FactoryConfig, error),
	decodeAuthoredLayout func([]byte) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	normalizeCanonical func([]byte) (*factorydefinitions.FactoryConfig, error),
	validateManifest func(string, *factorydefinitions.FactoryConfig) error,
	validateCanonicalFiles func(string, *factorydefinitions.FactoryConfig) error,
	validateBlockingLoad func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult,
	applyPortableFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadWorker func(string) (*factorydefinitions.FactoryWorkerConfig, error),
	loadWorkstation func(string) (*factorydefinitions.FactoryWorkstationConfig, error),
	loadWorkerBody func(string) (string, bool, error),
	loadWorkstationBody func(string) (string, bool, error),
	loadWorkstationPrompt func(string, string) (string, error),
	safeLayoutSegment func(string, string) (string, error),
	splitRuntimeEntityExists func(string) bool,
) *Loader {
	return &Loader{
		fileSystem:               fileSystem,
		loadAuthoredSource:       loadAuthoredSource,
		resolveCurrentDir:        resolveCurrentDir,
		newSource:                newSource,
		decodeFactory:            decodeFactory,
		decodeAuthoredLayout:     decodeAuthoredLayout,
		encodeFactory:            encodeFactory,
		normalizeAuthored:        normalizeAuthored,
		normalizeCanonical:       normalizeCanonical,
		validateManifest:         validateManifest,
		validateCanonicalFiles:   validateCanonicalFiles,
		validateBlockingLoad:     validateBlockingLoad,
		applyPortableFiles:       applyPortableFiles,
		applyStarterWork:         applyStarterWork,
		materializePortableFiles: materializePortableFiles,
		loadWorker:               loadWorker,
		loadWorkstation:          loadWorkstation,
		loadWorkerBody:           loadWorkerBody,
		loadWorkstationBody:      loadWorkstationBody,
		loadWorkstationPrompt:    loadWorkstationPrompt,
		safeLayoutSegment:        safeLayoutSegment,
		splitRuntimeEntityExists: splitRuntimeEntityExists,
	}
}

// FlattenFactoryConfig reads an authored Factory directory or canonical
// Factory JSON file and returns one self-contained canonical representation.
func (l *Loader) FlattenFactoryConfig(path string) ([]byte, error) {
	if err := l.validateFlatten(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("factory path is required")
	}

	data, sourcePath, factoryDir, requireSplitDefinitions, err :=
		l.readFactoryConfigSource(path)
	if err != nil {
		return nil, err
	}
	factoryConfig, err := l.decodeAuthoredLayout(data)
	if err != nil {
		return nil, fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		factoryDir,
		factoryConfig,
		requireSplitDefinitions,
		nil,
	)
	if err != nil {
		return nil, err
	}
	effectiveFactory, err := runtimeconfig.Merge(factoryConfig, runtimeDefinitions)
	if err != nil {
		return nil, err
	}
	if err := l.blockingLoadError(effectiveFactory); err != nil {
		return nil, err
	}
	if err := l.applyPortableFiles(
		factoryDir,
		effectiveFactory,
		true,
		true,
	); err != nil {
		return nil, fmt.Errorf(
			"collect portable bundled files %s: %w",
			factoryDir,
			err,
		)
	}
	if err := l.applyStarterWork(factoryDir, effectiveFactory); err != nil {
		return nil, fmt.Errorf(
			"collect shared factory starter work %s: %w",
			factoryDir,
			err,
		)
	}
	flattened, err := l.encodeFactory(effectiveFactory)
	if err != nil {
		return nil, fmt.Errorf("flatten factory config %s: %w", sourcePath, err)
	}
	return formatCanonicalFactoryJSON(flattened, sourcePath)
}

// PrepareFactoryLayoutExpansion reads one canonical Factory source and returns
// the effective definition and thin canonical representation to materialize.
func (l *Loader) PrepareFactoryLayoutExpansion(
	path string,
) (
	string,
	string,
	string,
	*factorydefinitions.FactoryConfig,
	[]byte,
	error,
) {
	if err := l.validate(); err != nil {
		return "", "", "", nil, nil, err
	}
	if path == "" {
		return "", "", "", nil, nil,
			fmt.Errorf("factory config path is required")
	}
	data, sourcePath, _, _, err := l.readFactoryConfigSource(path)
	if err != nil {
		return "", "", "", nil, nil, err
	}
	info, err := l.fileSystem.Stat(path)
	if err != nil {
		return "", "", "", nil, nil,
			fmt.Errorf("find factory config target %s: %w", path, err)
	}
	targetDir := filepath.Dir(sourcePath)
	if info.IsDir() {
		targetDir = path
	}
	factoryConfig, err := l.decodeAuthoredLayout(data)
	if err != nil {
		return "", "", "", nil, nil,
			fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}
	if err := l.validateCanonicalFiles(
		filepath.Dir(sourcePath),
		factoryConfig,
	); err != nil {
		return "", "", "", nil, nil, err
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		targetDir,
		factoryConfig,
		false,
		nil,
	)
	if err != nil {
		return "", "", "", nil, nil,
			fmt.Errorf(
				"load split runtime definitions for expand %s: %w",
				targetDir,
				err,
			)
	}
	effectiveFactory, err := runtimeconfig.Merge(
		factoryConfig,
		runtimeDefinitions,
	)
	if err != nil {
		return "", "", "", nil, nil, err
	}
	if err := l.blockingLoadError(effectiveFactory); err != nil {
		return "", "", "", nil, nil, err
	}
	authoredFactory, err := l.normalizeAuthored(effectiveFactory)
	if err != nil {
		return "", "", "", nil, nil,
			fmt.Errorf(
				"normalize authored factory config %s: %w",
				sourcePath,
				err,
			)
	}
	canonical, err := l.encodeFactory(authoredFactory)
	if err != nil {
		return "", "", "", nil, nil,
			fmt.Errorf("normalize factory config %s: %w", sourcePath, err)
	}
	return targetDir, filepath.Dir(sourcePath), sourcePath, effectiveFactory,
		canonical, nil
}

// LoadRuntimeSource resolves the current Factory directory and loads its
// effective definition.
func (l *Loader) LoadRuntimeSource(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if l == nil || l.resolveCurrentDir == nil {
		return nil, fmt.Errorf("current Factory directory resolver is required")
	}
	resolvedFactoryDir, err := l.resolveCurrentDir(factoryDir)
	if err != nil {
		if errors.Is(err, namedfactorypath.ErrLayoutNotFound) {
			return nil, fmt.Errorf("%w: %w", factorydefinitions.ErrFactoryLayoutNotFound, err)
		}
		return nil, err
	}
	return l.LoadSourceFromFactoryDir(resolvedFactoryDir, workstationLoader)
}

// LoadSourceFromFactoryDir loads one concrete Factory directory without
// following the current-Factory pointer.
func (l *Loader) LoadSourceFromFactoryDir(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	data, sourcePath, _, _, err := l.readFactoryConfigSource(factoryDir)
	if err != nil {
		return nil, err
	}
	factoryConfig, err := l.decodeFactory(data)
	if err != nil {
		return nil, sourceContextError(sourcePath, "parse factory config", err)
	}
	if err := l.blockingLoadError(factoryConfig); err != nil {
		return nil, err
	}
	if err := l.validateManifest(factoryDir, factoryConfig); err != nil {
		return nil, err
	}
	replacements, err := l.materializePortableFiles(
		factoryDir,
		factoryConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("materialize portable bundled files: %w", err)
	}
	if err := l.applyPortableFiles(
		factoryDir,
		factoryConfig,
		false,
		false,
	); err != nil {
		return nil, fmt.Errorf("collect portable bundled files: %w", err)
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		factoryDir,
		factoryConfig,
		hasInlineRuntimeDefinitions(factoryConfig),
		workstationLoader,
	)
	if err != nil {
		return nil, err
	}
	return l.newSource(
		factoryDir,
		factoryConfig,
		runtimeDefinitions,
		append([]factorydefinitions.PortableBundledFileReplacement(nil), replacements...),
	)
}

// LoadSourceFromCanonicalJSON normalizes one canonical representation and
// builds the same effective source used by directory loading.
func (l *Loader) LoadSourceFromCanonicalJSON(
	payload []byte,
	workstationLoader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	factoryConfig, err := l.normalizeCanonical(payload)
	if err != nil {
		return nil, err
	}
	if err := l.blockingLoadError(factoryConfig); err != nil {
		return nil, err
	}
	if err := l.validateCanonicalFiles("", factoryConfig); err != nil {
		return nil, err
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		"",
		factoryConfig,
		hasInlineRuntimeDefinitions(factoryConfig),
		workstationLoader,
	)
	if err != nil {
		return nil, err
	}
	return l.newSource("", factoryConfig, runtimeDefinitions, nil)
}

// ValidateFactoryDirReadOnly validates one concrete Factory directory without
// materializing or repairing any files.
func (l *Loader) ValidateFactoryDirReadOnly(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
	validatePortableFiles factorydefinitions.PortableBundledFileWritesValidator,
) error {
	if err := l.validate(); err != nil {
		return err
	}
	data, sourcePath, _, _, err := l.readFactoryConfigSource(factoryDir)
	if err != nil {
		return err
	}
	factoryConfig, err := l.decodeFactory(data)
	if err != nil {
		return sourceContextError(sourcePath, "parse factory config", err)
	}
	if err := l.blockingLoadError(factoryConfig); err != nil {
		return err
	}
	if validatePortableFiles != nil {
		if err := validatePortableFiles(factoryDir, factoryConfig); err != nil {
			return fmt.Errorf("validate portable bundled files: %w", err)
		}
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		factoryDir,
		factoryConfig,
		hasInlineRuntimeDefinitions(factoryConfig),
		workstationLoader,
	)
	if err != nil {
		return err
	}
	_, err = runtimeconfig.Merge(factoryConfig, runtimeDefinitions)
	return err
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (l *Loader) validate() error {
	switch {
	case l == nil:
		return fmt.Errorf("Factory Definitions loader is required")
	case l.fileSystem == nil:
		return fmt.Errorf("Factory Definitions loading filesystem is required")
	case l.loadAuthoredSource == nil:
		return fmt.Errorf("Factory Definitions authored source loader is required")
	case l.resolveCurrentDir == nil:
		return fmt.Errorf("current Factory directory resolver is required")
	case l.newSource == nil:
		return fmt.Errorf("Factory Definitions loaded-source factory is required")
	case l.decodeFactory == nil:
		return fmt.Errorf("Factory Definition decoder is required")
	case l.decodeAuthoredLayout == nil:
		return fmt.Errorf("authored Factory layout decoder is required")
	case l.encodeFactory == nil:
		return fmt.Errorf("Factory Definition encoder is required")
	case l.normalizeAuthored == nil:
		return fmt.Errorf("authored Factory Definition normalizer is required")
	case l.normalizeCanonical == nil:
		return fmt.Errorf("canonical Factory Definition normalizer is required")
	case l.validateManifest == nil:
		return fmt.Errorf("portable resource manifest validator is required")
	case l.validateCanonicalFiles == nil:
		return fmt.Errorf("canonical portable file validator is required")
	case l.validateBlockingLoad == nil:
		return fmt.Errorf("Factory Definition blocking-load validator is required")
	case l.applyPortableFiles == nil:
		return fmt.Errorf("portable bundled-files applier is required")
	case l.materializePortableFiles == nil:
		return fmt.Errorf("portable bundled-files materializer is required")
	case l.loadWorker == nil:
		return fmt.Errorf("authored Worker loader is required")
	case l.loadWorkstation == nil:
		return fmt.Errorf("authored Workstation loader is required")
	case l.loadWorkerBody == nil:
		return fmt.Errorf("authored Worker body loader is required")
	case l.loadWorkstationBody == nil:
		return fmt.Errorf("authored Workstation body loader is required")
	case l.loadWorkstationPrompt == nil:
		return fmt.Errorf("authored Workstation prompt loader is required")
	case l.safeLayoutSegment == nil:
		return fmt.Errorf("Factory layout segment validator is required")
	case l.splitRuntimeEntityExists == nil:
		return fmt.Errorf("split runtime entity lookup is required")
	default:
		return nil
	}
}

func (l *Loader) blockingLoadError(
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	return factorydefinitions.NewBlockingFactoryLoadError(
		l.validateBlockingLoad(factoryConfig),
	)
}

func (l *Loader) validateFlatten() error {
	if err := l.validate(); err != nil {
		return err
	}
	if l.applyStarterWork == nil {
		return fmt.Errorf("Factory starter-Work applier is required")
	}
	return nil
}

func (l *Loader) readFactoryConfigSource(
	path string,
) ([]byte, string, string, bool, error) {
	if l == nil || l.fileSystem == nil {
		return nil, "", "", false, fmt.Errorf(
			"Factory Definitions loading filesystem is required",
		)
	}
	info, err := l.fileSystem.Stat(path)
	if err != nil {
		return nil, "", "", false, fmt.Errorf(
			"find factory config source %s: %w",
			path,
			err,
		)
	}
	factoryDir := filepath.Dir(path)
	requireSplitDefinitions := false
	if info.IsDir() {
		factoryDir = path
		requireSplitDefinitions = true
	}
	source, err := l.loadAuthoredSource(path)
	if err != nil {
		return nil, "", "", false, err
	}
	return source.Data, source.Path, factoryDir, requireSplitDefinitions, nil
}

func sourceContextError(sourcePath, operation string, err error) error {
	var format factorydefinitions.AuthoredFactoryFormat
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".json":
		format = factorydefinitions.AuthoredFactoryFormatJSON
	case ".yaml", ".yml":
		format = factorydefinitions.AuthoredFactoryFormatYAML
	default:
		return fmt.Errorf("%s %s: %w", operation, sourcePath, err)
	}
	return fmt.Errorf("%s %s (%s): %w", operation, sourcePath, format, err)
}

func formatCanonicalFactoryJSON(data []byte, sourcePath string) ([]byte, error) {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return nil, fmt.Errorf(
			"format canonical factory config %s: %w",
			sourcePath,
			err,
		)
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}

type definitionLookup struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func (l *definitionLookup) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if l == nil {
		return nil, false
	}
	definition, ok := l.workers[name]
	return definition, ok
}

func (l *definitionLookup) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if l == nil {
		return nil, false
	}
	definition, ok := l.workstations[name]
	return definition, ok
}

func (l *Loader) discoverRuntimeDefinitions(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	requireSplitDefinitions bool,
	workstationLoader factorydefinitions.WorkstationLoader,
) (*definitionLookup, error) {
	definitions := &definitionLookup{
		workers: make(
			map[string]*factorydefinitions.FactoryWorkerConfig,
			len(factoryConfig.Workers),
		),
		workstations: make(
			map[string]*factorydefinitions.FactoryWorkstationConfig,
			len(factoryConfig.Workstations),
		),
	}
	for _, workstation := range factoryConfig.Workstations {
		definition, err := l.runtimeWorkstationDefinition(
			factoryDir,
			workstation,
			requireSplitDefinitions,
			workstationLoader,
		)
		if err != nil {
			return nil, fmt.Errorf("load workstation %q config: %w", workstation.Name, err)
		}
		if definition != nil {
			definitions.workstations[workstation.Name] = definition
		}
	}
	for _, worker := range factoryConfig.Workers {
		definition, err := l.runtimeWorkerDefinition(
			factoryDir,
			worker,
			requireSplitDefinitions,
		)
		if err != nil {
			return nil, fmt.Errorf("load worker %q config: %w", worker.Name, err)
		}
		if definition != nil {
			definitions.workers[worker.Name] = definition
		}
	}
	return definitions, nil
}

func (l *Loader) runtimeWorkerDefinition(
	factoryDir string,
	worker factorydefinitions.FactoryWorkerConfig,
	requireSplitDefinition bool,
) (*factorydefinitions.FactoryWorkerConfig, error) {
	if strings.TrimSpace(worker.Type) != "" {
		inlineWorker := factorydefinitions.CloneWorkerConfig(worker)
		segment, err := l.safeLayoutSegment("worker", worker.Name)
		if err != nil {
			return nil, err
		}
		workerDir := filepath.Join(factoryDir, factorydefinitions.WorkersDir, segment)
		body, found, err := l.loadWorkerBody(workerDir)
		if err != nil {
			return nil, err
		}
		if found {
			inlineWorker.Body = body
		} else if requireSplitDefinition &&
			strings.TrimSpace(inlineWorker.Body) == "" &&
			l.splitRuntimeEntityExists(workerDir) {
			return nil, fmt.Errorf(
				"worker %q is missing body-only AGENTS.md content required by the split authored layout",
				worker.Name,
			)
		}
		return &inlineWorker, nil
	}

	segment, err := l.safeLayoutSegment("worker", worker.Name)
	if err != nil {
		return nil, err
	}
	workerDir := filepath.Join(factoryDir, factorydefinitions.WorkersDir, segment)
	definition, err := l.loadWorker(workerDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if requireSplitDefinition {
				return nil, fmt.Errorf(
					"inline factory definition is incomplete: worker %q is missing definition and no AGENTS.md was found",
					worker.Name,
				)
			}
			return nil, nil
		}
		return nil, err
	}
	if definition.Name == "" {
		definition.Name = worker.Name
	}
	return definition, nil
}

func (l *Loader) runtimeWorkstationDefinition(
	factoryDir string,
	workstation factorydefinitions.FactoryWorkstationConfig,
	requireSplitDefinition bool,
	workstationLoader factorydefinitions.WorkstationLoader,
) (*factorydefinitions.FactoryWorkstationConfig, error) {
	if workstationHasRuntimeFields(workstation) {
		inlineDefinition := workstationRuntimeDefinitionFromInline(workstation)
		segment, err := l.safeLayoutSegment("workstation", workstation.Name)
		if err != nil {
			return nil, err
		}
		workstationDir := filepath.Join(
			factoryDir,
			factorydefinitions.WorkstationsDir,
			segment,
		)
		splitDefinition, err := l.splitWorkstationRuntimeDefinition(
			factoryDir,
			workstation,
			false,
			workstationLoader,
		)
		if err != nil {
			return nil, err
		}
		if splitDefinition == nil &&
			requireSplitDefinition &&
			strings.TrimSpace(inlineDefinition.Body) == "" &&
			l.splitRuntimeEntityExists(workstationDir) {
			return nil, fmt.Errorf(
				"workstation %q is missing body-only AGENTS.md content required by the split authored layout",
				workstation.Name,
			)
		}
		return mergeRuntimeWorkstationDefinitions(inlineDefinition, splitDefinition)
	}
	return l.splitWorkstationRuntimeDefinition(
		factoryDir,
		workstation,
		requireSplitDefinition,
		workstationLoader,
	)
}

func (l *Loader) splitWorkstationRuntimeDefinition(
	factoryDir string,
	workstation factorydefinitions.FactoryWorkstationConfig,
	requireSplitDefinition bool,
	workstationLoader factorydefinitions.WorkstationLoader,
) (*factorydefinitions.FactoryWorkstationConfig, error) {
	if workstationLoader != nil {
		definition, err := workstationLoader.Load(workstation.Name)
		if err != nil {
			return nil, err
		}
		if definition != nil {
			return definition, nil
		}
	}
	segment, err := l.safeLayoutSegment("workstation", workstation.Name)
	if err != nil {
		return nil, err
	}
	workstationDir := filepath.Join(
		factoryDir,
		factorydefinitions.WorkstationsDir,
		segment,
	)
	if workstationHasRuntimeFields(workstation) {
		definition, found, err := l.inlineBodyOnlyWorkstationRuntimeDefinition(
			workstationDir,
			workstation,
		)
		if err != nil || found {
			return definition, err
		}
	}
	definition, err := l.loadWorkstation(workstationDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if requireSplitDefinition {
				return nil, fmt.Errorf(
					"inline factory definition is incomplete: workstation %q is missing definition and no AGENTS.md was found",
					workstation.Name,
				)
			}
			return nil, nil
		}
		return nil, err
	}
	return definition, nil
}

func (l *Loader) inlineBodyOnlyWorkstationRuntimeDefinition(
	workstationDir string,
	workstation factorydefinitions.FactoryWorkstationConfig,
) (*factorydefinitions.FactoryWorkstationConfig, bool, error) {
	body, found, err := l.loadWorkstationBody(workstationDir)
	if err != nil || !found {
		return nil, found, err
	}
	definition := workstationRuntimeDefinitionFromInline(workstation)
	definition.Body = body
	if definition.PromptFile != "" {
		prompt, err := l.loadWorkstationPrompt(workstationDir, definition.PromptFile)
		if err != nil {
			return nil, false, err
		}
		definition.PromptTemplate = prompt
	} else {
		definition.PromptTemplate = body
	}
	return definition, true, nil
}

func workstationRuntimeDefinitionFromInline(
	workstation factorydefinitions.FactoryWorkstationConfig,
) *factorydefinitions.FactoryWorkstationConfig {
	definition := factorydefinitions.CloneWorkstationConfig(workstation)
	if strings.TrimSpace(definition.Type) == "" {
		if strings.TrimSpace(definition.WorkerTypeName) == "" {
			definition.Type = factorydefinitions.WorkstationTypeLogical
		} else {
			definition.Type = factorydefinitions.WorkstationTypeModel
		}
	}
	return &definition
}

func mergeRuntimeWorkstationDefinitions(
	inlineDefinition *factorydefinitions.FactoryWorkstationConfig,
	splitDefinition *factorydefinitions.FactoryWorkstationConfig,
) (*factorydefinitions.FactoryWorkstationConfig, error) {
	if inlineDefinition == nil {
		return splitDefinition, nil
	}
	if splitDefinition == nil {
		return inlineDefinition, nil
	}
	lookup := &definitionLookup{
		workers: map[string]*factorydefinitions.FactoryWorkerConfig{},
		workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
			inlineDefinition.Name: splitDefinition,
		},
	}
	mergedFactory, err := runtimeconfig.Merge(
		&factorydefinitions.FactoryConfig{
			Workstations: []factorydefinitions.FactoryWorkstationConfig{
				*inlineDefinition,
			},
		},
		lookup,
	)
	if err != nil {
		return nil, err
	}
	merged := &mergedFactory.Workstations[0]
	if inlineDefinition.Body == "" &&
		splitDefinition.Body == inlineDefinition.PromptTemplate &&
		merged.PromptTemplate == inlineDefinition.PromptTemplate {
		merged.Body = ""
	}
	return merged, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func hasInlineRuntimeDefinitions(factoryConfig *factorydefinitions.FactoryConfig) bool {
	for _, worker := range factoryConfig.Workers {
		if strings.TrimSpace(worker.Type) != "" ||
			strings.TrimSpace(worker.Provider) != "" ||
			strings.TrimSpace(worker.Model) != "" ||
			strings.TrimSpace(worker.ModelProvider) != "" ||
			strings.TrimSpace(worker.ExecutorProvider) != "" ||
			strings.TrimSpace(worker.SessionID) != "" ||
			strings.TrimSpace(worker.Command) != "" ||
			strings.TrimSpace(worker.Timeout) != "" ||
			strings.TrimSpace(worker.StopToken) != "" ||
			strings.TrimSpace(worker.OpenCodeAgent) != "" ||
			strings.TrimSpace(worker.Body) != "" ||
			len(worker.Args) > 0 ||
			len(worker.Resources) > 0 ||
			worker.Concurrency != 0 ||
			worker.SkipPermissions ||
			worker.Auth != nil ||
			worker.Linear != nil {
			return true
		}
	}
	for _, workstation := range factoryConfig.Workstations {
		if workstationHasInlineRuntimeDefinitionFields(workstation) {
			return true
		}
	}
	return false
}

func workstationHasInlineRuntimeDefinitionFields(
	workstation factorydefinitions.FactoryWorkstationConfig,
) bool {
	if strings.TrimSpace(workstation.Type) == factorydefinitions.WorkstationTypeLogical &&
		workstation.Runner == "" &&
		workstation.OpenCodeAgent == "" &&
		workstation.PromptFile == "" &&
		workstation.OutputSchema == "" &&
		workstation.Timeout == "" &&
		workstation.Limits.MaxRetries == 0 &&
		workstation.Limits.MaxExecutionTime == "" &&
		workstation.Body == "" &&
		workstation.PromptTemplate == "" &&
		workstation.WorkingDirectory == "" &&
		workstation.Worktree == "" &&
		len(workstation.Env) == 0 {
		return false
	}
	return workstationHasRuntimeFields(workstation)
}

func workstationHasRuntimeFields(
	workstation factorydefinitions.FactoryWorkstationConfig,
) bool {
	return strings.TrimSpace(workstation.Type) != "" ||
		workstation.Runner != "" ||
		workstation.OpenCodeAgent != "" ||
		workstation.PromptFile != "" ||
		workstation.OutputSchema != "" ||
		workstation.Timeout != "" ||
		workstation.Limits.MaxRetries != 0 ||
		workstation.Limits.MaxExecutionTime != "" ||
		workstation.Body != "" ||
		workstation.PromptTemplate != "" ||
		workstation.WorkingDirectory != "" ||
		workstation.Worktree != "" ||
		len(workstation.Env) > 0
}
