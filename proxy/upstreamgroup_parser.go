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
	// Name is the unique identifier for this group.
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

	// DomainGroups maps domain patterns to group names.
	DomainGroups map[string]string `yaml:"domain_groups"`

	// DefaultGroup is the name of the default group.
	DefaultGroup string `yaml:"default_group"`
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

	// Parse each group
	for i, groupSpec := range spec.Groups {
		group, parseErr := parseUpstreamGroup(&groupSpec, opts)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing group at index %d: %w", i, parseErr)
		}

		if addErr := ugc.AddGroup(group); addErr != nil {
			return nil, fmt.Errorf("adding group %q: %w", group.Name, addErr)
		}
	}

	// Load domains from files if specified
	expandedDomainGroups, err := LoadDomainsFromConfig(spec.DomainGroups, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("loading domain files: %w", err)
	}

	// Set domain-group mappings
	for domain, groupName := range expandedDomainGroups {
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

	return ugc, nil
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
