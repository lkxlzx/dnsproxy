package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/errors"
)

// ReloadConfig contains configuration for dynamic reloading.
type ReloadConfig struct {
	// Enabled indicates whether auto-reload is enabled.
	Enabled bool

	// WatchFile is the path to the configuration file to watch.
	WatchFile string

	// CheckInterval is how often to check for file changes.
	CheckInterval time.Duration

	// OnReload is called after a successful reload.
	OnReload func(*UpstreamGroupConfig)

	// OnError is called when reload fails.
	OnError func(error)
}

// DefaultReloadConfig returns the default reload configuration.
func DefaultReloadConfig() *ReloadConfig {
	return &ReloadConfig{
		Enabled:       false,
		CheckInterval: 30 * time.Second,
	}
}

// ConfigReloader handles dynamic configuration reloading.
type ConfigReloader struct {
	config      *ReloadConfig
	logger      *slog.Logger
	currentConf *UpstreamGroupConfig
	lastModTime time.Time
	mu          sync.RWMutex
	stopCh      chan struct{}
	stoppedCh   chan struct{}
}

// NewConfigReloader creates a new ConfigReloader.
func NewConfigReloader(config *ReloadConfig, logger *slog.Logger) *ConfigReloader {
	if config == nil {
		config = DefaultReloadConfig()
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &ConfigReloader{
		config:    config,
		logger:    logger,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start begins watching for configuration changes.
func (cr *ConfigReloader) Start(initialConf *UpstreamGroupConfig) error {
	if !cr.config.Enabled {
		cr.logger.Info("config reloading is disabled")
		return nil
	}

	if cr.config.WatchFile == "" {
		return errors.Error("watch file not specified")
	}

	cr.mu.Lock()
	cr.currentConf = initialConf
	cr.mu.Unlock()

	// Get initial file modification time
	info, err := os.Stat(cr.config.WatchFile)
	if err != nil {
		return fmt.Errorf("stat watch file: %w", err)
	}
	cr.lastModTime = info.ModTime()

	go cr.watch()

	cr.logger.Info(
		"config reloader started",
		"file", cr.config.WatchFile,
		"interval", cr.config.CheckInterval,
	)

	return nil
}

// Stop stops watching for configuration changes.
func (cr *ConfigReloader) Stop() {
	close(cr.stopCh)
	<-cr.stoppedCh
	cr.logger.Info("config reloader stopped")
}

// watch is the main watching loop.
func (cr *ConfigReloader) watch() {
	defer close(cr.stoppedCh)

	ticker := time.NewTicker(cr.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cr.stopCh:
			return
		case <-ticker.C:
			cr.checkAndReload()
		}
	}
}

// checkAndReload checks if the file has changed and reloads if necessary.
func (cr *ConfigReloader) checkAndReload() {
	info, err := os.Stat(cr.config.WatchFile)
	if err != nil {
		cr.logger.Error("failed to stat watch file", "error", err)
		if cr.config.OnError != nil {
			cr.config.OnError(err)
		}
		return
	}

	modTime := info.ModTime()
	if !modTime.After(cr.lastModTime) {
		return
	}

	cr.logger.Info("config file changed, reloading", "file", cr.config.WatchFile)
	cr.lastModTime = modTime

	// Load new configuration
	newConf, err := cr.loadConfig()
	if err != nil {
		cr.logger.Error("failed to load new config", "error", err)
		if cr.config.OnError != nil {
			cr.config.OnError(err)
		}
		return
	}

	// Validate new configuration
	if err := newConf.Validate(); err != nil {
		cr.logger.Error("new config validation failed", "error", err)
		if cr.config.OnError != nil {
			cr.config.OnError(err)
		}
		return
	}

	// Close old configuration
	cr.mu.Lock()
	oldConf := cr.currentConf
	cr.currentConf = newConf
	cr.mu.Unlock()

	if oldConf != nil {
		if err := oldConf.Close(); err != nil {
			cr.logger.Warn("failed to close old config", "error", err)
		}
	}

	cr.logger.Info("config reloaded successfully")

	if cr.config.OnReload != nil {
		cr.config.OnReload(newConf)
	}
}

// loadConfig loads the configuration from the watch file.
func (cr *ConfigReloader) loadConfig() (*UpstreamGroupConfig, error) {
	// Read file content
	content, err := os.ReadFile(cr.config.WatchFile)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Determine file format and parse
	// For simplicity, assume text format if file ends with .txt
	opts := &upstream.Options{
		Logger: cr.logger,
	}

	// Try text format first
	lines := splitLines(string(content))
	conf, err := ParseUpstreamGroupsFromLines(lines, opts)
	if err == nil {
		return conf, nil
	}

	// If text format fails, could try YAML format here
	return nil, fmt.Errorf("failed to parse config: %w", err)
}

// GetCurrentConfig returns the current configuration.
func (cr *ConfigReloader) GetCurrentConfig() *UpstreamGroupConfig {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.currentConf
}

// Reload manually triggers a configuration reload.
func (cr *ConfigReloader) Reload() error {
	cr.logger.Info("manual reload triggered")

	newConf, err := cr.loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := newConf.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	cr.mu.Lock()
	oldConf := cr.currentConf
	cr.currentConf = newConf
	cr.mu.Unlock()

	if oldConf != nil {
		if err := oldConf.Close(); err != nil {
			cr.logger.Warn("failed to close old config", "error", err)
		}
	}

	cr.logger.Info("manual reload completed successfully")

	if cr.config.OnReload != nil {
		cr.config.OnReload(newConf)
	}

	return nil
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	var lines []string
	var line string

	for _, ch := range content {
		if ch == '\n' {
			lines = append(lines, line)
			line = ""
		} else if ch != '\r' {
			line += string(ch)
		}
	}

	if line != "" {
		lines = append(lines, line)
	}

	return lines
}

// HotReloadManager manages hot reloading of upstream groups.
type HotReloadManager struct {
	reloader       *ConfigReloader
	healthChecker  *HealthChecker
	statsCollector *StatsCollector
	logger         *slog.Logger
	mu             sync.RWMutex
}

// NewHotReloadManager creates a new HotReloadManager.
func NewHotReloadManager(
	reloadConfig *ReloadConfig,
	healthConfig *HealthCheckConfig,
	logger *slog.Logger,
) *HotReloadManager {
	if logger == nil {
		logger = slog.Default()
	}

	hrm := &HotReloadManager{
		logger:         logger,
		statsCollector: NewStatsCollector(),
	}

	// Setup config reloader
	if reloadConfig != nil && reloadConfig.Enabled {
		hrm.reloader = NewConfigReloader(reloadConfig, logger)
		reloadConfig.OnReload = hrm.onConfigReload
		reloadConfig.OnError = hrm.onReloadError
	}

	// Setup health checker
	if healthConfig != nil && healthConfig.Enabled {
		hrm.healthChecker = NewHealthChecker(healthConfig, logger)
	}

	return hrm
}

// Start starts the hot reload manager.
func (hrm *HotReloadManager) Start(initialConf *UpstreamGroupConfig) error {
	if hrm.reloader != nil {
		if err := hrm.reloader.Start(initialConf); err != nil {
			return fmt.Errorf("start reloader: %w", err)
		}
	}

	if hrm.healthChecker != nil {
		// Add all upstreams to health checker
		for _, group := range initialConf.Groups {
			for _, u := range group.Upstreams {
				hrm.healthChecker.AddUpstream(u)
			}
		}
		hrm.healthChecker.Start()
	}

	hrm.logger.Info("hot reload manager started")
	return nil
}

// Stop stops the hot reload manager.
func (hrm *HotReloadManager) Stop() {
	if hrm.reloader != nil {
		hrm.reloader.Stop()
	}

	if hrm.healthChecker != nil {
		hrm.healthChecker.Stop()
	}

	hrm.logger.Info("hot reload manager stopped")
}

// onConfigReload is called when configuration is reloaded.
func (hrm *HotReloadManager) onConfigReload(newConf *UpstreamGroupConfig) {
	hrm.logger.Info("applying new configuration")

	// Update health checker with new upstreams
	if hrm.healthChecker != nil {
		for _, group := range newConf.Groups {
			for _, u := range group.Upstreams {
				hrm.healthChecker.AddUpstream(u)
			}
		}
	}

	hrm.logger.Info("new configuration applied successfully")
}

// onReloadError is called when reload fails.
func (hrm *HotReloadManager) onReloadError(err error) {
	hrm.logger.Error("config reload failed", "error", err)
}

// GetCurrentConfig returns the current configuration.
func (hrm *HotReloadManager) GetCurrentConfig() *UpstreamGroupConfig {
	if hrm.reloader != nil {
		return hrm.reloader.GetCurrentConfig()
	}
	return nil
}

// GetHealthChecker returns the health checker.
func (hrm *HotReloadManager) GetHealthChecker() *HealthChecker {
	return hrm.healthChecker
}

// GetStatsCollector returns the stats collector.
func (hrm *HotReloadManager) GetStatsCollector() *StatsCollector {
	return hrm.statsCollector
}

// Reload manually triggers a configuration reload.
func (hrm *HotReloadManager) Reload() error {
	if hrm.reloader == nil {
		return errors.Error("reloader not configured")
	}
	return hrm.reloader.Reload()
}
