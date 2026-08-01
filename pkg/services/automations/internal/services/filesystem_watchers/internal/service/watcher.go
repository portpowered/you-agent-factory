package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// watcher watches a directory for new .md and .json files and submits Work
// Requests through the injected admission collaborator.
//
// Directory layout:
//
//	inputs/<work-type>/default/    — manual submissions
//	inputs/<work-type>/<exec-id>/  — executor-generated work
//
// The execution-id from the channel directory name is attached to wrapped work
// request items so downstream guards can correlate generated work with the
// parent execution.
type watcher struct {
	dir    string
	submit automations.WorkRequestSubmitter
	logger *zap.Logger
	// knownWorkTypes restricts submissions to known work types.
	// If nil, all subdirectories are accepted.
	knownWorkTypes      map[string]bool
	knownWorkStates     map[string]map[string]bool
	files               inputFileSystem
	walkDirectory       directoryWalker
	workRequestIDs      work.RequestIDGenerator
	newWatcher          fileEventWatcherFactory
	clock               clockwork.Clock
	debounceWindow      time.Duration
	handledIdentities   handledIdentities
	lazyHandledIdentity *memoryHandledIdentities
}

type fileEventWatcher interface {
	Add(string) error
	Close() error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
}

type fileEventWatcherFactory func() (fileEventWatcher, error)

type fsnotifyEventWatcher struct {
	watcher *fsnotify.Watcher
}

func (w fsnotifyEventWatcher) Add(path string) error         { return w.watcher.Add(path) }
func (w fsnotifyEventWatcher) Close() error                  { return w.watcher.Close() }
func (w fsnotifyEventWatcher) Events() <-chan fsnotify.Event { return w.watcher.Events }
func (w fsnotifyEventWatcher) Errors() <-chan error          { return w.watcher.Errors }

func newFSNotifyEventWatcher() (fileEventWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return fsnotifyEventWatcher{watcher: watcher}, nil
}

const batchInputDirectoryName = "BATCH"

type inputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

type directoryWalker func(string, fs.WalkDirFunc) error

type handledIdentities interface {
	Contains(filesystemwatchers.ObservationIdentity) bool
	Record(filesystemwatchers.ObservationIdentity) error
}

func newWatcher(config filesystemwatchers.Config) *watcher {
	return newWatcherWithClockAndHandled(config, clockwork.NewRealClock(), newMemoryHandledIdentities())
}

func newWatcherWithClock(config filesystemwatchers.Config, clock clockwork.Clock) *watcher {
	return newWatcherWithClockAndHandled(config, clock, newMemoryHandledIdentities())
}

func newWatcherWithHandled(config filesystemwatchers.Config, handled handledIdentities) *watcher {
	return newWatcherWithClockAndHandled(config, clockwork.NewRealClock(), handled)
}

func newWatcherWithClockAndHandled(
	config filesystemwatchers.Config,
	clock clockwork.Clock,
	handled handledIdentities,
) *watcher {
	if config.Files == nil {
		panic("filesystem watcher input filesystem is required")
	}
	if config.WalkDirectory == nil {
		panic("filesystem watcher directory walker is required")
	}
	if config.WorkRequestIDs == nil {
		panic("Work Request ID generator is required")
	}
	if config.Submitter == nil {
		panic("filesystem watcher Work Request submitter is required")
	}
	var knownWorkTypes map[string]bool
	if config.KnownWorkTypes != nil {
		knownWorkTypes = make(map[string]bool, len(config.KnownWorkTypes))
		for _, workType := range config.KnownWorkTypes {
			knownWorkTypes[workType] = true
		}
	}
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	debounceWindow := config.DebounceWindow
	if debounceWindow <= 0 {
		debounceWindow = defaultDebounceWindow
	}
	return &watcher{
		dir:               config.Dir,
		submit:            config.Submitter,
		logger:            config.Logger,
		knownWorkTypes:    knownWorkTypes,
		knownWorkStates:   config.ValidStatesByType,
		files:             config.Files,
		walkDirectory:     directoryWalker(config.WalkDirectory),
		workRequestIDs:    config.WorkRequestIDs,
		newWatcher:        newFSNotifyEventWatcher,
		clock:             clock,
		debounceWindow:    debounceWindow,
		handledIdentities: handled,
	}
}

// Watch starts watching for file events. It blocks until ctx is cancelled.
func (fw *watcher) Watch(ctx context.Context) error {
	watcher, err := fw.newWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := fw.watchExistingDirs(watcher); err != nil {
		return err
	}

	fw.logger.Info("file watcher started",
		zap.String("dir", fw.dir))

	scheduler := newDebounceScheduler(fw.clock, fw.debounceWindow)
	defer scheduler.cancelAll()

	for {
		select {
		case <-ctx.Done():
			scheduler.cancelAll()
			return ctx.Err()
		case event, ok := <-watcher.Events():
			if !ok {
				return nil
			}
			if !isWatchedFileEvent(event.Op) {
				continue
			}

			// If a new directory was created, start watching it.
			info, err := fw.files.Stat(event.Name)
			if err != nil {
				fw.logger.Warn("failed to stat new file",
					zap.String("path", event.Name), zap.Error(err))
				continue
			}
			if info.IsDir() {
				if event.Op&fsnotify.Create != 0 {
					if err := watcher.Add(event.Name); err != nil {
						fw.logger.Warn("failed to watch new directory",
							zap.String("path", event.Name), zap.Error(err))
					}
				}
				continue
			}

			path := event.Name
			scheduler.schedule(path, func() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if err := fw.handleFile(ctx, path); err != nil {
					fw.logger.Error("failed to handle file",
						zap.String("path", path), zap.Error(err))
				}
			})
		case err, ok := <-watcher.Errors():
			if !ok {
				return nil
			}
			fw.logger.Error("watcher error", zap.Error(err))
		}
	}
}

func (fw *watcher) handledIdentityStore() handledIdentities {
	if fw.handledIdentities != nil {
		return fw.handledIdentities
	}
	if fw.lazyHandledIdentity == nil {
		fw.lazyHandledIdentity = newMemoryHandledIdentities()
	}
	return fw.lazyHandledIdentity
}

func (fw *watcher) recordHandledPath(path string) error {
	identity, err := observationIdentity(fw.dir, path)
	if err != nil {
		return err
	}
	return fw.handledIdentityStore().Record(identity)
}

// PreseedInputs scans the watched directory for existing eligible files and
// submits them to the factory as canonical work request batches. It is
// intended to be called once at startup so that work items staged before the
// factory started are picked up automatically. If no eligible files are found,
// it is a no-op.
func (fw *watcher) PreseedInputs(ctx context.Context) error {
	requests, paths, err := fw.collectPreseedRequests()
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
		if err := fw.submit(ctx, request); err != nil {
			return err
		}
	}
	for _, path := range paths {
		if err := fw.recordHandledPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (fw *watcher) collectPreseedRequests() ([]work.WorkRequest, []string, error) {
	var batchRequests []work.WorkRequest
	var fileWorks []work.Work
	var handledPaths []string
	usedFileWorkNames := map[string]int{}

	err := fw.walkDirectory(fw.dir, func(path string, d fs.DirEntry, walkErr error) error {
		request, explicitBatch, ok, err := fw.preseedFileRequest(path, d, walkErr)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		handledPaths = append(handledPaths, path)
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
		return nil, nil, fmt.Errorf("preseed walk: %w", err)
	}

	requests := make([]work.WorkRequest, 0, len(batchRequests)+1)
	if len(fileWorks) > 0 {
		requests = append(requests, work.WorkRequest{
			Type:  work.WorkRequestTypeFactoryRequestBatch,
			Works: fileWorks,
		})
	}
	requests = append(requests, batchRequests...)
	return requests, handledPaths, nil
}

func (fw *watcher) preseedFileRequest(path string, d fs.DirEntry, walkErr error) (work.WorkRequest, bool, bool, error) {
	if walkErr != nil {
		fw.logger.Warn("preseed: skipping unreadable path",
			zap.String("path", path), zap.Error(walkErr))
		return work.WorkRequest{}, false, false, nil
	}
	if d.IsDir() {
		return work.WorkRequest{}, false, false, nil
	}

	name := filepath.Base(path)
	if isTempFile(name) {
		fw.logger.Debug("preseed: skipping temp file",
			zap.String("path", path))
		return work.WorkRequest{}, false, false, nil
	}

	ext := strings.ToLower(filepath.Ext(name))
	if ext != JSON_EXTENSION && ext != MD_EXTENSION {
		fw.logger.Debug("preseed: skipping unsupported file type",
			zap.String("path", path), zap.String("extension", ext))
		return work.WorkRequest{}, false, false, nil
	}

	workType, executionID, deriveErr := fw.deriveWorkTypeAndChannel(path)
	if deriveErr != nil {
		fw.logger.Warn("preseed: failed to derive work type",
			zap.String("path", path), zap.Error(deriveErr))
		return work.WorkRequest{}, false, false, nil
	}
	identity, identityErr := observationIdentity(fw.dir, path)
	if identityErr != nil {
		return work.WorkRequest{}, false, false, identityErr
	}
	if fw.handledIdentityStore().Contains(identity) {
		fw.logger.Debug("preseed: skipping already-handled input",
			zap.String("path", path),
			zap.String("identity", string(identity)))
		return work.WorkRequest{}, false, false, nil
	}
	if fw.knownWorkTypes != nil && workType != batchInputDirectoryName && !fw.knownWorkTypes[workType] {
		fw.logger.Warn("preseed: skipping unknown work type",
			zap.String("path", path), zap.String("work_type", workType))
		return work.WorkRequest{}, false, false, nil
	}

	content, err := fw.files.ReadFile(path)
	if err != nil {
		fw.logger.Warn("preseed: skipping unreadable file",
			zap.String("path", path), zap.Error(err))
		return work.WorkRequest{}, false, false, nil
	}

	request, explicitBatch, err := fileToWorkRequest(name, ext, workType, executionID, content, fw.workRequestIDs)
	if err != nil {
		return work.WorkRequest{}, false, false, fmt.Errorf("preseed parse %s: %w", path, err)
	}
	fw.logger.Info("preseed: found existing input",
		zap.String("path", path), zap.String("work_type", workType))
	return request, explicitBatch, true, nil
}

func (fw *watcher) validatePreseedRequests(workRequests []work.WorkRequest) error {
	for i, request := range workRequests {
		if _, err := work.NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{
			ValidWorkTypes:    fw.knownWorkTypes,
			ValidStatesByType: fw.knownWorkStates,
			IDGenerator:       fw.workRequestIDs,
		}); err != nil {
			return fmt.Errorf("preseed validate request %d: %w", i, err)
		}
	}
	return nil
}

// watchExistingDirs adds the root and all existing subdirectories to the watcher,
// walking 2 levels deep (work-type then channel).
func (fw *watcher) watchExistingDirs(watcher fileEventWatcher) error {
	if err := watcher.Add(fw.dir); err != nil {
		return fmt.Errorf("watch %s: %w", fw.dir, err)
	}

	entries, err := fw.files.ReadDir(fw.dir)
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
		channelEntries, err := fw.files.ReadDir(subdir)
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

func isWatchedFileEvent(op fsnotify.Op) bool {
	return op&(fsnotify.Create|fsnotify.Write) != 0
}

// isTempFile returns true if the filename looks like a temporary file.
func isTempFile(name string) bool {
	return strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, "~")
}

// handleFile processes a newly created file.
func (fw *watcher) handleFile(ctx context.Context, path string) error {
	identity, err := observationIdentity(fw.dir, path)
	if err != nil {
		return err
	}
	if fw.handledIdentityStore().Contains(identity) {
		fw.logger.Debug("skipping duplicate filesystem observation",
			zap.String("path", path),
			zap.String("identity", string(identity)))
		return nil
	}

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
	content, err := fw.readFileWithRetry(path, 5, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}

	request, _, err := fileToWorkRequest(filename, ext, workType, executionID, content, fw.workRequestIDs)
	if err != nil {
		return err
	}
	if err := fw.submit(ctx, request); err != nil {
		return err
	}
	return fw.recordHandledPath(path)
}

// deriveWorkTypeAndChannel extracts the work type and optional execution ID
// from a canonical watched input path:
//
//	<root>/<work-type>/<channel>/file      → workType, channel (or "" if "default")
func (fw *watcher) deriveWorkTypeAndChannel(path string) (workType string, executionID string, err error) {
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

func fileToWorkRequest(filename, ext, workType, executionID string, content []byte, generateID work.RequestIDGenerator) (work.WorkRequest, bool, error) {
	if ext == JSON_EXTENSION {
		var probe struct {
			Type work.WorkRequestType `json:"type"`
		}
		if err := json.Unmarshal(content, &probe); err == nil && probe.Type == work.WorkRequestTypeFactoryRequestBatch {
			workRequest, err := parseFactoryRequestBatch(content, workType, executionID, generateID)
			if err != nil {
				return work.WorkRequest{}, false, err
			}
			return workRequest, true, nil
		}
	}

	return singleFileWorkRequest(filename, workType, executionID, content), false, nil
}

func parseFactoryRequestBatch(content []byte, workType string, executionID string, generateID work.RequestIDGenerator) (work.WorkRequest, error) {
	request, err := work.ParseCanonicalWorkRequestJSON(content)
	if err != nil {
		return work.WorkRequest{}, fmt.Errorf("parse work request batch: %w", err)
	}
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		return work.WorkRequest{}, fmt.Errorf("work request batch has unsupported type %q", request.Type)
	}
	if err := applyInternalBatchWorkFields(&request, content); err != nil {
		return work.WorkRequest{}, err
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
			return work.WorkRequest{}, fmt.Errorf("work request batch work %q has work_type_name %q that conflicts with watched work type %q", request.Works[i].Name, request.Works[i].WorkTypeID, workType)
		}
		if executionID != "" && request.Works[i].ExecutionID == "" {
			request.Works[i].ExecutionID = executionID
		}
	}
	if _, err := work.NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{IDGenerator: generateID}); err != nil {
		return work.WorkRequest{}, err
	}
	return request, nil
}

func applyInternalBatchWorkFields(request *work.WorkRequest, content []byte) error {
	var raw struct {
		Works []struct {
			ExecutionID      string          `json:"execution_id"`
			RuntimeRelations []work.Relation `json:"runtime_relations"`
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
			request.Works[i].RuntimeRelations = append([]work.Relation(nil), raw.Works[i].RuntimeRelations...)
		}
	}
	return nil
}

func singleFileWorkRequest(filename string, workType string, executionID string, content []byte) work.WorkRequest {
	return work.WorkRequest{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
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
func (fw *watcher) readFileWithRetry(path string, maxRetries int, delay time.Duration) ([]byte, error) {
	var content []byte
	var err error
	for i := range maxRetries {
		content, err = fw.files.ReadFile(path)
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
