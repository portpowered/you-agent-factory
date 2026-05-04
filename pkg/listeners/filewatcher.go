// Package listeners provides event listener implementations for the factory engine.
package listeners

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

// FileWatcher watches a directory for new .md and .json files and submits
// them to a Factory.
//
// Directory layout:
//
//	inputs/<work-type>/default/    — manual submissions
//	inputs/<work-type>/<exec-id>/  — executor-generated work
//
// The execution-id from the channel directory name is attached to wrapped work
// request items so downstream guards can correlate generated work with the
// parent execution.
type FileWatcher struct {
	dir     string
	factory factory.Factory
	logger  *zap.Logger
	// knownWorkTypes restricts submissions to known work types.
	// If nil, all subdirectories are accepted.
	knownWorkTypes  map[string]bool
	knownWorkStates map[string]map[string]bool
}

const batchInputDirectoryName = "BATCH"

// FileWatcherOption configures a FileWatcher.
type FileWatcherOption func(*FileWatcher)

// WithKnownWorkTypes restricts submissions to the given work type IDs.
// Subdirectories that don't match a known work type are logged and ignored.
func WithKnownWorkTypes(workTypes []string) FileWatcherOption {
	return func(fw *FileWatcher) {
		fw.knownWorkTypes = make(map[string]bool, len(workTypes))
		for _, wt := range workTypes {
			fw.knownWorkTypes[wt] = true
		}
	}
}

// WithKnownWorkStates enables boundary validation for explicit work-item states.
func WithKnownWorkStates(statesByType map[string]map[string]bool) FileWatcherOption {
	return func(fw *FileWatcher) {
		fw.knownWorkStates = statesByType
	}
}

// NewFileWatcher creates a FileWatcher that watches dir for new files.
func NewFileWatcher(dir string, f factory.Factory, logger *zap.Logger, opts ...FileWatcherOption) *FileWatcher {
	fw := &FileWatcher{
		dir:     dir,
		factory: f,
		logger:  logger,
	}
	for _, opt := range opts {
		opt(fw)
	}
	return fw
}

// Watch starts watching for file events. It blocks until ctx is cancelled.
func (fw *FileWatcher) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := fw.watchExistingDirs(watcher); err != nil {
		return err
	}

	fw.logger.Info("file watcher started",
		zap.String("dir", fw.dir))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&fsnotify.Create == 0 {
				continue
			}

			// If a new directory was created, start watching it.
			info, err := os.Stat(event.Name)
			if err != nil {
				fw.logger.Warn("failed to stat new file",
					zap.String("path", event.Name), zap.Error(err))
				continue
			}
			if info.IsDir() {
				if err := watcher.Add(event.Name); err != nil {
					fw.logger.Warn("failed to watch new directory",
						zap.String("path", event.Name), zap.Error(err))
				}
				continue
			}

			if err := fw.handleFile(ctx, event.Name); err != nil {
				fw.logger.Error("failed to handle file",
					zap.String("path", event.Name), zap.Error(err))
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fw.logger.Error("watcher error", zap.Error(err))
		}
	}
}

// PreseedInputs scans the watched directory for existing eligible files and
// submits them to the factory as canonical work request batches. It is
// intended to be called once at startup so that work items staged before the
// factory started are picked up automatically. If no eligible files are found,
// it is a no-op.
func (fw *FileWatcher) PreseedInputs(ctx context.Context) error {
	requests, err := fw.collectPreseedRequests()
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}

	if err := fw.validatePreseedRequests(requests); err != nil {
		return err
	}

	fw.logger.Info("preseeding factory with existing inputs", zap.Int("count", len(requests)))
	for _, request := range requests {
		if _, err := fw.factory.SubmitWorkRequest(ctx, request); err != nil {
			return err
		}
	}
	return nil
}

func (fw *FileWatcher) collectPreseedRequests() ([]interfaces.WorkRequest, error) {
	var batchRequests []interfaces.WorkRequest
	var fileWorks []interfaces.Work
	usedFileWorkNames := map[string]int{}

	err := filepath.WalkDir(fw.dir, func(path string, d fs.DirEntry, walkErr error) error {
		request, explicitBatch, ok, err := fw.preseedFileRequest(path, d, walkErr)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		if explicitBatch {
			batchRequests = append(batchRequests, request)
		} else if len(request.Works) == 1 {
			work := request.Works[0]
			work.Name = uniqueFileWorkName(work.Name, len(fileWorks), usedFileWorkNames)
			fileWorks = append(fileWorks, work)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("preseed walk: %w", err)
	}

	requests := make([]interfaces.WorkRequest, 0, len(batchRequests)+1)
	if len(fileWorks) > 0 {
		requests = append(requests, interfaces.WorkRequest{
			Type:  interfaces.WorkRequestTypeFactoryRequestBatch,
			Works: fileWorks,
		})
	}
	requests = append(requests, batchRequests...)
	return requests, nil
}

func (fw *FileWatcher) preseedFileRequest(path string, d fs.DirEntry, walkErr error) (interfaces.WorkRequest, bool, bool, error) {
	if walkErr != nil {
		fw.logger.Warn("preseed: skipping unreadable path",
			zap.String("path", path), zap.Error(walkErr))
		return interfaces.WorkRequest{}, false, false, nil
	}
	if d.IsDir() {
		return interfaces.WorkRequest{}, false, false, nil
	}

	name := filepath.Base(path)
	if isTempFile(name) {
		fw.logger.Debug("preseed: skipping temp file",
			zap.String("path", path))
		return interfaces.WorkRequest{}, false, false, nil
	}

	ext := strings.ToLower(filepath.Ext(name))
	if ext != JSON_EXTENSION && ext != MD_EXTENSION {
		fw.logger.Debug("preseed: skipping unsupported file type",
			zap.String("path", path), zap.String("extension", ext))
		return interfaces.WorkRequest{}, false, false, nil
	}

	workType, executionID, deriveErr := fw.deriveWorkTypeAndChannel(path)
	if deriveErr != nil {
		fw.logger.Warn("preseed: failed to derive work type",
			zap.String("path", path), zap.Error(deriveErr))
		return interfaces.WorkRequest{}, false, false, nil
	}
	if fw.knownWorkTypes != nil && workType != batchInputDirectoryName && !fw.knownWorkTypes[workType] {
		fw.logger.Warn("preseed: skipping unknown work type",
			zap.String("path", path), zap.String("work_type", workType))
		return interfaces.WorkRequest{}, false, false, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		fw.logger.Warn("preseed: skipping unreadable file",
			zap.String("path", path), zap.Error(err))
		return interfaces.WorkRequest{}, false, false, nil
	}

	request, explicitBatch, err := fileToWorkRequest(name, ext, workType, executionID, content)
	if err != nil {
		return interfaces.WorkRequest{}, false, false, fmt.Errorf("preseed parse %s: %w", path, err)
	}
	fw.logger.Info("preseed: found existing input",
		zap.String("path", path), zap.String("work_type", workType))
	return request, explicitBatch, true, nil
}

func (fw *FileWatcher) validatePreseedRequests(requests []interfaces.WorkRequest) error {
	for i, request := range requests {
		if _, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
			ValidWorkTypes:    fw.knownWorkTypes,
			ValidStatesByType: fw.knownWorkStates,
		}); err != nil {
			return fmt.Errorf("preseed validate request %d: %w", i, err)
		}
	}
	return nil
}

// watchExistingDirs adds the root and all existing subdirectories to the watcher,
// walking 2 levels deep (work-type then channel).
func (fw *FileWatcher) watchExistingDirs(watcher *fsnotify.Watcher) error {
	if err := watcher.Add(fw.dir); err != nil {
		return fmt.Errorf("watch %s: %w", fw.dir, err)
	}

	entries, err := os.ReadDir(fw.dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", fw.dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(fw.dir, entry.Name())
		if err := watcher.Add(subdir); err != nil {
			fw.logger.Warn("failed to watch subdirectory",
				zap.String("path", subdir), zap.Error(err))
			continue
		}

		// Also watch channel subdirectories.
		channelEntries, err := os.ReadDir(subdir)
		if err != nil {
			fw.logger.Warn("failed to read work-type subdirectory",
				zap.String("path", subdir), zap.Error(err))
			continue
		}
		for _, ch := range channelEntries {
			if ch.IsDir() {
				channelDir := filepath.Join(subdir, ch.Name())
				if err := watcher.Add(channelDir); err != nil {
					fw.logger.Warn("failed to watch channel directory",
						zap.String("path", channelDir), zap.Error(err))
				}
			}
		}
	}
	return nil
}

// isTempFile returns true if the filename looks like a temporary file.
func isTempFile(name string) bool {
	return strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, "~")
}

// handleFile processes a newly created file.
func (fw *FileWatcher) handleFile(ctx context.Context, path string) error {
	filename := filepath.Base(path)

	// Ignore temp files.
	if isTempFile(filename) {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != MD_EXTENSION && ext != JSON_EXTENSION {
		fw.logger.Warn("unsupported file type, ignoring", zap.String("filename", filename))
		return nil
	}

	workType, executionID, err := fw.deriveWorkTypeAndChannel(path)
	if err != nil {
		return fmt.Errorf("derive work type for %s: %w", path, err)
	}

	// Check against known work types if configured.
	if fw.knownWorkTypes != nil && workType != batchInputDirectoryName && !fw.knownWorkTypes[workType] {
		fw.logger.Warn("unknown work type subdirectory, ignoring",
			zap.String("dir", workType), zap.String("file", filename))
		return nil
	}

	fw.logger.Info("new input detected",
		zap.String("filename", filename),
		zap.String("work-type", workType),
		zap.String("execution-id", executionID))
	// fmt.Printf("new input detected: %s\n", filename)

	// Wait briefly for the file to be fully written. On Windows, fsnotify
	// fires CREATE before the writer has flushed all content.
	content, err := readFileWithRetry(path, 5, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}

	request, _, err := fileToWorkRequest(filename, ext, workType, executionID, content)
	if err != nil {
		return err
	}
	_, err = fw.factory.SubmitWorkRequest(ctx, request)
	return err
}

// deriveWorkTypeAndChannel extracts the work type and optional execution ID
// from a canonical watched input path:
//
//	<root>/<work-type>/<channel>/file      → workType, channel (or "" if "default")
func (fw *FileWatcher) deriveWorkTypeAndChannel(path string) (workType string, executionID string, err error) {
	targetPath, err := filepath.Rel(fw.dir, path)
	if err != nil {
		return "", "", fmt.Errorf("failed to get relative path for %s: %w", path, err)
	}

	parts := strings.Split(filepath.ToSlash(targetPath), "/")
	switch len(parts) {
	case 3:
		// <work-type>/<channel>/file — standard 3-level layout.
		workType = parts[0]
		if parts[1] != interfaces.DefaultChannelName {
			executionID = parts[1]
		}
		return workType, executionID, nil
	default:
		return "", "", fmt.Errorf("unexpected path depth (%d segments) for %s: expected <work-type>/<channel>/file", len(parts), targetPath)
	}
}

const (
	JSON_EXTENSION = ".json"
	MD_EXTENSION   = ".md"
)

func fileToWorkRequest(filename, ext, workType, executionID string, content []byte) (interfaces.WorkRequest, bool, error) {
	if ext == JSON_EXTENSION {
		var probe struct {
			Type interfaces.WorkRequestType `json:"type"`
		}
		if err := json.Unmarshal(content, &probe); err == nil && probe.Type == interfaces.WorkRequestTypeFactoryRequestBatch {
			workRequest, err := parseFactoryRequestBatch(content, workType, executionID)
			if err != nil {
				return interfaces.WorkRequest{}, false, err
			}
			return workRequest, true, nil
		}
	}

	return singleFileWorkRequest(filename, workType, executionID, content), false, nil
}

func parseFactoryRequestBatch(content []byte, workType string, executionID string) (interfaces.WorkRequest, error) {
	request, err := factory.ParseCanonicalWorkRequestJSON(content)
	if err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("parse work request batch: %w", err)
	}
	if request.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		return interfaces.WorkRequest{}, fmt.Errorf("work request batch has unsupported type %q", request.Type)
	}
	if err := applyInternalBatchWorkFields(&request, content); err != nil {
		return interfaces.WorkRequest{}, err
	}
	defaultWorkType := workType
	if workType == batchInputDirectoryName {
		defaultWorkType = ""
	}
	for i := range request.Works {
		if request.Works[i].WorkTypeID == "" && defaultWorkType != "" {
			request.Works[i].WorkTypeID = workType
		}
		if defaultWorkType != "" && request.Works[i].WorkTypeID != defaultWorkType {
			return interfaces.WorkRequest{}, fmt.Errorf("work request batch work %q has work_type_name %q that conflicts with watched work type %q", request.Works[i].Name, request.Works[i].WorkTypeID, workType)
		}
		if executionID != "" && request.Works[i].ExecutionID == "" {
			request.Works[i].ExecutionID = executionID
		}
	}
	if _, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{}); err != nil {
		return interfaces.WorkRequest{}, err
	}
	return request, nil
}

func applyInternalBatchWorkFields(request *interfaces.WorkRequest, content []byte) error {
	var raw struct {
		Works []struct {
			ExecutionID      string                `json:"execution_id"`
			RuntimeRelations []interfaces.Relation `json:"runtime_relations"`
		} `json:"works"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		return fmt.Errorf("parse work request batch internal fields: %w", err)
	}
	for i := range raw.Works {
		if i >= len(request.Works) {
			break
		}
		if raw.Works[i].ExecutionID != "" {
			request.Works[i].ExecutionID = raw.Works[i].ExecutionID
		}
		if len(raw.Works[i].RuntimeRelations) > 0 {
			request.Works[i].RuntimeRelations = append([]interfaces.Relation(nil), raw.Works[i].RuntimeRelations...)
		}
	}
	return nil
}

func singleFileWorkRequest(filename string, workType string, executionID string, content []byte) interfaces.WorkRequest {
	return interfaces.WorkRequest{
		Type: interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:        strings.TrimSuffix(filename, filepath.Ext(filename)),
			WorkTypeID:  workType,
			Payload:     append([]byte(nil), content...),
			ExecutionID: executionID,
		}},
	}
}

func uniqueFileWorkName(name string, index int, used map[string]int) string {
	base := name
	if base == "" {
		base = "work-" + strconv.Itoa(index+1)
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

// readFileWithRetry reads a file, retrying if the content is empty.
// This handles the race where fsnotify fires CREATE before the writer
// has finished flushing the file content (common on Windows).
func readFileWithRetry(path string, maxRetries int, delay time.Duration) ([]byte, error) {
	var content []byte
	var err error
	for i := range maxRetries {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(content) > 0 {
			return content, nil
		}
		if i < maxRetries-1 {
			time.Sleep(delay)
		}
	}
	return content, nil
}
