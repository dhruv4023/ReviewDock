package stats

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// CacheService manages the on-disk stats cache (stats_cache.json) for all
// tracked pull requests.  Entries are keyed by PR ID.
type CacheService struct {
	mu      sync.Mutex
	dataDir string
}

// NewCacheService creates a CacheService that persists data under dataDir.
// The directory must already exist (created by the storage service on startup).
func NewCacheService(dataDir string) *CacheService {
	return &CacheService{dataDir: dataDir}
}

func (s *CacheService) filePath() string {
	return filepath.Join(s.dataDir, "stats_cache.json")
}

// readAll loads the full cache map from disk.  Returns an empty map if the
// file does not exist yet; returns an error only on real I/O or parse failures.
func (s *CacheService) readAll() (map[string]PRStatsCache, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]PRStatsCache), nil
		}
		return nil, err
	}
	var cache map[string]PRStatsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		// Corrupt file — start fresh rather than surfacing an error to the user.
		return make(map[string]PRStatsCache), nil
	}
	return cache, nil
}

// writeAll atomically replaces the cache file.
func (s *CacheService) writeAll(cache map[string]PRStatsCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), data, 0600)
}

// Get returns the cached entry for prID and whether it was found.
func (s *CacheService) Get(prID string) (PRStatsCache, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		return PRStatsCache{}, false
	}
	entry, ok := all[prID]
	return entry, ok
}

// Set stores or overwrites the cache entry for prID.
func (s *CacheService) Set(prID string, entry PRStatsCache) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		all = make(map[string]PRStatsCache)
	}
	all[prID] = entry
	return s.writeAll(all)
}

// Delete removes an entry from the cache (e.g. when a PR is removed).
func (s *CacheService) Delete(prID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		return err
	}
	delete(all, prID)
	return s.writeAll(all)
}
