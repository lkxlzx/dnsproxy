package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
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
	Name            string        // List name (e.g., "china", "gfw")
	Source          string        // Original source (URL or file)
	LocalPath       string        // Local cached file path
	Group           string        // Target upstream group
	Enabled         bool          // Whether the list is enabled
	LastUpdate      time.Time     // Last update time
	DomainCount     int           // Number of domains
	Format          string        // Detected or specified format
	AutoUpdate      bool          // Whether to auto-update
	RefreshInterval time.Duration // Custom refresh interval for this list (0 = use default)
}

// GetStats returns statistics for frontend display.
func (ml *ManagedList) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"name":         ml.Name,
		"source":       ml.Source,
		"group":        ml.Group,
		"enabled":      ml.Enabled,
		"domain_count": ml.DomainCount,
		"last_updated": ml.LastUpdate.Format(time.RFC3339),
		"auto_update":  ml.AutoUpdate,
		"format":       ml.Format,
	}
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

// NeedsRefresh checks if a list needs to be refreshed based on its refresh interval.
// Returns true if the list should be refreshed.
func (m *DomainListManager) NeedsRefresh(name string, defaultInterval time.Duration) bool {
	m.mu.RLock()
	list, exists := m.lists[name]
	m.mu.RUnlock()

	if !exists || !list.Enabled || !list.AutoUpdate {
		return false
	}

	// Determine refresh interval
	interval := list.RefreshInterval
	if interval == 0 {
		interval = defaultInterval
	}

	// Check if enough time has passed since last update
	return time.Since(list.LastUpdate) >= interval
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

// GetAllStats returns statistics for all managed lists (for frontend).
func (m *DomainListManager) GetAllStats() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]map[string]interface{}, 0, len(m.lists))
	for _, list := range m.lists {
		stats = append(stats, list.GetStats())
	}

	return stats
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

// DownloadAndCache downloads a remote list and caches it locally in YAML format.
// This is the main function AdGuard Home will call.
// It handles download, format detection, parsing, conversion to YAML, and caching automatically.
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

	// Convert to YAML format for local storage
	yamlContent, err := m.convertToYAML(domains, source)
	if err != nil {
		return fmt.Errorf("convert to YAML: %w", err)
	}

	// Save to cache
	if err := m.cache.SaveToCache(source, yamlContent); err != nil {
		return fmt.Errorf("save to cache: %w", err)
	}

	// Also save to specified local path if different
	if localPath != "" && localPath != m.cache.GetCachePath(source) {
		if err := os.WriteFile(localPath, yamlContent, 0644); err != nil {
			return fmt.Errorf("save to local path: %w", err)
		}
		m.logger.Info("saved to local path", "path", localPath)
	}

	m.logger.Info("download and cache completed", "source", source, "domains", len(domains))
	return nil
}

// convertToYAML converts domain list to YAML format.
// This creates a standardized YAML format that can be easily loaded by the core.
func (m *DomainListManager) convertToYAML(domains []string, source string) ([]byte, error) {
	// Create YAML structure (without comments in the map)
	data := map[string]interface{}{
		"domains": domains,
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal YAML: %w", err)
	}

	// Prepend comments manually
	header := fmt.Sprintf("# Generated from: %s\n# Generated at: %s\n# Total domains: %d\n\n",
		source,
		time.Now().Format(time.RFC3339),
		len(domains))

	result := append([]byte(header), yamlBytes...)
	return result, nil
}

// saveDomainsAsYAML saves domains to a file in YAML format.
func (m *DomainListManager) saveDomainsAsYAML(domains []string, source, filePath string) error {
	// Convert to YAML
	yamlContent, err := m.convertToYAML(domains, source)
	if err != nil {
		return fmt.Errorf("convert to YAML: %w", err)
	}

	// Ensure directory exists
	dir := filePath
	if lastSep := strings.LastIndexAny(filePath, "/\\"); lastSep > 0 {
		dir = filePath[:lastSep]
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(filePath, yamlContent, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

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

// StartAutoRefresh starts a background goroutine that automatically refreshes lists.
// It checks all lists periodically and refreshes those that need updating.
func (m *DomainListManager) StartAutoRefresh(defaultInterval time.Duration, checkInterval time.Duration) {
	if checkInterval == 0 {
		checkInterval = 1 * time.Hour // Default check every hour
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		m.logger.Info("auto-refresh started", "check_interval", checkInterval)

		for range ticker.C {
			m.refreshExpiredLists(defaultInterval)
		}
	}()
}

// refreshExpiredLists checks and refreshes all lists that need updating.
func (m *DomainListManager) refreshExpiredLists(defaultInterval time.Duration) {
	m.mu.RLock()
	listsToRefresh := make([]*ManagedList, 0)

	for _, list := range m.lists {
		if !list.Enabled || !list.AutoUpdate {
			continue
		}

		// Determine refresh interval
		interval := list.RefreshInterval
		if interval == 0 {
			interval = defaultInterval
		}

		// Check if needs refresh
		if time.Since(list.LastUpdate) >= interval {
			listsToRefresh = append(listsToRefresh, list)
		}
	}
	m.mu.RUnlock()

	// Refresh lists outside the lock
	for _, list := range listsToRefresh {
		m.logger.Info("auto-refreshing list",
			"name", list.Name,
			"source", list.Source,
			"last_update", list.LastUpdate,
			"interval", list.RefreshInterval)

		if err := m.UpdateList(list.Name); err != nil {
			m.logger.Error("failed to auto-refresh list",
				"name", list.Name,
				"error", err)
		} else {
			m.logger.Info("auto-refresh completed",
				"name", list.Name,
				"domains", list.DomainCount)
		}
	}
}

// StopAutoRefresh stops the auto-refresh goroutine.
// Note: This is a placeholder. In a real implementation, you'd need to
// store the ticker and provide a way to stop it.
func (m *DomainListManager) StopAutoRefresh() {
	m.logger.Info("auto-refresh stopped")
}
