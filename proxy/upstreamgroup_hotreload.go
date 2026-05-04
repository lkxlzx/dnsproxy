package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

// HotReloadManager manages hot reloading of configuration
type HotReloadManager struct {
	configPath  string
	spec        *UpstreamGroupsSpec
	ugc         *UpstreamGroupConfig
	opts        *upstream.Options
	mu          sync.RWMutex
	logger      *slog.Logger
	lastModTime time.Time
	onChange    func(*UpstreamGroupConfig) error
}

// NewHotReloadManager creates a new hot reload manager
func NewHotReloadManager(
	configPath string,
	spec *UpstreamGroupsSpec,
	opts *upstream.Options,
	onChange func(*UpstreamGroupConfig) error,
) *HotReloadManager {
	if opts == nil {
		opts = &upstream.Options{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &HotReloadManager{
		configPath: configPath,
		spec:       spec,
		opts:       opts,
		logger:     opts.Logger,
		onChange:   onChange,
	}
}

// Start starts watching for configuration changes
func (hrm *HotReloadManager) Start(checkInterval time.Duration) {
	if checkInterval == 0 {
		checkInterval = 5 * time.Second
	}

	// Get initial modification time
	info, err := os.Stat(hrm.configPath)
	if err != nil {
		hrm.logger.Error("failed to stat config file", "error", err)
		return
	}
	hrm.lastModTime = info.ModTime()

	hrm.logger.Info("hot reload started",
		"config", hrm.configPath,
		"check_interval", checkInterval)

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for range ticker.C {
			if err := hrm.checkAndReload(); err != nil {
				hrm.logger.Error("hot reload failed", "error", err)
			}
		}
	}()
}

// checkAndReload checks if config file changed and reloads if needed
func (hrm *HotReloadManager) checkAndReload() error {
	info, err := os.Stat(hrm.configPath)
	if err != nil {
		return fmt.Errorf("stat config file: %w", err)
	}

	// Check if file was modified
	if !info.ModTime().After(hrm.lastModTime) {
		return nil
	}

	hrm.logger.Info("config file changed, reloading",
		"last_mod", hrm.lastModTime,
		"new_mod", info.ModTime())

	hrm.lastModTime = info.ModTime()

	// Reload configuration
	return hrm.Reload()
}

// Reload reloads the configuration
func (hrm *HotReloadManager) Reload() error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()
	return hrm.reloadLocked()
}

// reloadLocked reloads the configuration (caller must hold the lock)
func (hrm *HotReloadManager) reloadLocked() error {
	hrm.logger.Info("reloading configuration", "config", hrm.configPath)

	// Read config file
	data, err := os.ReadFile(hrm.configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Parse new spec
	var newSpec UpstreamGroupsSpec
	if err := yaml.Unmarshal(data, &newSpec); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Parse upstream groups
	newUGC, err := ParseUpstreamGroups(&newSpec, hrm.opts)
	if err != nil {
		return fmt.Errorf("parse upstream groups: %w", err)
	}

	// Update spec and ugc
	hrm.spec = &newSpec
	hrm.ugc = newUGC

	hrm.logger.Info("configuration reloaded successfully",
		"groups", len(newUGC.Groups),
		"domains", len(newUGC.DomainGroups))

	// Collect and update stats
	stats, err := CollectDomainListStats(&newSpec)
	if err == nil && len(stats) > 0 {
		if err := UpdateConfigFileStats(hrm.configPath, stats); err != nil {
			hrm.logger.Warn("failed to update config stats", "error", err)
		}
	}

	// Call onChange callback
	if hrm.onChange != nil {
		if err := hrm.onChange(newUGC); err != nil {
			hrm.logger.Error("onChange callback failed", "error", err)
		}
	}

	return nil
}

// GetCurrentConfig returns the current configuration
func (hrm *HotReloadManager) GetCurrentConfig() *UpstreamGroupConfig {
	hrm.mu.RLock()
	defer hrm.mu.RUnlock()
	return hrm.ugc
}

// AddDomainList adds a new domain list dynamically
func (hrm *HotReloadManager) AddDomainList(list DomainListSpec) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	hrm.logger.Info("adding new domain list", "name", list.Name, "source", list.Source)

	// Add to spec
	hrm.spec.DomainLists = append(hrm.spec.DomainLists, list)

	// Save to config file
	data, err := yaml.Marshal(hrm.spec)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(hrm.configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	hrm.logger.Info("domain list added to config", "name", list.Name)

	// Reload to apply changes
	return hrm.reloadLocked()
}

// RemoveDomainList removes a domain list dynamically
func (hrm *HotReloadManager) RemoveDomainList(name string) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	hrm.logger.Info("removing domain list", "name", name)

	// Find and remove from spec
	newLists := make([]DomainListSpec, 0)
	found := false
	for _, list := range hrm.spec.DomainLists {
		if list.Name == name {
			found = true
			continue
		}
		newLists = append(newLists, list)
	}

	if !found {
		return fmt.Errorf("domain list %q not found", name)
	}

	hrm.spec.DomainLists = newLists

	// Save to config file
	data, err := yaml.Marshal(hrm.spec)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(hrm.configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	hrm.logger.Info("domain list removed from config", "name", name)

	// Reload to apply changes
	return hrm.reloadLocked()
}

// UpdateDomainList updates an existing domain list
func (hrm *HotReloadManager) UpdateDomainList(name string, updates map[string]interface{}) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	hrm.logger.Info("updating domain list", "name", name)

	// Find and update
	found := false
	for i := range hrm.spec.DomainLists {
		if hrm.spec.DomainLists[i].Name == name {
			found = true

			// Apply updates
			if source, ok := updates["source"].(string); ok {
				hrm.spec.DomainLists[i].Source = source
			}
			if group, ok := updates["group"].(string); ok {
				hrm.spec.DomainLists[i].Group = group
			}
			if enabled, ok := updates["enabled"].(bool); ok {
				hrm.spec.DomainLists[i].Enabled = enabled
			}
			if autoUpdate, ok := updates["auto_update"].(bool); ok {
				hrm.spec.DomainLists[i].AutoUpdate = autoUpdate
			}
			if refreshInterval, ok := updates["refresh_interval"].(string); ok {
				hrm.spec.DomainLists[i].RefreshInterval = refreshInterval
			}

			break
		}
	}

	if !found {
		return fmt.Errorf("domain list %q not found", name)
	}

	// Save to config file
	data, err := yaml.Marshal(hrm.spec)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(hrm.configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	hrm.logger.Info("domain list updated in config", "name", name)

	// Reload to apply changes
	return hrm.reloadLocked()
}
