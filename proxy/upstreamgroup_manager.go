package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// DomainListManager manages domain lists for AdGuard Home integration.
// This provides the core functionality that AdGuard Home UI will use.
type DomainListManager struct {
	cache  *DomainListCache
	lists  map[string]*ManagedList
	mu     sync.RWMutex
	logger *slog.Logger
}

// ManagedList represents a managed domain list.
type ManagedList struct {
	Name        string    // List name (e.g., "china", "gfw")
	Source      string    // Original source (URL or file)
	LocalPath   string    // Local cached file path
	Group       string    // Target upstream group
	Enabled     bool      // Whether the list is enabled
	LastUpdate  time.Time // Last update time
	DomainCount int       // Number of domains
	Format      string    // Detected format
	AutoUpdate  bool      // Whether to auto-update
}

// NewDomainListManager creates a new domain list manager.
func NewDomainListManager(cacheDir string, logger *slog.Logger) *DomainListManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &DomainListManager{
		cache:  NewDomainListCache(cacheDir, logger),
		lists:  make(map[string]*ManagedList),
		logger: logger,
	}
}

// AddList adds a new managed list.
// This is called by AdGuard Home when user adds a list via UI.
func (m *DomainListManager) AddList(list *ManagedList) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.lists[list.Name]; exists {
		return fmt.Errorf("list %q already exists", list.Name)
	}

	m.lists[list.Name] = list
	m.logger.Info("added managed list", "name", list.Name, "source", list.Source)
	return nil
}

// RemoveList removes a managed list.
func (m *DomainListManager) RemoveList(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.lists[name]; !exists {
		return fmt.Errorf("list %q not found", name)
	}

	delete(m.lists, name)
	m.logger.Info("removed managed list", "name", name)
	return nil
}

// GetList returns a managed list by name.
func (m *DomainListManager) GetList(name string) (*ManagedList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list, exists := m.lists[name]
	if !exists {
		return nil, fmt.Errorf("list %q not found", name)
	}

	return list, nil
}

// ListAll returns all managed lists.
func (m *DomainListManager) ListAll() []*ManagedList {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lists := make([]*ManagedList, 0, len(m.lists))
	for _, list := range m.lists {
		lists = append(lists, list)
	}

	return lists
}

// UpdateList updates a managed list.
// This is called by AdGuard Home when user clicks "Update" in UI.
func (m *DomainListManager) UpdateList(name string) error {
	// Get source safely
	m.mu.RLock()
	list, exists := m.lists[name]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("list %q not found", name)
	}
	source := list.Source
	m.mu.RUnlock()

	m.logger.Info("updating list", "name", name, "source", source)

	// Load domains (will download if URL)
	loader := NewDomainFileLoader(m.logger)
	domains, err := loader.LoadDomains(source)
	if err != nil {
		return fmt.Errorf("load domains: %w", err)
	}

	// Update list info safely
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Re-check existence after acquiring write lock
	list, exists = m.lists[name]
	if !exists {
		return fmt.Errorf("list %q was removed during update", name)
	}
	
	list.DomainCount = len(domains)
	list.LastUpdate = time.Now()

	m.logger.Info("updated list", "name", name, "domains", len(domains))
	return nil
}

// DownloadAndCache downloads a remote list and caches it locally.
// This is the main function AdGuard Home will call.
// It handles download, format conversion, and caching automatically.
func (m *DomainListManager) DownloadAndCache(source, localPath string) error {
	m.logger.Info("downloading domain list", "source", source, "target", localPath)

	// Use the domain file loader (already supports all formats)
	loader := NewDomainFileLoader(m.logger)
	
	// Download and parse (loader handles format detection and conversion)
	domains, err := loader.LoadDomains(source)
	if err != nil {
		return fmt.Errorf("download and parse failed: %w", err)
	}

	m.logger.Info("downloaded and parsed", "source", source, "domains", len(domains))

	// Convert to plain text format for local storage
	converter := NewFormatConverter(m.logger)
	content := converter.toPlainText(domains)

	// Save to cache
	if err := m.cache.SaveToCache(source, content); err != nil {
		return fmt.Errorf("save to cache: %w", err)
	}

	// Also save to specified local path if different
	if localPath != "" && localPath != m.cache.GetCachePath(source) {
		if err := os.WriteFile(localPath, content, 0644); err != nil {
			return fmt.Errorf("save to local path: %w", err)
		}
		m.logger.Info("saved to local path", "path", localPath)
	}

	m.logger.Info("download and cache completed", "source", source, "domains", len(domains))
	return nil
}

// GetCachePath returns the cache path for a source.
// AdGuard Home can use this to know where the cached file is.
func (m *DomainListManager) GetCachePath(source string) string {
	return m.cache.GetCachePath(source)
}

// CleanExpiredCache removes expired cache entries.
func (m *DomainListManager) CleanExpiredCache(ttl time.Duration) error {
	return m.cache.CleanCache(ttl)
}

// BuildDomainGroupsConfig builds domain_groups configuration from managed lists.
// This generates the configuration that will be used by ParseUpstreamGroups.
func (m *DomainListManager) BuildDomainGroupsConfig() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config := make(map[string]string)

	for _, list := range m.lists {
		if !list.Enabled {
			continue
		}

		// Use local cached path if available, otherwise use source
		path := list.LocalPath
		if path == "" {
			path = list.Source
		}

		// Map: group name -> file path
		config[list.Group] = path
	}

	return config
}
