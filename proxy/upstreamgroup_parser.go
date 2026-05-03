package proxy

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/errors"
)

// UpstreamGroupSpec represents the specification for an upstream group from configuration.
type UpstreamGroupSpec struct {
	// ID is the unique identifier for this group (UUID format recommended).
	// Used for integration with external systems like AdGuard Home.
	ID string `yaml:"id"`

	// Name is the human-readable name for this group.
	Name string `yaml:"name"`

	// Upstreams is the list of upstream server addresses.
	Upstreams []string `yaml:"upstreams"`

	// Mode determines the upstream selection logic.
	Mode string `yaml:"mode"`

	// Timeout is the timeout for queries in human-readable format (e.g., "10s").
	Timeout string `yaml:"timeout"`

	// MaxRetries is the maximum number of retries for failed queries.
	MaxRetries int `yaml:"max_retries"`

	// Enabled indicates whether this group is active.
	Enabled bool `yaml:"enabled"`

	// Priority is used for fallback ordering.
	Priority int `yaml:"priority"`
}

// UpstreamGroupsSpec represents the complete upstream groups configuration.
type UpstreamGroupsSpec struct {
	// Groups is the list of upstream group specifications.
	Groups []UpstreamGroupSpec `yaml:"groups"`

	// DomainGroups maps domain patterns to group names or domain source specs.
	// Supports both simple string format and object format with refresh_interval.
	DomainGroups map[string]interface{} `yaml:"domain_groups"`

	// DomainLists is the list of remote domain list specifications.
	DomainLists []DomainListSpec `yaml:"domains_lists"`

	// DefaultGroup is the name of the default group.
	DefaultGroup string `yaml:"default_group"`

	// Cache configuration
	Cache *CacheConfigSpec `yaml:"cache"`
}

// DomainListSpec represents a domain list specification.
type DomainListSpec struct {
	// Name is the display name of this list
	Name string `yaml:"name"`

	// Source is the URL or file path
	Source string `yaml:"source"`

	// Group is the target upstream group name
	Group string `yaml:"group"`

	// File is the local cache file path
	File string `yaml:"file"`

	// AutoUpdate enables automatic updates
	AutoUpdate bool `yaml:"auto_update"`

	// RefreshInterval is the refresh interval
	RefreshInterval string `yaml:"refresh_interval"`

	// Enabled indicates whether this list is active
	Enabled bool `yaml:"enabled"`

	// Format specifies the file format (optional, for faster parsing)
	Format string `yaml:"format"`
}

// CacheConfigSpec represents cache configuration.
type CacheConfigSpec struct {
	Enabled                bool   `yaml:"enabled"`
	Directory              string `yaml:"directory"`
	TTL                    string `yaml:"ttl"`
	DefaultRefreshInterval string `yaml:"default_refresh_interval"`
	AutoUpdate             bool   `yaml:"auto_update"`
	MaxSize                string `yaml:"max_size"`
	CleanupInterval        string `yaml:"cleanup_interval"`
}

// DomainSourceSpec represents a domain source with custom refresh interval.
type DomainSourceSpec struct {
	// Source is the URL or file path
	Source string `yaml:"source"`

	// RefreshInterval is the custom refresh interval for this source
	RefreshInterval string `yaml:"refresh_interval"`

	// Enabled indicates whether this source is active
	Enabled bool `yaml:"enabled"`
}

// ParseUpstreamGroups parses upstream group specifications and creates an UpstreamGroupConfig.
func ParseUpstreamGroups(
	spec *UpstreamGroupsSpec,
	opts *upstream.Options,
) (ugc *UpstreamGroupConfig, err error) {
	if spec == nil {
		return nil, errors.Error("upstream groups spec cannot be nil")
	}

	if opts == nil {
		opts = &upstream.Options{}
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	ugc = NewUpstreamGroupConfig()
	ugc.DefaultGroup = spec.DefaultGroup

	// Parse each group and build ID-to-Name mapping
	idToName := make(map[string]string)
	for i, groupSpec := range spec.Groups {
		group, parseErr := parseUpstreamGroup(&groupSpec, opts)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing group at index %d: %w", i, parseErr)
		}

		if addErr := ugc.AddGroup(group); addErr != nil {
			return nil, fmt.Errorf("adding group %q: %w", group.Name, addErr)
		}

		// Build ID-to-Name mapping
		if group.ID != "" {
			idToName[group.ID] = group.Name
		}
	}

	// Resolve default_group: if it's an ID, convert to name
	if ugc.DefaultGroup != "" {
		if groupName, isID := idToName[ugc.DefaultGroup]; isID {
			opts.Logger.Info("resolved default_group ID to name",
				"id", ugc.DefaultGroup,
				"name", groupName)
			ugc.DefaultGroup = groupName
		}
	}

	// Initialize domain list manager if needed
	var manager *DomainListManager
	if spec.Cache != nil && spec.Cache.Enabled {
		cacheDir := spec.Cache.Directory
		if cacheDir == "" {
			cacheDir = "./cache"
		}
		manager = NewDomainListManager(cacheDir, opts.Logger)
	}

	// Process domains_lists first (if exists)
	if len(spec.DomainLists) > 0 {
		if err := processDomainLists(spec.DomainLists, manager, ugc, idToName, opts.Logger); err != nil {
			return nil, fmt.Errorf("processing domain lists: %w", err)
		}
	}

	// Load domains from domain_groups configuration
	expandedDomainGroups, err := LoadDomainsFromConfig(spec.DomainGroups, opts.Logger, manager)
	if err != nil {
		return nil, fmt.Errorf("loading domain files: %w", err)
	}

	// Set domain-group mappings (resolve IDs to names)
	for domain, groupRef := range expandedDomainGroups {
		// Check if groupRef is an ID, if so convert to name
		groupName := groupRef
		if resolvedName, isID := idToName[groupRef]; isID {
			groupName = resolvedName
		}
		
		if setErr := ugc.SetDomainGroup(domain, groupName); setErr != nil {
			return nil, fmt.Errorf("setting domain group for %q: %w", domain, setErr)
		}
	}

	// Validate the configuration
	if err = ugc.Validate(); err != nil {
		return nil, fmt.Errorf("validating upstream groups config: %w", err)
	}

	// Rebuild trie for optimized domain matching
	ugc.RebuildTrie()

	// Start auto-refresh if enabled
	if manager != nil && spec.Cache != nil && spec.Cache.AutoUpdate {
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

	return ugc, nil
}

// processDomainLists processes the domains_lists configuration.
func processDomainLists(
	lists []DomainListSpec,
	manager *DomainListManager,
	ugc *UpstreamGroupConfig,
	idToName map[string]string,
	logger *slog.Logger,
) error {
	loader := NewDomainFileLoader(logger)

	for i, listSpec := range lists {
		// Skip if disabled
		if !listSpec.Enabled {
			logger.Info("skipping disabled domain list", "name", listSpec.Name)
			continue
		}

		// Validate required fields
		if listSpec.Source == "" {
			return fmt.Errorf("domain list at index %d: missing source", i)
		}
		if listSpec.Group == "" {
			return fmt.Errorf("domain list at index %d: missing group", i)
		}

		// Resolve group reference (could be ID or name)
		groupRef := listSpec.Group
		groupName := groupRef
		if resolvedName, isID := idToName[groupRef]; isID {
			groupName = resolvedName
			logger.Info("resolved group ID to name",
				"list", listSpec.Name,
				"id", groupRef,
				"name", groupName)
		}

		logger.Info("loading domain list",
			"name", listSpec.Name,
			"source", listSpec.Source,
			"group", groupName)

		// Load domains
		domains, err := loader.LoadDomains(listSpec.Source)
		if err != nil {
			return fmt.Errorf("loading domain list %q: %w", listSpec.Name, err)
		}

		// If cache file is specified and manager exists, save as YAML
		if listSpec.File != "" && manager != nil {
			if err := manager.saveDomainsAsYAML(domains, listSpec.Source, listSpec.File); err != nil {
				logger.Warn("failed to save domains as YAML", "file", listSpec.File, "error", err)
			} else {
				logger.Info("saved domains as YAML", "file", listSpec.File, "domains", len(domains))
			}
		}

		// Add domains to configuration (use resolved group name)
		for _, domain := range domains {
			if setErr := ugc.SetDomainGroup(domain, groupName); setErr != nil {
				return fmt.Errorf("setting domain %q to group %q: %w", domain, groupName, setErr)
			}
		}

		logger.Info("loaded domain list",
			"name", listSpec.Name,
			"domains", len(domains),
			"group", groupName)

		// Register with manager if provided
		if manager != nil && listSpec.AutoUpdate {
			var refreshInterval time.Duration
			if listSpec.RefreshInterval != "" {
				var parseErr error
				refreshInterval, parseErr = time.ParseDuration(listSpec.RefreshInterval)
				if parseErr != nil {
					return fmt.Errorf("invalid refresh_interval for list %q: %w", listSpec.Name, parseErr)
				}
			}

			managedList := &ManagedList{
				Name:            listSpec.Name,
				Source:          listSpec.Source,
				LocalPath:       listSpec.File,
				Group:           listSpec.Group,
				Enabled:         listSpec.Enabled,
				LastUpdate:      time.Now(),
				DomainCount:     len(domains),
				AutoUpdate:      listSpec.AutoUpdate,
				RefreshInterval: refreshInterval,
				Format:          listSpec.Format,
			}

			if err := manager.AddList(managedList); err != nil {
				logger.Warn("failed to add managed list", "name", listSpec.Name, "error", err)
			}
		}
	}

	return nil
}

// parseUpstreamGroup parses a single upstream group specification.
func parseUpstreamGroup(
	spec *UpstreamGroupSpec,
	opts *upstream.Options,
) (group *UpstreamGroup, err error) {
	if spec.Name == "" {
		return nil, errors.Error("group name cannot be empty")
	}

	if len(spec.Upstreams) == 0 {
		return nil, fmt.Errorf("group %q has no upstreams", spec.Name)
	}

	group = &UpstreamGroup{
		ID:         spec.ID,
		Name:       spec.Name,
		MaxRetries: spec.MaxRetries,
		Enabled:    spec.Enabled,
		Priority:   spec.Priority,
	}

	// Parse mode
	if spec.Mode != "" {
		mode := UpstreamMode(spec.Mode)
		switch mode {
		case UpstreamModeLoadBalance, UpstreamModeParallel, UpstreamModeFastestAddr:
			group.Mode = mode
		default:
			return nil, fmt.Errorf("invalid upstream mode %q", spec.Mode)
		}
	} else {
		group.Mode = UpstreamModeLoadBalance
	}

	// Parse timeout
	if spec.Timeout != "" {
		timeout, parseErr := time.ParseDuration(spec.Timeout)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing timeout: %w", parseErr)
		}
		group.Timeout = timeout
	} else {
		group.Timeout = 10 * time.Second
	}

	// Parse upstreams
	upstreamIndex := make(map[string]upstream.Upstream)
	for i, upstreamAddr := range spec.Upstreams {
		// Check if already created (avoid duplicates)
		if u, exists := upstreamIndex[upstreamAddr]; exists {
			group.Upstreams = append(group.Upstreams, u)
			continue
		}

		// Create new upstream
		u, createErr := upstream.AddressToUpstream(upstreamAddr, opts.Clone())
		if createErr != nil {
			return nil, fmt.Errorf("creating upstream at index %d (%s): %w", i, upstreamAddr, createErr)
		}

		upstreamIndex[upstreamAddr] = u
		group.Upstreams = append(group.Upstreams, u)
	}

	return group, nil
}

// ParseUpstreamGroupsFromLines parses upstream groups from text lines.
// This provides a simple text-based configuration format.
//
// Format:
//   [group:group_name:mode:timeout]
//   upstream1
//   upstream2
//   [/domain1/domain2/]group_name
//
// Example:
//   [group:fast:load_balance:5s]
//   1.1.1.1
//   8.8.8.8
//   [group:secure:parallel:10s]
//   tls://dns.adguard.com
//   https://dns.google/dns-query
//   [/example.com/test.com/]secure
//   [default]fast
func ParseUpstreamGroupsFromLines(
	lines []string,
	opts *upstream.Options,
) (ugc *UpstreamGroupConfig, err error) {
	if opts == nil {
		opts = &upstream.Options{}
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	ugc = NewUpstreamGroupConfig()
	var currentGroup *UpstreamGroup
	upstreamIndex := make(map[string]upstream.Upstream)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for group definition
		if strings.HasPrefix(line, "[group:") && strings.HasSuffix(line, "]") {
			// Parse group definition: [group:name:mode:timeout]
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "[group:"), "]"), ":")
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: invalid group definition format", lineNum+1)
			}

			groupName := parts[0]
			mode := UpstreamModeLoadBalance
			timeout := 10 * time.Second

			if len(parts) >= 2 && parts[1] != "" {
				mode = UpstreamMode(parts[1])
			}

			if len(parts) >= 3 && parts[2] != "" {
				var parseErr error
				timeout, parseErr = time.ParseDuration(parts[2])
				if parseErr != nil {
					return nil, fmt.Errorf("line %d: invalid timeout: %w", lineNum+1, parseErr)
				}
			}

			// Save previous group if exists
			if currentGroup != nil && len(currentGroup.Upstreams) > 0 {
				if addErr := ugc.AddGroup(currentGroup); addErr != nil {
					return nil, fmt.Errorf("line %d: %w", lineNum+1, addErr)
				}
			}

			// Create new group
			currentGroup = &UpstreamGroup{
				Name:    groupName,
				Mode:    mode,
				Timeout: timeout,
				Enabled: true,
			}

			continue
		}

		// Check for domain-group mapping
		if strings.HasPrefix(line, "[/") && strings.Contains(line, "/]") {
			parts := strings.SplitN(line[2:], "/]", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: invalid domain-group mapping", lineNum+1)
			}

			domains := strings.Split(parts[0], "/")
			groupName := strings.TrimSpace(parts[1])

			for _, domain := range domains {
				domain = strings.TrimSpace(domain)
				if domain != "" {
					ugc.DomainGroups[domain] = groupName
				}
			}

			continue
		}

		// Check for default group setting
		if strings.HasPrefix(line, "[default]") {
			groupName := strings.TrimSpace(strings.TrimPrefix(line, "[default]"))
			ugc.DefaultGroup = groupName
			continue
		}

		// Otherwise, it's an upstream address
		if currentGroup == nil {
			return nil, fmt.Errorf("line %d: upstream defined before group", lineNum+1)
		}

		// Check if upstream already exists
		if u, exists := upstreamIndex[line]; exists {
			currentGroup.Upstreams = append(currentGroup.Upstreams, u)
			continue
		}

		// Create new upstream
		u, createErr := upstream.AddressToUpstream(line, opts.Clone())
		if createErr != nil {
			return nil, fmt.Errorf("line %d: creating upstream: %w", lineNum+1, createErr)
		}

		upstreamIndex[line] = u
		currentGroup.Upstreams = append(currentGroup.Upstreams, u)
	}

	// Save last group
	if currentGroup != nil && len(currentGroup.Upstreams) > 0 {
		if err = ugc.AddGroup(currentGroup); err != nil {
			return nil, err
		}
	}

	// Validate
	if err = ugc.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	// Rebuild trie for optimized domain matching
	ugc.RebuildTrie()

	return ugc, nil
}
