package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/errors"
)

// UpstreamGroup represents a group of upstream servers with shared configuration.
type UpstreamGroup struct {
	// ID is the unique identifier for this group (UUID format recommended).
	// Used for integration with external systems like AdGuard Home.
	ID string

	// Name is the human-readable name for this group.
	Name string

	// Upstreams is the list of upstream servers in this group.
	Upstreams []upstream.Upstream

	// Mode determines the logic through which upstreams in this group will be used.
	Mode UpstreamMode

	// Timeout is the timeout for queries to upstreams in this group.
	Timeout time.Duration

	// MaxRetries is the maximum number of retries for failed queries.
	MaxRetries int

	// Enabled indicates whether this group is active.
	Enabled bool

	// Priority is used for fallback ordering. Lower values have higher priority.
	Priority int
}

// UpstreamGroupConfig contains configuration for upstream groups.
type UpstreamGroupConfig struct {
	// Groups maps group names to their configurations.
	Groups map[string]*UpstreamGroup

	// DomainGroups maps domain patterns to group names.
	// Format: domain -> group name
	DomainGroups map[string]string

	// DefaultGroup is the name of the group to use when no specific group is matched.
	DefaultGroup string

	// domainTrie is an optimized trie structure for fast domain matching.
	// It's built from DomainGroups for O(m) lookup time where m is domain length.
	domainTrie *DomainTrie
	
	// domainRadix is an ultra-optimized radix tree for extreme performance.
	// It provides O(1) exact match and compressed wildcard matching.
	// Use this for maximum performance with large domain lists (10K+).
	domainRadix *RadixTree
	
	// useRadix determines whether to use Radix Tree (true) or Trie (false).
	// Radix Tree is 4-5x faster for exact matches but uses slightly more memory.
	useRadix bool
}

// type check
var _ io.Closer = (*UpstreamGroupConfig)(nil)

// NewUpstreamGroupConfig creates a new UpstreamGroupConfig with initialized maps.
// By default, it uses Radix Tree for maximum performance.
func NewUpstreamGroupConfig() *UpstreamGroupConfig {
	return &UpstreamGroupConfig{
		Groups:       make(map[string]*UpstreamGroup),
		DomainGroups: make(map[string]string),
		domainRadix:  NewRadixTree(),
		useRadix:     true, // Use Radix by default for best performance
	}
}

// NewUpstreamGroupConfigWithTrie creates a config using Trie instead of Radix.
// Use this if you prefer Trie's characteristics or for compatibility.
func NewUpstreamGroupConfigWithTrie() *UpstreamGroupConfig {
	return &UpstreamGroupConfig{
		Groups:       make(map[string]*UpstreamGroup),
		DomainGroups: make(map[string]string),
		domainTrie:   NewDomainTrie(),
		useRadix:     false,
	}
}

// AddGroup adds a new upstream group to the configuration.
func (ugc *UpstreamGroupConfig) AddGroup(group *UpstreamGroup) error {
	if group == nil {
		return errors.Error("group cannot be nil")
	}

	if group.Name == "" {
		return errors.Error("group name cannot be empty")
	}

	if _, exists := ugc.Groups[group.Name]; exists {
		return fmt.Errorf("group %q already exists", group.Name)
	}

	if len(group.Upstreams) == 0 {
		return fmt.Errorf("group %q has no upstreams", group.Name)
	}

	ugc.Groups[group.Name] = group

	return nil
}

// SetDomainGroup associates a domain pattern with a group.
func (ugc *UpstreamGroupConfig) SetDomainGroup(domain, groupName string) error {
	if domain == "" {
		return errors.Error("domain cannot be empty")
	}

	if groupName == "" {
		return errors.Error("group name cannot be empty")
	}

	if _, exists := ugc.Groups[groupName]; !exists {
		return fmt.Errorf("group %q does not exist", groupName)
	}

	// Normalize domain: remove trailing dot and convert to lowercase
	// This ensures consistent storage format regardless of input format
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	ugc.DomainGroups[domain] = groupName
	
	// Update the active data structure
	if ugc.useRadix && ugc.domainRadix != nil {
		ugc.domainRadix.Insert(domain, groupName)
	} else if ugc.domainTrie != nil {
		ugc.domainTrie.Insert(domain, groupName)
	}

	return nil
}

// GetGroupForDomain returns the upstream group for the given domain.
// If no specific group is found, it returns the default group.
// Supports exact match and wildcard matching (*.example.com).
//
// Performance: 
//   - Radix Tree: O(1) for exact match, O(k) for wildcard
//   - Trie Tree: O(m) where m is domain length
func (ugc *UpstreamGroupConfig) GetGroupForDomain(domain string) (*UpstreamGroup, error) {
	// Normalize domain: remove trailing dot for matching
	// DNS queries use FQDN format (with trailing dot), but we store domains without it
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	
	// Try Radix Tree first (if enabled) - fastest option
	if ugc.useRadix && ugc.domainRadix != nil {
		if groupName, found := ugc.domainRadix.Search(domain); found {
			if group, exists := ugc.Groups[groupName]; exists {
				if group.Enabled {
					return group, nil
				}
			}
		}
	} else if ugc.domainTrie != nil {
		// Fallback to Trie Tree
		if groupName, found := ugc.domainTrie.Search(domain); found {
			if group, exists := ugc.Groups[groupName]; exists {
				if group.Enabled {
					return group, nil
				}
			}
		}
	}

	// Fallback to map-based lookup (for backward compatibility)
	// This path is only used if neither Radix nor Trie is initialized
	
	// Try exact match first
	if groupName, ok := ugc.DomainGroups[domain]; ok {
		if group, exists := ugc.Groups[groupName]; exists {
			if group.Enabled {
				return group, nil
			}
		}
	}

	// Try wildcard matching
	// For domain "www.example.com", try:
	// 1. "*.example.com" and "*.example.com."
	// 2. "*.com" and "*.com."
	labels := strings.Split(domain, ".")
	for i := 1; i < len(labels); i++ {
		wildcard := "*." + strings.Join(labels[i:], ".")
		
		// Try without trailing dot
		if groupName, ok := ugc.DomainGroups[wildcard]; ok {
			if group, exists := ugc.Groups[groupName]; exists {
				if group.Enabled {
					return group, nil
				}
			}
		}
		
		// Try with trailing dot
		wildcardWithDot := wildcard + "."
		if groupName, ok := ugc.DomainGroups[wildcardWithDot]; ok {
			if group, exists := ugc.Groups[groupName]; exists {
				if group.Enabled {
					return group, nil
				}
			}
		}
	}

	// Return default group
	if ugc.DefaultGroup != "" {
		if group, exists := ugc.Groups[ugc.DefaultGroup]; exists {
			if group.Enabled {
				return group, nil
			}
			return nil, fmt.Errorf("default group %q is disabled", ugc.DefaultGroup)
		}
		return nil, fmt.Errorf("default group %q not found", ugc.DefaultGroup)
	}

	return nil, errors.Error("no suitable group found for domain")
}

// Validate checks if the configuration is valid.
func (ugc *UpstreamGroupConfig) Validate() error {
	if ugc == nil {
		return errors.ErrNoValue
	}

	if len(ugc.Groups) == 0 {
		return errors.Error("no upstream groups defined")
	}

	if ugc.DefaultGroup == "" {
		return errors.Error("default group not specified")
	}

	if _, exists := ugc.Groups[ugc.DefaultGroup]; !exists {
		return fmt.Errorf("default group %q does not exist", ugc.DefaultGroup)
	}

	// Validate each group
	for name, group := range ugc.Groups {
		if group.Name != name {
			return fmt.Errorf("group name mismatch: key=%q, name=%q", name, group.Name)
		}

		if len(group.Upstreams) == 0 {
			return fmt.Errorf("group %q has no upstreams", name)
		}

		// Validate mode
		switch group.Mode {
		case "", UpstreamModeLoadBalance, UpstreamModeParallel, UpstreamModeFastestAddr:
			// Valid modes
		default:
			return fmt.Errorf("group %q has invalid mode: %q", name, group.Mode)
		}
	}

	// Validate domain group references
	for domain, groupName := range ugc.DomainGroups {
		if _, exists := ugc.Groups[groupName]; !exists {
			return fmt.Errorf("domain %q references non-existent group %q", domain, groupName)
		}
	}

	return nil
}

// Close implements the io.Closer interface for *UpstreamGroupConfig.
func (ugc *UpstreamGroupConfig) Close() error {
	if ugc == nil {
		return nil
	}

	var errs []error
	for name, group := range ugc.Groups {
		for i, u := range group.Upstreams {
			if err := u.Close(); err != nil {
				errs = append(errs, fmt.Errorf("group %q upstream %d: %w", name, i, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close some upstreams: %w", errors.Join(errs...))
	}

	return nil
}

// LogGroupInfo logs information about all configured groups.
func (ugc *UpstreamGroupConfig) LogGroupInfo(logger *slog.Logger) {
	if ugc == nil || logger == nil {
		return
	}

	logger.Info("upstream groups configured", "count", len(ugc.Groups), "default", ugc.DefaultGroup)

	for name, group := range ugc.Groups {
		logger.Info(
			"upstream group",
			"name", name,
			"upstreams", len(group.Upstreams),
			"mode", group.Mode,
			"enabled", group.Enabled,
			"priority", group.Priority,
			"timeout", group.Timeout,
		)
	}

	if len(ugc.DomainGroups) > 0 {
		logger.Info("domain-specific groups", "count", len(ugc.DomainGroups))
		if ugc.useRadix && ugc.domainRadix != nil {
			stats := ugc.domainRadix.Stats()
			logger.Info("domain radix tree", 
				"exact", stats.ExactMatches,
				"wildcard", stats.WildcardPatterns,
				"total", stats.TotalDomains,
				"depth", stats.TreeDepth)
		} else if ugc.domainTrie != nil {
			logger.Info("domain trie", "size", ugc.domainTrie.Size())
		}
	}
}

// RebuildTrie rebuilds the domain trie from the current DomainGroups map.
// This is useful when domains are loaded in bulk (e.g., from a file).
// Call this method after loading all domains to optimize lookup performance.
func (ugc *UpstreamGroupConfig) RebuildTrie() {
	if ugc == nil {
		return
	}

	if ugc.useRadix {
		// Rebuild Radix Tree
		ugc.domainRadix = NewRadixTree()
		for domain, groupName := range ugc.DomainGroups {
			ugc.domainRadix.Insert(domain, groupName)
		}
	} else {
		// Rebuild Trie
		ugc.domainTrie = NewDomainTrie()
		for domain, groupName := range ugc.DomainGroups {
			ugc.domainTrie.Insert(domain, groupName)
		}
	}
}

// EnableRadixTree switches to using Radix Tree for maximum performance.
// This provides 4-5x faster lookups compared to Trie.
func (ugc *UpstreamGroupConfig) EnableRadixTree() {
	if ugc == nil || ugc.useRadix {
		return
	}

	ugc.useRadix = true
	ugc.domainRadix = NewRadixTree()
	
	// Migrate existing domains
	for domain, groupName := range ugc.DomainGroups {
		ugc.domainRadix.Insert(domain, groupName)
	}
}

// EnableTrieTree switches to using Trie Tree.
// Use this if you prefer Trie's characteristics.
func (ugc *UpstreamGroupConfig) EnableTrieTree() {
	if ugc == nil || !ugc.useRadix {
		return
	}

	ugc.useRadix = false
	ugc.domainTrie = NewDomainTrie()
	
	// Migrate existing domains
	for domain, groupName := range ugc.DomainGroups {
		ugc.domainTrie.Insert(domain, groupName)
	}
}
