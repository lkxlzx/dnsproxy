package proxy

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DomainListSource represents a source for domain lists (file or URL).
type DomainListSource struct {
	// FilePath is the local file path (used as cache for URL sources)
	FilePath string

	// URL is the remote URL to fetch the list from (optional)
	URL string

	// Format is the format of the domain list (auto-detected if empty)
	Format string

	// UpdateInterval is how often to check for updates (0 = no auto-update)
	UpdateInterval time.Duration

	// LastUpdate is the timestamp of the last successful update
	LastUpdate time.Time

	// LastChecksum is the SHA256 checksum of the last downloaded content
	LastChecksum string

	// DomainCount is the number of valid domains in the list
	DomainCount int

	// LastError is the last error encountered during update
	LastError error
}

// DomainListUpdater manages automatic updates of domain lists from remote URLs.
type DomainListUpdater struct {
	mu      sync.RWMutex
	sources map[string]*DomainListSource // key: group name
	logger  *slog.Logger
	client  *http.Client
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewDomainListUpdater creates a new domain list updater.
func NewDomainListUpdater(logger *slog.Logger) *DomainListUpdater {
	ctx, cancel := context.WithCancel(context.Background())

	return &DomainListUpdater{
		sources: make(map[string]*DomainListSource),
		logger:  logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// AddSource adds or updates a domain list source.
func (u *DomainListUpdater) AddSource(groupName string, source *DomainListSource) error {
	if groupName == "" {
		return fmt.Errorf("group name cannot be empty")
	}

	if source.FilePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	u.sources[groupName] = source

	// Start auto-update if URL and interval are specified
	if source.URL != "" && source.UpdateInterval > 0 {
		u.wg.Add(1)
		go u.autoUpdate(groupName, source)
	}

	return nil
}

// GetSource retrieves a domain list source by group name.
func (u *DomainListUpdater) GetSource(groupName string) (*DomainListSource, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	source, ok := u.sources[groupName]
	return source, ok
}

// UpdateNow triggers an immediate update for a specific group.
func (u *DomainListUpdater) UpdateNow(groupName string) error {
	u.mu.RLock()
	source, ok := u.sources[groupName]
	u.mu.RUnlock()

	if !ok {
		return fmt.Errorf("source not found: %s", groupName)
	}

	if source.URL == "" {
		return fmt.Errorf("no URL configured for group: %s", groupName)
	}

	return u.updateSource(groupName, source)
}

// UpdateAll triggers an immediate update for all groups with URLs.
func (u *DomainListUpdater) UpdateAll() map[string]error {
	u.mu.RLock()
	sources := make(map[string]*DomainListSource, len(u.sources))
	for name, source := range u.sources {
		if source.URL != "" {
			sources[name] = source
		}
	}
	u.mu.RUnlock()

	results := make(map[string]error)
	for name, source := range sources {
		results[name] = u.updateSource(name, source)
	}

	return results
}

// autoUpdate runs periodic updates for a source.
func (u *DomainListUpdater) autoUpdate(groupName string, source *DomainListSource) {
	defer u.wg.Done()

	ticker := time.NewTicker(source.UpdateInterval)
	defer ticker.Stop()

	u.logger.Info("starting auto-update for domain list",
		"group", groupName,
		"url", source.URL,
		"interval", source.UpdateInterval)

	for {
		select {
		case <-u.ctx.Done():
			u.logger.Info("stopping auto-update", "group", groupName)
			return

		case <-ticker.C:
			if err := u.updateSource(groupName, source); err != nil {
				u.logger.Error("failed to update domain list",
					"group", groupName,
					"error", err)
			}
		}
	}
}

// updateSource downloads and updates a domain list from its URL.
func (u *DomainListUpdater) updateSource(groupName string, source *DomainListSource) error {
	u.logger.Info("updating domain list", "group", groupName, "url", source.URL)

	// Download content
	content, checksum, err := u.downloadURL(source.URL)
	if err != nil {
		u.updateSourceError(groupName, err)
		return fmt.Errorf("download failed: %w", err)
	}

	// Check if content changed
	if checksum == source.LastChecksum {
		u.logger.Debug("domain list unchanged", "group", groupName)
		return nil
	}

	// Parse and count domains
	domains, err := u.parseDomainList(content, source.Format)
	if err != nil {
		u.updateSourceError(groupName, err)
		return fmt.Errorf("parse failed: %w", err)
	}

	// Save to file
	if err := u.saveToFile(source.FilePath, content); err != nil {
		u.updateSourceError(groupName, err)
		return fmt.Errorf("save failed: %w", err)
	}

	// Update source metadata
	u.mu.Lock()
	source.LastUpdate = time.Now()
	source.LastChecksum = checksum
	source.DomainCount = len(domains)
	source.LastError = nil
	u.mu.Unlock()

	u.logger.Info("domain list updated successfully",
		"group", groupName,
		"domains", len(domains),
		"checksum", checksum[:16])

	return nil
}

// downloadURL downloads content from a URL and returns content and checksum.
func (u *DomainListUpdater) downloadURL(url string) (content []byte, checksum string, err error) {
	req, err := http.NewRequestWithContext(u.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http status: %d", resp.StatusCode)
	}

	content, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	// Calculate checksum
	hash := sha256.Sum256(content)
	checksum = fmt.Sprintf("%x", hash)

	return content, checksum, nil
}

// parseDomainList parses domain list content and returns valid domains.
func (u *DomainListUpdater) parseDomainList(content []byte, format string) ([]string, error) {
	// Detect format if not specified
	var detectedFormat DomainListFormat
	if format == "" {
		detectedFormat = detectFormatFromContent(content)
	} else {
		// Convert string to DomainListFormat
		switch format {
		case "plain":
			detectedFormat = FormatPlain
		case "dnsmasq":
			detectedFormat = FormatDnsmasq
		case "gfwlist":
			detectedFormat = FormatGFWList
		case "clash":
			detectedFormat = FormatClash
		default:
			detectedFormat = FormatPlain
		}
	}

	// Parse content directly
	domains, err := parseDomainListContent(content, detectedFormat)
	if err != nil {
		return nil, fmt.Errorf("parse format: %w", err)
	}

	return domains, nil
}

// detectFormatFromContent detects the format from content
func detectFormatFromContent(content []byte) DomainListFormat {
	contentStr := string(content)
	
	// Check for GFWList (Base64 encoded, starts with specific marker)
	if len(content) > 0 && (content[0] == '[' || strings.Contains(contentStr[:minInt(100, len(contentStr))], "AutoProxy")) {
		return FormatGFWList
	}
	
	// Check for Dnsmasq format
	if strings.Contains(contentStr, "server=/") || strings.Contains(contentStr, "address=/") {
		return FormatDnsmasq
	}
	
	// Check for Clash YAML
	if strings.Contains(contentStr, "payload:") && strings.Contains(contentStr, "DOMAIN") {
		return FormatClash
	}
	
	// Default to plain text
	return FormatPlain
}

// parseDomainListContent parses domain list content based on format
func parseDomainListContent(content []byte, format DomainListFormat) ([]string, error) {
	lines := strings.Split(string(content), "\n")
	domains := make([]string, 0)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		
		var domain string
		switch format {
		case FormatDnsmasq:
			// Parse dnsmasq format: server=/domain/dns or address=/domain/ip
			if strings.HasPrefix(line, "server=/") {
				parts := strings.Split(line, "/")
				if len(parts) >= 2 {
					domain = parts[1]
				}
			} else if strings.HasPrefix(line, "address=/") {
				parts := strings.Split(line, "/")
				if len(parts) >= 2 {
					domain = parts[1]
				}
			}
		case FormatGFWList:
			// Parse GFWList format: ||domain or .domain
			if strings.HasPrefix(line, "||") {
				domain = strings.TrimPrefix(line, "||")
			} else if strings.HasPrefix(line, ".") {
				domain = strings.TrimPrefix(line, ".")
			}
		case FormatClash:
			// Parse Clash format: DOMAIN,domain or DOMAIN-SUFFIX,domain
			if strings.Contains(line, "DOMAIN,") {
				parts := strings.Split(line, ",")
				if len(parts) >= 2 {
					domain = parts[1]
				}
			} else if strings.Contains(line, "DOMAIN-SUFFIX,") {
				parts := strings.Split(line, ",")
				if len(parts) >= 2 {
					domain = parts[1]
				}
			}
		default: // FormatPlain
			domain = line
		}
		
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	
	return domains, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// saveToFile saves content to a file atomically.
func (u *DomainListUpdater) saveToFile(filePath string, content []byte) error {
	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write to temporary file first
	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, filePath); err != nil {
		os.Remove(tmpFile) // Clean up on error
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}

// updateSourceError updates the error status of a source.
func (u *DomainListUpdater) updateSourceError(groupName string, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if source, ok := u.sources[groupName]; ok {
		source.LastError = err
	}
}

// GetStatus returns the status of all domain list sources.
func (u *DomainListUpdater) GetStatus() map[string]DomainListSourceStatus {
	u.mu.RLock()
	defer u.mu.RUnlock()

	status := make(map[string]DomainListSourceStatus, len(u.sources))
	for name, source := range u.sources {
		status[name] = DomainListSourceStatus{
			FilePath:       source.FilePath,
			URL:            source.URL,
			Format:         source.Format,
			UpdateInterval: source.UpdateInterval,
			LastUpdate:     source.LastUpdate,
			LastChecksum:   source.LastChecksum,
			DomainCount:    source.DomainCount,
			LastError:      formatError(source.LastError),
		}
	}

	return status
}

// DomainListSourceStatus represents the status of a domain list source.
type DomainListSourceStatus struct {
	FilePath       string        `json:"file_path"`
	URL            string        `json:"url,omitempty"`
	Format         string        `json:"format,omitempty"`
	UpdateInterval time.Duration `json:"update_interval,omitempty"`
	LastUpdate     time.Time     `json:"last_update,omitempty"`
	LastChecksum   string        `json:"last_checksum,omitempty"`
	DomainCount    int           `json:"domain_count"`
	LastError      string        `json:"last_error,omitempty"`
}

// formatError converts an error to a string, handling nil.
func formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Stop stops all auto-update goroutines.
func (u *DomainListUpdater) Stop() {
	u.logger.Info("stopping domain list updater")
	u.cancel()
	u.wg.Wait()
	u.logger.Info("domain list updater stopped")
}
