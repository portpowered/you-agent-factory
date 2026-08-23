package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	runtimeMetricsDateLayout = "2006/01/02"
	runtimeMetricsTimeLayout = "150405.000000000"
)

// RuntimeMetricsRetentionFileSystem is the exact filesystem effect required by
// the root retention policy. Lstat is intentional: a recognized symlink is
// protected rather than followed or removed through its target.
type RuntimeMetricsRetentionFileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	WalkDir(string, fs.WalkDirFunc) error
	ReadDir(string) ([]fs.DirEntry, error)
	Remove(string) error
}

// RuntimeMetricsRetentionRequest selects one root-level retention operation.
// MaxBackups remains a per-filename rolling-writer policy; the root sweep
// applies MaxAge and MaxSize to all recognized metrics artifacts together.
type RuntimeMetricsRetentionRequest struct {
	RootDirectory string
	Config        RuntimeMetricsConfig
}

// RuntimeMetricsRetentionTotals reports a file count and aggregate byte size.
type RuntimeMetricsRetentionTotals struct {
	Files int
	Bytes int64
}

// RuntimeMetricsRetentionFailure identifies one artifact that could not be
// safely inspected or removed. The path is always beneath the selected root.
type RuntimeMetricsRetentionFailure struct {
	Path  string
	Error error
}

// RuntimeMetricsRetentionReport describes one deterministic root sweep.
// Before and After count recognized regular metrics artifacts only. Protected
// and Failed count artifacts that were not safe to remove; unrecognized files
// are deliberately absent from every total.
type RuntimeMetricsRetentionReport struct {
	RootDirectory string
	Skipped       bool
	Before        RuntimeMetricsRetentionTotals
	After         RuntimeMetricsRetentionTotals
	Scanned       RuntimeMetricsRetentionTotals
	Removed       RuntimeMetricsRetentionTotals
	Protected     RuntimeMetricsRetentionTotals
	Failed        RuntimeMetricsRetentionTotals
	Failures      []RuntimeMetricsRetentionFailure
}

// RuntimeMetricsRetention owns root-level metrics cleanup. Its clock and
// filesystem are injected so age and ordering remain deterministic in tests.
type RuntimeMetricsRetention struct {
	filesystem   RuntimeMetricsRetentionFileSystem
	now          func() time.Time
	coordination RuntimeMetricsCoordination
}

// NewRuntimeMetricsRetention constructs the root-level metrics retention
// policy from the exact filesystem and clock selected by the composition root.
func NewRuntimeMetricsRetention(
	filesystem RuntimeMetricsRetentionFileSystem,
	now func() time.Time,
	coordination ...RuntimeMetricsCoordination,
) (*RuntimeMetricsRetention, error) {
	if filesystem == nil {
		return nil, errors.New("construct runtime metrics retention: filesystem is required")
	}
	if now == nil {
		return nil, errors.New("construct runtime metrics retention: clock is required")
	}
	selectedCoordination, err := selectRuntimeMetricsCoordination(coordination)
	if err != nil {
		return nil, fmt.Errorf("construct runtime metrics retention: %w", err)
	}
	return &RuntimeMetricsRetention{
		filesystem: filesystem, now: now, coordination: selectedCoordination,
	}, nil
}

// Sweep removes safe, recognized metrics artifacts beneath the selected root.
// Age pruning runs before aggregate-size pruning. Complete eligible date
// directories are preferred, while directories containing unknown or
// protected entries fall back to independently safe file candidates.
func (retention *RuntimeMetricsRetention) Sweep(
	ctx context.Context,
	request RuntimeMetricsRetentionRequest,
) (report RuntimeMetricsRetentionReport, returnErr error) {
	if retention == nil || retention.filesystem == nil || retention.now == nil || retention.coordination == nil {
		return report, errors.New("sweep runtime metrics: retention is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("sweep runtime metrics: %w", err)
	}
	preparation, err := retention.prepareSweep(ctx, request)
	if err != nil {
		return report, err
	}
	report.RootDirectory = preparation.root
	if preparation.skipped {
		report.Skipped = true
		return report, nil
	}
	defer func() {
		if closeErr := preparation.rootLock.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("release runtime metrics sweep coordination: %w", closeErr))
		}
	}()
	if err := retention.sweepLocked(ctx, request, preparation, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (retention *RuntimeMetricsRetention) sweepLocked(
	ctx context.Context,
	request RuntimeMetricsRetentionRequest,
	preparation retentionSweepPreparation,
	report *RuntimeMetricsRetentionReport,
) error {
	config := normalizeRuntimeMetricsConfig(request.Config)
	initial, err := retention.inventory(ctx, preparation.root)
	if err != nil {
		return err
	}
	report.Before = retentionTotals(initial.artifacts)
	report.Scanned = report.Before
	state := newRetentionSweepState(preparation.root, initial.artifacts, report)
	state.addInventoryOutcomes(initial)
	if err := retention.pruneAge(ctx, state, preparation.now, config.MaxAge); err != nil {
		return err
	}
	maxBytes := int64(config.MaxSize) * 1024 * 1024
	if err := retention.prune(ctx, state, func(artifact retentionArtifact) bool {
		return artifact.hasTimestamp
	}, true, maxBytes); err != nil {
		return err
	}
	after, err := retention.inventory(ctx, preparation.root)
	if err != nil {
		return err
	}
	report.After = retentionTotals(after.artifacts)
	return nil
}

func (retention *RuntimeMetricsRetention) pruneAge(
	ctx context.Context,
	state *retentionSweepState,
	now time.Time,
	maxAge int,
) error {
	if maxAge <= 0 {
		return nil
	}
	cutoff := now.Add(-time.Duration(maxAge) * 24 * time.Hour)
	return retention.prune(ctx, state, func(artifact retentionArtifact) bool {
		return artifact.hasTimestamp && artifact.timestamp.Before(cutoff)
	}, false, 0)
}

type retentionSweepPreparation struct {
	root     string
	now      time.Time
	rootLock io.Closer
	skipped  bool
}

func (retention *RuntimeMetricsRetention) prepareSweep(
	ctx context.Context,
	request RuntimeMetricsRetentionRequest,
) (retentionSweepPreparation, error) {
	root := filepath.Clean(strings.TrimSpace(request.RootDirectory))
	if root == "." || strings.TrimSpace(request.RootDirectory) == "" {
		return retentionSweepPreparation{}, errors.New("sweep runtime metrics: root is required")
	}
	rootInfo, err := retention.filesystem.Lstat(root)
	if err != nil {
		return retentionSweepPreparation{}, fmt.Errorf("sweep runtime metrics root %q: %w", root, err)
	}
	if rootInfo.Mode()&fs.ModeSymlink != 0 || !rootInfo.IsDir() {
		return retentionSweepPreparation{}, fmt.Errorf("sweep runtime metrics root %q: not a directory", root)
	}
	now := retention.now().UTC()
	if now.IsZero() {
		return retentionSweepPreparation{}, errors.New("sweep runtime metrics: clock returned zero time")
	}
	rootLock, err := retention.coordination.TryLockRoot(root)
	if errors.Is(err, ErrRuntimeMetricsRootBusy) {
		return retentionSweepPreparation{root: root, now: now, skipped: true}, nil
	}
	if err != nil {
		return retentionSweepPreparation{}, fmt.Errorf("coordinate runtime metrics sweep: %w", err)
	}
	return retentionSweepPreparation{root: root, now: now, rootLock: rootLock}, nil
}

type retentionArtifact struct {
	path          string
	size          int64
	timestamp     time.Time
	dateDirectory string
	hasTimestamp  bool
}

type retentionOutcome struct {
	path string
	size int64
	err  error
}

type retentionInventory struct {
	artifacts map[string]retentionArtifact
	protected []retentionOutcome
	failed    []retentionOutcome
}

func (retention *RuntimeMetricsRetention) inventory(
	ctx context.Context,
	root string,
) (retentionInventory, error) {
	inventory := retentionInventory{artifacts: make(map[string]retentionArtifact)}
	err := retention.filesystem.WalkDir(root, func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if entry != nil && isRuntimeMetricsArtifact(entry.Name()) {
				inventory.failed = append(inventory.failed, retentionOutcome{path: path, err: walkErr})
			}
			return nil
		}
		if entry == nil || entry.IsDir() || !isRuntimeMetricsArtifact(entry.Name()) {
			return nil
		}
		info, err := retention.filesystem.Lstat(path)
		if err != nil {
			inventory.failed = append(inventory.failed, retentionOutcome{path: path, err: err})
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			inventory.protected = append(inventory.protected, retentionOutcome{path: path, size: info.Size()})
			return nil
		}
		timestamp, dateDirectory, hasTimestamp := runtimeMetricsArtifactTimestamp(root, path)
		inventory.artifacts[path] = retentionArtifact{
			path: path, size: info.Size(), timestamp: timestamp,
			dateDirectory: dateDirectory, hasTimestamp: hasTimestamp,
		}
		if !hasTimestamp {
			inventory.protected = append(inventory.protected, retentionOutcome{path: path, size: info.Size()})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inventory, fmt.Errorf("inventory runtime metrics under %q: %w", root, err)
		}
		return inventory, fmt.Errorf("inventory runtime metrics under %q: %w", root, err)
	}
	return inventory, nil
}

func runtimeMetricsArtifactTimestamp(root, path string) (time.Time, string, bool) {
	if !pathWithinRoot(root, path) {
		return time.Time{}, "", false
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || filepath.IsAbs(relative) {
		return time.Time{}, "", false
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	if len(parts) != 4 {
		return time.Time{}, "", false
	}
	date, err := time.Parse(runtimeMetricsDateLayout, strings.Join(parts[:3], "/"))
	if err != nil || len(parts[3]) < len(runtimeMetricsTimeLayout) {
		return time.Time{}, "", false
	}
	clock, err := time.Parse(runtimeMetricsTimeLayout, parts[3][:len(runtimeMetricsTimeLayout)])
	if err != nil {
		return time.Time{}, "", false
	}
	timestamp := time.Date(
		date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(),
		clock.Second(), clock.Nanosecond(), time.UTC,
	)
	return timestamp, filepath.Join(filepath.Clean(root), parts[0], parts[1], parts[2]), true
}

func pathWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || filepath.IsAbs(relative) || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

type retentionSweepState struct {
	root      string
	remaining map[string]retentionArtifact
	blocked   map[string]struct{}
	protected map[string]struct{}
	failed    map[string]struct{}
	removed   map[string]struct{}
	report    *RuntimeMetricsRetentionReport
}

func newRetentionSweepState(
	root string,
	artifacts map[string]retentionArtifact,
	report *RuntimeMetricsRetentionReport,
) *retentionSweepState {
	remaining := make(map[string]retentionArtifact, len(artifacts))
	for path, artifact := range artifacts {
		remaining[path] = artifact
	}
	return &retentionSweepState{
		root: root, remaining: remaining,
		blocked:   make(map[string]struct{}),
		protected: make(map[string]struct{}),
		failed:    make(map[string]struct{}),
		removed:   make(map[string]struct{}),
		report:    report,
	}
}

func (state *retentionSweepState) addInventoryOutcomes(inventory retentionInventory) {
	for _, outcome := range inventory.protected {
		state.addProtected(outcome.path, outcome.size)
		state.blocked[outcome.path] = struct{}{}
	}
	for _, outcome := range inventory.failed {
		state.addFailed(outcome.path, outcome.size, outcome.err)
		state.blocked[outcome.path] = struct{}{}
	}
}

func (state *retentionSweepState) addProtected(path string, size int64) {
	if _, exists := state.protected[path]; exists {
		return
	}
	if _, exists := state.failed[path]; exists {
		return
	}
	state.protected[path] = struct{}{}
	state.report.Protected.Files++
	state.report.Protected.Bytes += size
}

func (state *retentionSweepState) addFailed(path string, size int64, err error) {
	if _, exists := state.failed[path]; exists {
		return
	}
	if _, exists := state.protected[path]; exists {
		return
	}
	state.failed[path] = struct{}{}
	state.report.Failed.Files++
	state.report.Failed.Bytes += size
	state.report.Failures = append(state.report.Failures, RuntimeMetricsRetentionFailure{Path: path, Error: err})
}

func (state *retentionSweepState) markRemoved(artifact retentionArtifact) {
	if _, exists := state.removed[artifact.path]; exists {
		return
	}
	delete(state.remaining, artifact.path)
	state.removed[artifact.path] = struct{}{}
	state.report.Removed.Files++
	state.report.Removed.Bytes += artifact.size
}

func (state *retentionSweepState) bytes() int64 {
	var total int64
	for _, artifact := range state.remaining {
		total += artifact.size
	}
	return total
}

func (retention *RuntimeMetricsRetention) prune(
	ctx context.Context,
	state *retentionSweepState,
	eligible func(retentionArtifact) bool,
	stopAtBudget bool,
	maxBytes int64,
) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("prune runtime metrics: %w", err)
		}
		if stopAtBudget && state.bytes() <= maxBytes {
			return nil
		}
		group := retention.oldestCompleteDateDirectory(state, eligible)
		if len(group) > 0 {
			for _, artifact := range group {
				retention.tryRemove(state, artifact)
			}
			if allDateArtifactsRemoved(state, group) {
				_ = retention.filesystem.Remove(group[0].dateDirectory)
			}
			continue
		}
		candidate, found := oldestEligibleArtifact(state, eligible)
		if !found {
			return nil
		}
		retention.tryRemove(state, candidate)
	}
}

func (retention *RuntimeMetricsRetention) oldestCompleteDateDirectory(
	state *retentionSweepState,
	eligible func(retentionArtifact) bool,
) []retentionArtifact {
	type dateGroup struct {
		directory string
		artifacts []retentionArtifact
		oldest    time.Time
	}
	groups := make(map[string]dateGroup)
	for _, artifact := range state.remaining {
		if !artifact.hasTimestamp || artifact.dateDirectory == "" || !eligible(artifact) {
			continue
		}
		if _, blocked := state.blocked[artifact.path]; blocked {
			continue
		}
		group := groups[artifact.dateDirectory]
		group.directory = artifact.dateDirectory
		group.artifacts = append(group.artifacts, artifact)
		if group.oldest.IsZero() || artifact.timestamp.Before(group.oldest) {
			group.oldest = artifact.timestamp
		}
		groups[artifact.dateDirectory] = group
	}
	complete := make([]dateGroup, 0, len(groups))
	for _, group := range groups {
		if retention.completeEligibleDateDirectory(state, group, eligible) {
			complete = append(complete, group)
		}
	}
	if len(complete) == 0 {
		return nil
	}
	sort.Slice(complete, func(i, j int) bool {
		if !complete[i].oldest.Equal(complete[j].oldest) {
			return complete[i].oldest.Before(complete[j].oldest)
		}
		return complete[i].directory < complete[j].directory
	})
	sort.Slice(complete[0].artifacts, func(i, j int) bool {
		return retentionArtifactLess(complete[0].artifacts[i], complete[0].artifacts[j])
	})
	return complete[0].artifacts
}

func (retention *RuntimeMetricsRetention) completeEligibleDateDirectory(
	state *retentionSweepState,
	group struct {
		directory string
		artifacts []retentionArtifact
		oldest    time.Time
	},
	eligible func(retentionArtifact) bool,
) bool {
	entries, err := retention.filesystem.ReadDir(group.directory)
	if err != nil || len(entries) == 0 {
		return false
	}
	stateCount := 0
	for _, artifact := range state.remaining {
		if artifact.dateDirectory == group.directory {
			stateCount++
		}
	}
	if stateCount != len(entries) {
		return false
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		path, ok := retention.completeDateEntry(state, group, eligible, entry)
		if !ok {
			return false
		}
		seen[path] = struct{}{}
	}
	return len(seen) == len(group.artifacts)
}

func (retention *RuntimeMetricsRetention) completeDateEntry(
	state *retentionSweepState,
	group struct {
		directory string
		artifacts []retentionArtifact
		oldest    time.Time
	},
	eligible func(retentionArtifact) bool,
	entry fs.DirEntry,
) (string, bool) {
	if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 || !isRuntimeMetricsArtifact(entry.Name()) {
		return "", false
	}
	path := filepath.Join(group.directory, entry.Name())
	if !pathWithinRoot(state.root, path) {
		return "", false
	}
	artifact, ok := state.remaining[path]
	if !ok || !eligible(artifact) {
		return "", false
	}
	info, err := retention.filesystem.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}

func allDateArtifactsRemoved(state *retentionSweepState, group []retentionArtifact) bool {
	for _, artifact := range group {
		if _, exists := state.remaining[artifact.path]; exists {
			return false
		}
	}
	return true
}

func oldestEligibleArtifact(
	state *retentionSweepState,
	eligible func(retentionArtifact) bool,
) (retentionArtifact, bool) {
	candidates := make([]retentionArtifact, 0)
	for _, artifact := range state.remaining {
		if !artifact.hasTimestamp || !eligible(artifact) {
			continue
		}
		if _, blocked := state.blocked[artifact.path]; blocked {
			continue
		}
		candidates = append(candidates, artifact)
	}
	if len(candidates) == 0 {
		return retentionArtifact{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return retentionArtifactLess(candidates[i], candidates[j])
	})
	return candidates[0], true
}

func retentionArtifactLess(left, right retentionArtifact) bool {
	if !left.timestamp.Equal(right.timestamp) {
		return left.timestamp.Before(right.timestamp)
	}
	return left.path < right.path
}

func (retention *RuntimeMetricsRetention) tryRemove(
	state *retentionSweepState,
	artifact retentionArtifact,
) {
	initialSize, ok := retention.inspectRemovalCandidate(state, artifact)
	if !ok {
		return
	}
	claim, currentInfo, ok := retention.claimRemovalCandidate(state, artifact, initialSize)
	if !ok {
		return
	}
	defer func() { _ = claim.Close() }()
	if currentInfo.Size() != artifact.size {
		retention.protectChangedRemovalCandidate(state, artifact, currentInfo.Size())
		return
	}
	retention.removeCandidate(state, artifact)
}

func (retention *RuntimeMetricsRetention) inspectRemovalCandidate(
	state *retentionSweepState,
	artifact retentionArtifact,
) (int64, bool) {
	if _, blocked := state.blocked[artifact.path]; blocked {
		return 0, false
	}
	if !pathWithinRoot(state.root, artifact.path) {
		state.addProtected(artifact.path, artifact.size)
		state.blocked[artifact.path] = struct{}{}
		return 0, false
	}
	info, err := retention.filesystem.Lstat(artifact.path)
	if errors.Is(err, fs.ErrNotExist) {
		state.markRemoved(artifact)
		return 0, false
	}
	if err != nil {
		state.addFailed(artifact.path, artifact.size, err)
		state.blocked[artifact.path] = struct{}{}
		return 0, false
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		state.addProtected(artifact.path, info.Size())
		state.blocked[artifact.path] = struct{}{}
		return 0, false
	}
	return info.Size(), true
}

func (retention *RuntimeMetricsRetention) claimRemovalCandidate(
	state *retentionSweepState,
	artifact retentionArtifact,
	initialSize int64,
) (io.Closer, fs.FileInfo, bool) {
	claim, err := retention.coordination.TryClaim(artifact.path)
	if errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
		state.addProtected(artifact.path, initialSize)
		state.blocked[artifact.path] = struct{}{}
		return nil, nil, false
	}
	if err != nil {
		state.addFailed(artifact.path, initialSize, err)
		state.blocked[artifact.path] = struct{}{}
		return nil, nil, false
	}
	currentInfo, err := retention.filesystem.Lstat(artifact.path)
	if errors.Is(err, fs.ErrNotExist) {
		_ = claim.Close()
		state.markRemoved(artifact)
		return nil, nil, false
	}
	if err != nil {
		_ = claim.Close()
		state.addFailed(artifact.path, initialSize, err)
		state.blocked[artifact.path] = struct{}{}
		return nil, nil, false
	}
	if currentInfo.Mode()&fs.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() {
		_ = claim.Close()
		state.addProtected(artifact.path, currentInfo.Size())
		state.blocked[artifact.path] = struct{}{}
		return nil, nil, false
	}
	return claim, currentInfo, true
}

func (retention *RuntimeMetricsRetention) protectChangedRemovalCandidate(
	state *retentionSweepState,
	artifact retentionArtifact,
	currentSize int64,
) {
	state.addProtected(artifact.path, currentSize)
	state.blocked[artifact.path] = struct{}{}
	state.remaining[artifact.path] = retentionArtifact{
		path: artifact.path, size: currentSize, timestamp: artifact.timestamp,
		dateDirectory: artifact.dateDirectory, hasTimestamp: artifact.hasTimestamp,
	}
}

func (retention *RuntimeMetricsRetention) removeCandidate(
	state *retentionSweepState,
	artifact retentionArtifact,
) {
	if err := retention.filesystem.Remove(artifact.path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			state.markRemoved(artifact)
			return
		}
		state.addFailed(artifact.path, artifact.size, err)
		state.blocked[artifact.path] = struct{}{}
		return
	}
	state.markRemoved(artifact)
}

func retentionTotals(artifacts map[string]retentionArtifact) RuntimeMetricsRetentionTotals {
	var totals RuntimeMetricsRetentionTotals
	totals.Files = len(artifacts)
	for _, artifact := range artifacts {
		totals.Bytes += artifact.size
	}
	return totals
}
