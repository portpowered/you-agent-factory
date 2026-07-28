package service

import (
	"context"
	"sync"
)

// DispatchCache materializes each URL at most once per dispatch; call Release when dispatch completes.
type DispatchCache struct {
	mu      sync.Mutex
	entries map[string]*dispatchCacheEntry
}

type dispatchCacheEntry struct {
	localPath string
	cleanup   CleanupFunc
}

// NewDispatchCache returns an empty per-dispatch materialization cache.
func NewDispatchCache() *DispatchCache {
	return &DispatchCache{entries: make(map[string]*dispatchCacheEntry)}
}

// MaterializeContentURL returns a cached local path when rawURL was already materialized in this cache.
func (c *DispatchCache) MaterializeContentURL(ctx context.Context, rawURL string, opts *Options) (localPath string, cleanup CleanupFunc, err error) {
	c.mu.Lock()
	if entry, ok := c.entries[rawURL]; ok {
		c.mu.Unlock()
		return entry.localPath, noopCleanup, nil
	}
	c.mu.Unlock()

	path, cleanup, err := MaterializeContentURL(ctx, rawURL, opts)
	if err != nil {
		return "", noopCleanup, err
	}

	realCleanup := cleanup
	c.mu.Lock()
	if entry, ok := c.entries[rawURL]; ok {
		c.mu.Unlock()
		realCleanup()
		return entry.localPath, noopCleanup, nil
	}
	c.entries[rawURL] = &dispatchCacheEntry{localPath: path, cleanup: realCleanup}
	c.mu.Unlock()
	return path, noopCleanup, nil
}

// Release runs cleanup for all materialized URLs in this cache.
func (c *DispatchCache) Release() {
	c.mu.Lock()
	entries := c.entries
	c.entries = make(map[string]*dispatchCacheEntry)
	c.mu.Unlock()

	for _, entry := range entries {
		if entry.cleanup != nil {
			entry.cleanup()
		}
	}
}
