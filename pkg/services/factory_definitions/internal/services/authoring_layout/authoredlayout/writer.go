package authoredlayout

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimeconfig"
)

// Writer materializes prepared Factory Definitions into their split authored
// filesystem representation.
type Writer struct {
	renderWorker      func(factorydefinitions.FactoryWorkerConfig) ([]byte, error)
	renderWorkstation func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error)
	renderBody        func(string) []byte
	writeAgents       func(string, []byte) error
	safeSegment       func(string, string) (string, error)
	safePromptPath    func(string, string) (string, error)
	fileSystem        factorydefinitions.AuthoredLayoutWriterFileSystem
	ensureInbox       factorydefinitions.InputInboxSentinelEnsurer
}

// NewWriter constructs a split-layout writer from flat representation
// adapters selected by Wire.
func NewWriter(
	renderWorker func(factorydefinitions.FactoryWorkerConfig) ([]byte, error),
	renderWorkstation func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error),
	renderBody func(string) []byte,
	writeAgents func(string, []byte) error,
	safeSegment func(string, string) (string, error),
	safePromptPath func(string, string) (string, error),
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
	ensureInbox factorydefinitions.InputInboxSentinelEnsurer,
) *Writer {
	return &Writer{
		renderWorker:      renderWorker,
		renderWorkstation: renderWorkstation,
		renderBody:        renderBody,
		writeAgents:       writeAgents,
		safeSegment:       safeSegment,
		safePromptPath:    safePromptPath,
		fileSystem:        fileSystem,
		ensureInbox:       ensureInbox,
	}
}

// WriteAgentsFile materializes one rendered AGENTS.md file. Authored-layout
// persistence owns this filesystem effect; mapping only renders the bytes.
func NewAgentsFileWriter(
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
) func(string, []byte) error {
	return func(dir string, content []byte) error {
		if fileSystem == nil {
			return fmt.Errorf("Factory Definitions authored-layout writer filesystem is required")
		}
		return writeAgentsFile(fileSystem, dir, content)
	}
}

func writeAgentsFile(
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
	dir string,
	content []byte,
) error {
	if err := fileSystem.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	if err := fileSystem.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WritePrepared materializes one prepared Factory Definition and its portable
// content into targetDir.
func (w *Writer) WritePrepared(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	sourcePath string,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	prunePortableDocs factorydefinitions.PortableBundledDocsPruner,
) error {
	if err := w.validate(); err != nil {
		return err
	}
	if prepared == nil {
		return fmt.Errorf("prepared Factory layout payload is required")
	}
	if err := w.fileSystem.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}
	rootFileName, err := preparedRootFileName(prepared.RootFileName)
	if err != nil {
		return err
	}
	formatted, err := formatCanonicalFactory(
		prepared.Canonical,
		sourcePath,
		rootFileName,
	)
	if err != nil {
		return err
	}
	factoryPath := filepath.Join(targetDir, rootFileName)
	if err := w.fileSystem.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	if _, _, _, err := w.writeRuntimeFiles(
		targetDir,
		prepared.Config,
		true,
	); err != nil {
		return err
	}
	if materializePortableFiles != nil {
		if _, err := materializePortableFiles(targetDir, prepared.Config); err != nil {
			return err
		}
	}
	if prunePortableDocs != nil {
		if err := prunePortableDocs(targetDir, prepared.Config); err != nil {
			return err
		}
	}
	if err := w.ensureDefaultInputChannelDirectories(
		targetDir,
		prepared.Config,
	); err != nil {
		return err
	}
	return w.ensureCanonicalInputInboxSentinels(targetDir, prepared.Config)
}

// Expand materializes canonical Factory JSON into a split authored layout
// without overwriting existing split Worker or Workstation definitions.
func (w *Writer) Expand(
	targetDir string,
	sourceDir string,
	sourcePath string,
	factoryConfig *factorydefinitions.FactoryConfig,
	canonical []byte,
	validatePortableFiles factorydefinitions.PortableBundledFileWritesValidator,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	copyPortableFiles factorydefinitions.PortableBundledFilesCopier,
) (factorydefinitions.LayoutExpansionReport, error) {
	if err := w.validate(); err != nil {
		return factorydefinitions.LayoutExpansionReport{}, err
	}
	if factoryConfig == nil {
		return factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("Factory Definition config is required")
	}
	if validatePortableFiles != nil {
		if err := validatePortableFiles(targetDir, factoryConfig); err != nil {
			return factorydefinitions.LayoutExpansionReport{}, err
		}
	}
	if err := w.fileSystem.MkdirAll(targetDir, 0o755); err != nil {
		return factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}
	formatted, err := formatCanonicalFactoryJSON(canonical, sourcePath)
	if err != nil {
		return factorydefinitions.LayoutExpansionReport{}, err
	}
	factoryPath := filepath.Join(targetDir, factorydefinitions.FactoryConfigFile)
	if err := w.fileSystem.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	workerPaths, workstationPaths, promptPaths, err := w.writeRuntimeFiles(
		targetDir,
		factoryConfig,
		false,
	)
	if err != nil {
		return factorydefinitions.LayoutExpansionReport{}, err
	}
	report := factorydefinitions.LayoutExpansionReport{
		FactoryConfigPaths:    1,
		WorkerAgentPaths:      workerPaths,
		WorkstationAgentPaths: workstationPaths,
		PromptPaths:           promptPaths,
	}
	if materializePortableFiles != nil {
		replacements, err := materializePortableFiles(targetDir, factoryConfig)
		if err != nil {
			return factorydefinitions.LayoutExpansionReport{}, err
		}
		report.BundledReplacements = replacements
	}
	if copyPortableFiles != nil {
		if err := copyPortableFiles(
			sourceDir,
			targetDir,
			factoryConfig,
		); err != nil {
			return factorydefinitions.LayoutExpansionReport{}, err
		}
	}
	if err := w.writeExpandedReferencedScripts(
		sourceDir,
		targetDir,
		factoryConfig,
	); err != nil {
		return factorydefinitions.LayoutExpansionReport{}, err
	}
	return report, nil
}

func (w *Writer) validate() error {
	switch {
	case w == nil:
		return fmt.Errorf("Factory Definitions authored-layout writer is required")
	case w.renderWorker == nil:
		return fmt.Errorf("Worker authored-layout renderer is required")
	case w.renderWorkstation == nil:
		return fmt.Errorf("Workstation authored-layout renderer is required")
	case w.renderBody == nil:
		return fmt.Errorf("authored-layout body renderer is required")
	case w.writeAgents == nil:
		return fmt.Errorf("authored-layout AGENTS.md writer is required")
	case w.safeSegment == nil:
		return fmt.Errorf("authored-layout segment resolver is required")
	case w.safePromptPath == nil:
		return fmt.Errorf("authored-layout prompt resolver is required")
	case w.fileSystem == nil:
		return fmt.Errorf("Factory Definitions authored-layout writer filesystem is required")
	case w.ensureInbox == nil:
		return fmt.Errorf("Factory Definitions input inbox sentinel ensurer is required")
	default:
		return nil
	}
}

func (w *Writer) writeRuntimeFiles(
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	overwrite bool,
) (int, int, int, error) {
	if factoryConfig == nil {
		return 0, 0, 0, fmt.Errorf("Factory Definition config is required")
	}
	workerPaths, err := w.writeWorkers(
		targetDir,
		factoryConfig.Workers,
		overwrite,
	)
	if err != nil {
		return 0, 0, 0, err
	}
	workstationPaths, promptPaths, err := w.writeWorkstations(
		targetDir,
		factoryConfig.Workstations,
		overwrite,
	)
	if err != nil {
		return 0, 0, 0, err
	}
	if overwrite {
		if err := w.pruneStaleRuntimeDirs(targetDir, factoryConfig); err != nil {
			return 0, 0, 0, err
		}
	}
	return workerPaths, workstationPaths, promptPaths, nil
}

func (w *Writer) writeWorkers(
	targetDir string,
	workers []factorydefinitions.FactoryWorkerConfig,
	overwrite bool,
) (int, error) {
	workersDir := filepath.Join(targetDir, factorydefinitions.WorkersDir)
	if err := w.fileSystem.MkdirAll(workersDir, 0o755); err != nil {
		return 0, fmt.Errorf(
			"create workers directory %s: %w",
			workersDir,
			err,
		)
	}
	workers = append([]factorydefinitions.FactoryWorkerConfig(nil), workers...)
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].Name < workers[j].Name
	})
	written := 0
	for _, worker := range workers {
		segment, err := w.safeSegment("worker", worker.Name)
		if err != nil {
			return 0, err
		}
		workerDir := filepath.Join(workersDir, segment)
		content := w.renderBody(worker.Body)
		if worker.Type == "" {
			if !overwrite {
				exists, err := w.agentsFileExists(workerDir)
				if err != nil {
					return 0, fmt.Errorf(
						"check worker %q AGENTS.md: %w",
						worker.Name,
						err,
					)
				}
				if exists {
					continue
				}
			}
			content, err = w.renderWorker(workerForExpansion(worker))
			if err != nil {
				return 0, fmt.Errorf(
					"render worker %q AGENTS.md: %w",
					worker.Name,
					err,
				)
			}
		}
		if err := w.writeAgents(workerDir, content); err != nil {
			return 0, fmt.Errorf(
				"write worker %q AGENTS.md: %w",
				worker.Name,
				err,
			)
		}
		written++
	}
	return written, nil
}

func (w *Writer) writeWorkstations(
	targetDir string,
	workstations []factorydefinitions.FactoryWorkstationConfig,
	overwrite bool,
) (int, int, error) {
	workstationsDir := filepath.Join(
		targetDir,
		factorydefinitions.WorkstationsDir,
	)
	if err := w.fileSystem.MkdirAll(workstationsDir, 0o755); err != nil {
		return 0, 0, fmt.Errorf(
			"create workstations directory %s: %w",
			workstationsDir,
			err,
		)
	}
	workstations = append(
		[]factorydefinitions.FactoryWorkstationConfig(nil),
		workstations...,
	)
	sort.Slice(workstations, func(i, j int) bool {
		return workstations[i].Name < workstations[j].Name
	})
	agentsWritten := 0
	promptsWritten := 0
	for _, workstation := range workstations {
		agents, prompts, err := w.writeWorkstation(
			workstationsDir,
			workstation,
			overwrite,
		)
		if err != nil {
			return 0, 0, err
		}
		agentsWritten += agents
		promptsWritten += prompts
	}
	return agentsWritten, promptsWritten, nil
}

func (w *Writer) writeWorkstation(
	workstationsDir string,
	workstation factorydefinitions.FactoryWorkstationConfig,
	overwrite bool,
) (int, int, error) {
	segment, err := w.safeSegment("workstation", workstation.Name)
	if err != nil {
		return 0, 0, err
	}
	workstationDir := filepath.Join(workstationsDir, segment)
	if !workstationHasRuntimeFields(workstation) && !overwrite {
		exists, err := w.agentsFileExists(workstationDir)
		if err != nil {
			return 0, 0, fmt.Errorf(
				"check workstation %q AGENTS.md: %w",
				workstation.Name,
				err,
			)
		}
		if exists {
			return 0, 0, nil
		}
	}
	definition, promptContent := workstationForExpansion(workstation)
	content := w.renderBody(definition.Body)
	if !workstationHasRuntimeFields(workstation) {
		content, err = w.renderWorkstation(definition)
		if err != nil {
			return 0, 0, fmt.Errorf(
				"render workstation %q AGENTS.md: %w",
				workstation.Name,
				err,
			)
		}
	}
	if err := w.writeAgents(workstationDir, content); err != nil {
		return 0, 0, fmt.Errorf(
			"write workstation %q AGENTS.md: %w",
			workstation.Name,
			err,
		)
	}
	if definition.PromptFile == "" {
		return 1, 0, nil
	}
	promptPath, err := w.safePromptPath(
		workstationDir,
		definition.PromptFile,
	)
	if err != nil {
		return 0, 0, fmt.Errorf(
			"resolve workstation %q prompt file: %w",
			workstation.Name,
			err,
		)
	}
	if err := w.fileSystem.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return 0, 0, fmt.Errorf(
			"create workstation %q prompt directory: %w",
			workstation.Name,
			err,
		)
	}
	if err := w.fileSystem.WriteFile(promptPath, []byte(promptContent), 0o644); err != nil {
		return 0, 0, fmt.Errorf(
			"write workstation %q prompt file: %w",
			workstation.Name,
			err,
		)
	}
	return 1, 1, nil
}

func workerForExpansion(
	worker factorydefinitions.FactoryWorkerConfig,
) factorydefinitions.FactoryWorkerConfig {
	if worker.Type == "" {
		return factorydefinitions.FactoryWorkerConfig{
			Type: factorydefinitions.WorkerTypeModel,
		}
	}
	expanded := factorydefinitions.CloneWorkerConfig(worker)
	expanded.Name = ""
	return expanded
}

func workstationForExpansion(
	workstation factorydefinitions.FactoryWorkstationConfig,
) (factorydefinitions.FactoryWorkstationConfig, string) {
	if !workstationHasRuntimeFields(workstation) {
		definition := factorydefinitions.FactoryWorkstationConfig{
			Type:           factorydefinitions.WorkstationTypeModel,
			WorkerTypeName: workstation.WorkerTypeName,
			StopWords:      append([]string(nil), workstation.StopWords...),
		}
		if workstation.WorkerTypeName == "" {
			definition.Type = factorydefinitions.WorkstationTypeLogical
		}
		return definition, ""
	}
	definition := factorydefinitions.CloneWorkstationConfig(workstation)
	runtimeconfig.NormalizeCanonicalWorkstationRuntime(&definition)
	promptContent := ""
	if definition.PromptFile != "" {
		promptContent = definition.PromptTemplate
		if promptContent == "" {
			promptContent = definition.Body
		}
	} else if definition.Body == "" {
		definition.Body = definition.PromptTemplate
	}
	return definition, promptContent
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

func (w *Writer) pruneStaleRuntimeDirs(
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	if err := w.pruneStaleEntityDirs(
		targetDir,
		factorydefinitions.WorkersDir,
		"worker",
		len(factoryConfig.Workers),
		func(index int) (string, error) {
			return w.safeSegment("worker", factoryConfig.Workers[index].Name)
		},
	); err != nil {
		return err
	}
	return w.pruneStaleEntityDirs(
		targetDir,
		factorydefinitions.WorkstationsDir,
		"workstation",
		len(factoryConfig.Workstations),
		func(index int) (string, error) {
			return w.safeSegment(
				"workstation",
				factoryConfig.Workstations[index].Name,
			)
		},
	)
}

func (w *Writer) pruneStaleEntityDirs(
	targetDir string,
	parentDirName string,
	kind string,
	count int,
	segmentAt func(int) (string, error),
) error {
	parentDir := filepath.Join(targetDir, parentDirName)
	info, err := w.fileSystem.Stat(parentDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s directory: %w", parentDirName, err)
	}
	if !info.IsDir() {
		return nil
	}
	keep := make(map[string]struct{}, count)
	for index := 0; index < count; index++ {
		segment, err := segmentAt(index)
		if err != nil {
			return err
		}
		keep[segment] = struct{}{}
	}
	children, err := w.fileSystem.ReadDir(parentDir)
	if err != nil {
		return fmt.Errorf("read %s directory: %w", parentDirName, err)
	}
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		if _, ok := keep[child.Name()]; ok {
			continue
		}
		entityDir := filepath.Join(parentDir, child.Name())
		if err := w.fileSystem.RemoveAll(entityDir); err != nil {
			return fmt.Errorf("prune %s %q: %w", kind, child.Name(), err)
		}
	}
	return nil
}

func (w *Writer) agentsFileExists(dir string) (bool, error) {
	_, err := w.fileSystem.Stat(filepath.Join(dir, factorydefinitions.FactoryAgentsFileName))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (w *Writer) writeExpandedReferencedScripts(
	sourceDir string,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	workersByName := make(
		map[string]factorydefinitions.FactoryWorkerConfig,
		len(factoryConfig.Workers),
	)
	for _, worker := range factoryConfig.Workers {
		workersByName[worker.Name] = factorydefinitions.CloneWorkerConfig(worker)
	}
	copied := make(map[string]bool)
	for _, workstation := range factoryConfig.Workstations {
		if !workstation.CopyReferencedScripts {
			continue
		}
		paths, err := workstationReferencedScriptPaths(
			workstation,
			workersByName,
		)
		if err != nil {
			return fmt.Errorf(
				"copy referenced scripts for workstation %q: %w",
				workstation.Name,
				err,
			)
		}
		for _, relativePath := range paths {
			if copied[relativePath] {
				continue
			}
			if err := w.copyFactoryRelativeFile(
				sourceDir,
				targetDir,
				relativePath,
			); err != nil {
				return fmt.Errorf(
					"copy referenced script %q for workstation %q: %w",
					relativePath,
					workstation.Name,
					err,
				)
			}
			copied[relativePath] = true
		}
	}
	return nil
}

func workstationReferencedScriptPaths(
	workstation factorydefinitions.FactoryWorkstationConfig,
	workersByName map[string]factorydefinitions.FactoryWorkerConfig,
) ([]string, error) {
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		return nil, nil
	}
	worker, ok := workersByName[workstation.WorkerTypeName]
	if !ok {
		return nil, fmt.Errorf("worker %q not found", workstation.WorkerTypeName)
	}
	if worker.Type != factorydefinitions.WorkerTypeScript {
		return nil, nil
	}
	paths := make([]string, 0, 2)
	commandPath, err := referencedScriptPath(worker.Command)
	if err != nil {
		return nil, err
	}
	if commandPath != "" {
		paths = append(paths, commandPath)
	}
	if !isScriptInterpreterCommand(worker.Command) {
		return paths, nil
	}
	argPath, err := firstReferencedScriptArg(worker.Command, worker.Args)
	if err != nil {
		return nil, err
	}
	if argPath != "" && argPath != commandPath {
		paths = append(paths, argPath)
	}
	return paths, nil
}

func referencedScriptPath(raw string) (string, error) {
	if !looksLikeScriptPathReference(raw) {
		return "", nil
	}
	return normalizeFactoryRelativeScriptPath(raw)
}

func firstReferencedScriptArg(command string, args []string) (string, error) {
	skipNextValue := false
	nextValueIsScriptPath := false
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if nextValueIsScriptPath && shouldUseScriptArg(trimmed) {
			return normalizeFactoryRelativeScriptPath(trimmed)
		}
		nextValueIsScriptPath = false
		if skipNextValue {
			skipNextValue = false
			continue
		}
		if !shouldInspectScriptArg(trimmed) {
			continue
		}
		if trimmed == "--" {
			nextValueIsScriptPath = true
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			switch interpreterFlagModeForArg(command, trimmed) {
			case interpreterArgFlagSkipNextValue:
				skipNextValue = true
			case interpreterArgFlagScriptPathValue:
				nextValueIsScriptPath = true
			}
			continue
		}
		if looksLikeScriptPathReference(trimmed) {
			return normalizeFactoryRelativeScriptPath(trimmed)
		}
	}
	return "", nil
}

func shouldUseScriptArg(arg string) bool {
	return shouldInspectScriptArg(arg) && arg != "--"
}

func shouldInspectScriptArg(arg string) bool {
	return arg != "" &&
		!strings.Contains(arg, "{{") &&
		!strings.Contains(arg, "}}")
}

type interpreterArgFlagMode int

const (
	interpreterArgFlagIgnore interpreterArgFlagMode = iota
	interpreterArgFlagSkipNextValue
	interpreterArgFlagScriptPathValue
)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func interpreterFlagModeForArg(
	command string,
	arg string,
) interpreterArgFlagMode {
	normalized := strings.ToLower(strings.TrimSpace(arg))
	if normalized == "" ||
		!strings.HasPrefix(normalized, "-") ||
		strings.Contains(normalized, "=") {
		return interpreterArgFlagIgnore
	}
	switch interpreterCommandKey(command) {
	case "node", "bun":
		switch normalized {
		case "-e", "--eval", "-p", "--print", "-r", "--require", "--import",
			"--loader", "--experimental-loader", "--conditions", "--input-type",
			"--env-file", "--env-file-if-exists", "--inspect-port",
			"--openssl-config", "--redirect-warnings",
			"--trace-event-categories", "--title", "--watch-path":
			return interpreterArgFlagSkipNextValue
		}
	case "python", "python3":
		switch normalized {
		case "-c", "-m", "-w", "-x":
			return interpreterArgFlagSkipNextValue
		}
	case "powershell", "pwsh":
		switch normalized {
		case "-file", "-f":
			return interpreterArgFlagScriptPathValue
		case "-command", "-c", "-configurationname", "-custompipename",
			"-encodedcommand", "-ec", "-executionpolicy", "-inputformat",
			"-outputformat", "-settingsfile", "-workingdirectory":
			return interpreterArgFlagSkipNextValue
		}
	case "bash", "sh":
		switch normalized {
		case "-c", "-o":
			return interpreterArgFlagSkipNextValue
		}
	case "ruby":
		switch normalized {
		case "-c", "-e", "-i", "-r":
			return interpreterArgFlagSkipNextValue
		}
	case "perl":
		switch normalized {
		case "-e", "-i", "-m", "-x":
			return interpreterArgFlagSkipNextValue
		}
	}
	return interpreterArgFlagIgnore
}

func interpreterCommandKey(command string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func looksLikeScriptPathReference(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return true
	}
	if strings.Contains(trimmed, "{{") || strings.Contains(trimmed, "}}") {
		return false
	}
	if strings.ContainsAny(trimmed, `/\`) || strings.HasPrefix(trimmed, ".") {
		return true
	}
	switch strings.ToLower(filepath.Ext(trimmed)) {
	case ".py", ".ps1", ".psm1", ".sh", ".bash", ".zsh", ".js", ".mjs",
		".cjs", ".ts", ".rb", ".pl", ".cmd", ".bat":
		return true
	default:
		return false
	}
}

func isScriptInterpreterCommand(command string) bool {
	switch interpreterCommandKey(command) {
	case "python", "python3", "bash", "sh", "powershell", "pwsh", "node",
		"bun", "ruby", "perl":
		return true
	default:
		return false
	}
}

func normalizeFactoryRelativeScriptPath(raw string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("script path is required")
	}
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf(
			"script path %q must be relative to the factory directory",
			raw,
		)
	}
	if cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"script path %q cannot escape the factory directory",
			raw,
		)
	}
	return cleaned, nil
}

func (w *Writer) copyFactoryRelativeFile(
	sourceDir string,
	targetDir string,
	relativePath string,
) error {
	sourcePath := filepath.Join(sourceDir, relativePath)
	sourceInfo, err := w.fileSystem.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("read source file %s: %w", sourcePath, err)
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source file %s is a directory", sourcePath)
	}
	data, err := w.fileSystem.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source file %s: %w", sourcePath, err)
	}
	targetPath := filepath.Join(targetDir, relativePath)
	if err := w.fileSystem.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf(
			"create target directory for %s: %w",
			targetPath,
			err,
		)
	}
	mode := sourceInfo.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if err := w.fileSystem.WriteFile(targetPath, data, mode); err != nil {
		return fmt.Errorf("write target file %s: %w", targetPath, err)
	}
	return nil
}

func (w *Writer) ensureDefaultInputChannelDirectories(
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	for _, workType := range factoryConfig.WorkTypes {
		name := strings.TrimSpace(workType.Name)
		if name == "" {
			continue
		}
		channelDir := filepath.Join(
			targetDir,
			factorydefinitions.InputsDir,
			name,
			factorydefinitions.DefaultChannelName,
		)
		if err := w.fileSystem.MkdirAll(channelDir, 0o755); err != nil {
			return fmt.Errorf(
				"create inputs/%s/%s directory: %w",
				name,
				factorydefinitions.DefaultChannelName,
				err,
			)
		}
	}
	return nil
}

func (w *Writer) ensureCanonicalInputInboxSentinels(
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	const batchInputInboxChannelName = "BATCH"
	if err := w.ensureInbox.EnsureInputInboxGitkeep(
		targetDir,
		filepath.Join(
			factorydefinitions.InputsDir,
			batchInputInboxChannelName,
			factorydefinitions.DefaultChannelName,
			".gitkeep",
		),
	); err != nil {
		return fmt.Errorf("ensure batch inbox sentinel: %w", err)
	}
	for _, workType := range factoryConfig.WorkTypes {
		name := strings.TrimSpace(workType.Name)
		if name == "" {
			continue
		}
		if err := w.ensureInbox.EnsureInputInboxGitkeep(
			targetDir,
			filepath.Join(
				factorydefinitions.InputsDir,
				name,
				factorydefinitions.DefaultChannelName,
				".gitkeep",
			),
		); err != nil {
			return fmt.Errorf(
				"ensure inputs/%s/%s .gitkeep: %w",
				name,
				factorydefinitions.DefaultChannelName,
				err,
			)
		}
	}
	return nil
}
