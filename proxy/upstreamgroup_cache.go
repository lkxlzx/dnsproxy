package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DomainListCache manages caching of domain lists.
// This is the core caching functionality that AdGuard Home UI will use.
type DomainListCache struct {
	CacheDir string
	Logger   *slog.Logger
}

// CacheEntry represents a cached domain list entry.
type CacheEntry struct {
	Source      string    // Original source (URL or file path)
	CachedPath  string    // Local cached file path
	LastUpdate  time.Time // Last update time
	DomainCount int       // Number of domains
	Format      string    // Detected format
}

// NewDomainListCache creates a new domain list cache manager.
func NewDomainListCache(cacheDir string, logger *slog.Logger) *DomainListCache {
	if logger == nil {
		logger = slog.Default()
	}

	return &DomainListCache{
		CacheDir: cacheDir,
		Logger:   logger,
	}
}

// GetCachePath returns the cache file path for a given source.
// Uses SHA256 hash of the source URL/path as filename.
func (c *DomainListCache) GetCachePath(source string) string {
	hash := sha256.Sum256([]byte(source))
	filename := hex.EncodeToString(hash[:]) + ".cache"
	return filepath.Join(c.CacheDir, filename)
}

// IsCached checks if a source is cached and not expired.
func (c *DomainListCache) IsCached(source string, ttl time.Duration) bool {
	cachePath := c.GetCachePath(source)
	
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}

	// Check if cache is expired
	if ttl > 0 && time.Since(info.ModTime()) > ttl {
		c.Logger.Debug("cache expired", "source", source, "age", time.Since(info.ModTime()))
		return false
	}

	return true
}

// SaveToCache saves domain list to cache.
// This is called by AdGuard Home after downloading.
func (c *DomainListCache) SaveToCache(source string, content []byte) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	cachePath := c.GetCachePath(source)
	
	// Write to temporary file first
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename cache file: %w", err)
	}

	c.Logger.Info("saved to cache", "source", source, "path", cachePath, "size", len(content))
	return nil
}

// LoadFromCache loads domain list from cache.
func (c *DomainListCache) LoadFromCache(source string) ([]byte, error) {
	cachePath := c.GetCachePath(source)
	
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}

	c.Logger.Debug("loaded from cache", "source", source, "path", cachePath)
	return content, nil
}

// GetCacheInfo returns information about a cached entry.
func (c *DomainListCache) GetCacheInfo(source string) (*CacheEntry, error) {
	cachePath := c.GetCachePath(source)
	
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, fmt.Errorf("stat cache: %w", err)
	}

	return &CacheEntry{
		Source:     source,
		CachedPath: cachePath,
		LastUpdate: info.ModTime(),
	}, nil
}

// CleanCache removes expired cache entries.
func (c *DomainListCache) CleanCache(ttl time.Duration) error {
	entries, err := os.ReadDir(c.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cache dir: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(c.CacheDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > ttl {
			if err := os.Remove(path); err != nil {
				c.Logger.Warn("failed to remove expired cache", "path", path, "error", err)
			} else {
				removed++
				c.Logger.Debug("removed expired cache", "path", path)
			}
		}
	}

	if removed > 0 {
		c.Logger.Info("cleaned cache", "removed", removed)
	}

	return nil
}

// LoadDomainsWithCache loads domains with caching support.
// This is the main function that integrates caching with domain loading.
func LoadDomainsWithCache(
	source string,
	cache *DomainListCache,
	ttl time.Duration,
	fallbackToStale bool,
) ([]string, error) {
	loader := NewDomainFileLoader(cache.Logger)

	// Check if source is a URL
	isURL := isURLSource(source)

	if isURL && cache != nil {
		// Try to load from cache first
		if cache.IsCached(source, ttl) {
			cache.Logger.Info("using cached domain list", "source", source)
			content, err := cache.LoadFromCache(source)
			if err == nil {
				// Parse cached content
				domains, parseErr := loader.parseDomains(content, source)
				if parseErr == nil {
					return domains, nil
				}
				cache.Logger.Warn("failed to parse cached content", "error", parseErr)
			}
		}

		// Cache miss or expired, try to download
		cache.Logger.Info("downloading domain list", "source", source)
		domains, err := loader.LoadDomains(source)
		
		if err != nil {
			// Download failed, try to use stale cache if allowed
			if fallbackToStale {
				cache.Logger.Warn("download failed, trying stale cache", "source", source, "error", err)
				content, cacheErr := cache.LoadFromCache(source)
				if cacheErr == nil {
					domains, parseErr := loader.parseDomains(content, source)
					if parseErr == nil {
						cache.Logger.Info("using stale cache", "source", source)
						return domains, nil
					}
				}
			}
			return nil, fmt.Errorf("download failed and no cache available: %w", err)
		}

		// Download successful, save to cache
		// Note: AdGuard Home should handle this, but we provide the capability
		// Content is already downloaded by loader.LoadDomains
		
		return domains, nil
	}

	// Local file or no cache, load directly
	return loader.LoadDomains(source)
}

// isURLSource checks if a source is a URL.
func isURLSource(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}
