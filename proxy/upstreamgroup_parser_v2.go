package proxy

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/errors"
)

// DomainGroupConfigV2 represents the enhanced domain group configuration.
type DomainGroupConfigV2 struct {
	// CustomDomains are directly specified domain patterns
	CustomDomains []string `yaml:"custom_domains"`

	// RemoteLists are remote domain list sources
	RemoteLists []RemoteListSpec `yaml:"remote_lists"`
}

// RemoteListSpec represents a remote domain list specification.
type RemoteListSpec struct {
	// Name is the display name of this list
	Name string `yaml:"name"`

	// Source is the URL or file path
	Source string `yaml:"source"`

	// CacheFile is the local cache file path
	CacheFile string `yaml:"cache_file"`

	// AutoUpdate enables automatic updates
	AutoUpdate bool `yaml:"auto_update"`

	// RefreshInterval is the refresh interval
	RefreshInterval string `yaml:"refresh_interval"`

	// Enabled indicates whether this list is active
	Enabled bool `yaml:"enabled"`

	// Format specifies the file format (optional, for faster parsing)
	Format string `yaml:"format"`
}

// UpstreamGroupsSpecV2 represents the enhanced configuration format.
type UpstreamGroupsSpecV2 struct {
	// Groups is the list of upstream group specifications
	Groups []UpstreamGroupSpec `yaml:"upstream_groups"`

	// DomainGroups maps group names to domain configurations
	// Supports both old format (map[string]interface{}) and new format (map[string]DomainGroupConfigV2)
	DomainGroups map[string]interface{} `yaml:"domain_groups"`

	// DefaultGroup is the name of the default group
	DefaultGroup string `yaml:"default_group"`

	// Cache configuration
	Cache *CacheConfig `yaml:"cache"`
}

// CacheConfig represents cache configuration.
type CacheConfig struct {
	Enabled               bool   `yaml:"enabled"`
	Directory             string `yaml:"directory"`
	TTL                   string `yaml:"ttl"`
	DefaultRefreshInterval string `yaml:"default_refresh_interval"`
	AutoUpdate            bool   `yaml:"auto_update"`
	MaxSize               string `yaml:"max_size"`
	CleanupInterval       string `yaml:"cleanup_interval"`
}

// ValidateGroupName validates a group name, supporting UTF-8 characters.
func ValidateGroupName(name string) error {
	if name == "" {
		return errors.Error("group name cannot be empty")
	}

	// Check if it's valid UTF-8
	if !utf8.ValidString(name) {
		return fmt.Errorf("group name %q contains invalid UTF-8 characters", name)
	}

	// Trim spaces
	trimmed := strings.TrimSpace(name)
	if trimmed != name {
		return fmt.Errorf("group name %q contains leading or trailing spaces", name)
	}

	// Check for control characters
	for _, r := range name {
		if r < 32 || r == 127 {
			return fmt.Errorf("group name %q contains control characters", name)
		}
	}

	return nil
}

// NormalizeGroupName normalizes a group name for internal use.
// It preserves UTF-8 characters but ensures consistent formatting.
func NormalizeGroupName(name string) string {
	// Trim spaces
	name = strings.TrimSpace(name)

	// Normalize whitespace (replace multiple spaces with single space)
	name = strings.Join(strings.Fields(name), " ")

	return name
}

// ParseUpstreamGroupsV2 parses the enhanced configuration format.
func ParseUpstreamGroupsV2(
	spec *UpstreamGroupsSpecV2,
	opts *upstream.Options,
) (ugc *UpstreamGroupConfig, manager *DomainListManager, err error) {
	if spec == nil {
		return nil, nil, errors.Error("upstream groups spec cannot be nil")
	}

	if opts == nil {
		opts = &upstream.Options{}
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	ugc = NewUpstreamGroupConfig()

	// Normalize and validate default group name
	if spec.DefaultGroup != "" {
		normalizedDefault := NormalizeGroupName(spec.DefaultGroup)
		if err := ValidateGroupName(normalizedDefault); err != nil {
			return nil, nil, fmt.Errorf("invalid default group name: %w", err)
		}
		ugc.DefaultGroup = normalizedDefault
	}

	// Parse each group
	for i, groupSpec := range spec.Groups {
		// Normalize and validate group name
		normalizedName := NormalizeGroupName(groupSpec.Name)
		if err := ValidateGroupName(normalizedName); err != nil {
			return nil, nil, fmt.Errorf("invalid group name at index %d: %w", i, err)
		}
		groupSpec.Name = normalizedName

		group, parseErr := parseUpstreamGroup(&groupSpec, opts)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parsing group at index %d: %w", i, parseErr)
		}

		if addErr := ugc.AddGroup(group); addErr != nil {
			return nil, nil, fmt.Errorf("adding group %q: %w", group.Name, addErr)
		}
	}

	// Initialize domain list manager if cache is enabled
	var cacheDir string
	if spec.Cache != nil && spec.Cache.Enabled {
		cacheDir = spec.Cache.Directory
		if cacheDir == "" {
			cacheDir = "./cache"
		}
	}

	manager = NewDomainListManager(cacheDir, opts.Logger)

	// Parse domain groups
	domainMappings, err := parseDomainGroupsV2(spec.DomainGroups, manager, opts.Logger)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing domain groups: %w", err)
	}

	// Set domain-group mappings
	for domain, groupName := range domainMappings {
		// Normalize group name in mappings
		normalizedGroup := NormalizeGroupName(groupName)
		if setErr := ugc.SetDomainGroup(domain, normalizedGroup); setErr != nil {
			return nil, nil, fmt.Errorf("setting domain group for %q: %w", domain, setErr)
		}
	}

	// Validate the configuration
	if err = ugc.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validating upstream groups config: %w", err)
	}

	// Rebuild trie for optimized domain matching
	ugc.RebuildTrie()

	// Start auto-refresh if enabled
	if spec.Cache != nil && spec.Cache.AutoUpdate {
		defaultInterval := 24 * time.Hour
		if spec.Cache.DefaultRefreshInterval != "" {
			if interval, parseErr := time.ParseDuration(spec.Cache.DefaultRefreshInterval); parseErr == nil {
				defaultInterval = interval
			}
		}

		checkInterval := 1 * time.Hour
		if spec.Cache.CleanupInterval != "" {
			if interval, parseErr := time.ParseDuration(spec.Cache.CleanupInterval); parseErr == nil {
				checkInterval = interval
			}
		}

		manager.StartAutoRefresh(defaultInterval, checkInterval)
		opts.Logger.Info("auto-refresh started",
			"default_interval", defaultInterval,
			"check_interval", checkInterval)
	}

	return ugc, manager, nil
}

// parseDomainGroupsV2 parses the enhanced domain groups configuration.
func parseDomainGroupsV2(
	domainGroups map[string]interface{},
	manager *DomainListManager,
	logger *slog.Logger,
) (map[string]string, error) {
	result := make(map[string]string)

	for groupName, value := range domainGroups {
		// Normalize group name
		normalizedGroup := NormalizeGroupName(groupName)
		if err := ValidateGroupName(normalizedGroup); err != nil {
			return nil, fmt.Errorf("invalid group name %q: %w", groupName, err)
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// New format: DomainGroupConfigV2
			if err := parseDomainGroupConfigV2(v, normalizedGroup, manager, result, logger); err != nil {
				return nil, fmt.Errorf("parsing group %q: %w", normalizedGroup, err)
			}

		case []interface{}:
			// Old format: array of sources
			for i, item := range v {
				if err := processArrayItem(item, normalizedGroup, manager, result, logger); err != nil {
					return nil, fmt.Errorf("parsing group %q item %d: %w", normalizedGroup, i, err)
				}
			}

		case string:
			// Old format: simple string
			if err := processSimpleString(v, normalizedGroup, manager, result, logger); err != nil {
				return nil, fmt.Errorf("parsing group %q: %w", normalizedGroup, err)
			}

		default:
			return nil, fmt.Errorf("unsupported value type for group %q", normalizedGroup)
		}
	}

	return result, nil
}

// parseDomainGroupConfigV2 parses the new DomainGroupConfigV2 format.
func parseDomainGroupConfigV2(
	config map[string]interface{},
	groupName string,
	manager *DomainListManager,
	result map[string]string,
	logger *slog.Logger,
) error {
	// Parse custom_domains
	if customDomainsVal, ok := config["custom_domains"]; ok {
		if customDomains, ok := customDomainsVal.([]interface{}); ok {
			for _, domainVal := range customDomains {
				if domain, ok := domainVal.(string); ok {
					result[domain] = groupName
				}
			}
		}
	}

	// Parse remote_lists
	if remoteListsVal, ok := config["remote_lists"]; ok {
		if remoteLists, ok := remoteListsVal.([]interface{}); ok {
			for i, listVal := range remoteLists {
				if listMap, ok := listVal.(map[string]interface{}); ok {
					if err := parseRemoteListSpec(listMap, groupName, manager, result, logger); err != nil {
						return fmt.Errorf("parsing remote list %d: %w", i, err)
					}
				}
			}
		}
	}

	return nil
}

// parseRemoteListSpec parses a RemoteListSpec.
func parseRemoteListSpec(
	spec map[string]interface{},
	groupName string,
	manager *DomainListManager,
	result map[string]string,
	logger *slog.Logger,
) error {
	// Extract fields
	source, _ := spec["source"].(string)
	if source == "" {
		return errors.Error("missing 'source' field")
	}

	enabled := true
	if enabledVal, ok := spec["enabled"]; ok {
		if enabledBool, ok := enabledVal.(bool); ok {
			enabled = enabledBool
		}
	}

	if !enabled {
		logger.Info("skipping disabled remote list", "source", source, "group", groupName)
		return nil
	}

	// Parse refresh interval
	var refreshInterval time.Duration
	if intervalStr, ok := spec["refresh_interval"].(string); ok && intervalStr != "" {
		var err error
		refreshInterval, err = time.ParseDuration(intervalStr)
		if err != nil {
			return fmt.Errorf("invalid refresh_interval: %w", err)
		}
	}

	// Load domains
	loader := NewDomainFileLoader(logger)
	domains, err := loader.LoadDomains(source)
	if err != nil {
		return fmt.Errorf("load domains from %q: %w", source, err)
	}

	// Add to result
	for _, domain := range domains {
		result[domain] = groupName
	}

	// Register with manager
	if manager != nil {
		name, _ := spec["name"].(string)
		if name == "" {
			name = fmt.Sprintf("%s_%s", groupName, hashSource(source))
		}

		cacheFile, _ := spec["cache_file"].(string)
		autoUpdate, _ := spec["auto_update"].(bool)

		managedList := &ManagedList{
			Name:            name,
			Source:          source,
			LocalPath:       cacheFile,
			Group:           groupName,
			Enabled:         enabled,
			LastUpdate:      time.Now(),
			DomainCount:     len(domains),
			AutoUpdate:      autoUpdate,
			RefreshInterval: refreshInterval,
		}

		if err := manager.AddList(managedList); err != nil {
			logger.Warn("failed to add managed list", "name", name, "error", err)
		}
	}

	logger.Info("loaded remote list",
		"source", source,
		"group", groupName,
		"domains", len(domains),
		"refresh_interval", refreshInterval)

	return nil
}

// processArrayItem processes an array item (old format compatibility).
func processArrayItem(
	item interface{},
	groupName string,
	manager *DomainListManager,
	result map[string]string,
	logger *slog.Logger,
) error {
	switch v := item.(type) {
	case string:
		return processSimpleString(v, groupName, manager, result, logger)
	case map[string]interface{}:
		return processObjectSource(v, groupName, NewDomainFileLoader(logger), result, manager, logger)
	default:
		return fmt.Errorf("unsupported array item type")
	}
}

// processSimpleString processes a simple string (old format compatibility).
func processSimpleString(
	value string,
	groupName string,
	manager *DomainListManager,
	result map[string]string,
	logger *slog.Logger,
) error {
	if isFileOrURLSource(value) {
		return loadDomainsFromSource(value, groupName, NewDomainFileLoader(logger), result, logger)
	}
	result[value] = groupName
	return nil
}
