package proxy

import (
	"fmt"
	"sync"
)

// UpstreamGroup represents a named group of upstream DNS servers.
// This allows reusing upstream configurations across multiple domain groups.
// Supports primary-fallback mode where fallback upstreams are only used when all primary upstreams fail.
type UpstreamGroup struct {
	// ID is the unique identifier for this upstream group
	ID string

	// Upstreams is the list of primary upstream DNS server addresses
	Upstreams []string

	// FallbackUpstreams is the list of fallback upstream DNS server addresses
	// These are only used when all primary upstreams fail
	FallbackUpstreams []string

	// Description is an optional description of this group
	Description string
}

// UpstreamGroupManager manages upstream groups and provides lookup by ID.
type UpstreamGroupManager struct {
	mu     sync.RWMutex
	groups map[string]*UpstreamGroup
}

// NewUpstreamGroupManager creates a new upstream group manager.
func NewUpstreamGroupManager() *UpstreamGroupManager {
	return &UpstreamGroupManager{
		groups: make(map[string]*UpstreamGroup),
	}
}

// AddGroup adds or updates an upstream group.
func (m *UpstreamGroupManager) AddGroup(group *UpstreamGroup) error {
	if group.ID == "" {
		return fmt.Errorf("upstream group ID cannot be empty")
	}

	if len(group.Upstreams) == 0 {
		return fmt.Errorf("upstream group %s must have at least one upstream", group.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.groups[group.ID] = group
	return nil
}

// GetGroup retrieves an upstream group by ID.
func (m *UpstreamGroupManager) GetGroup(id string) (*UpstreamGroup, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	group, ok := m.groups[id]
	return group, ok
}

// GetUpstreams retrieves the upstream addresses for a group ID.
// Returns primary upstreams only (fallback upstreams are handled separately).
func (m *UpstreamGroupManager) GetUpstreams(id string) ([]string, error) {
	group, ok := m.GetGroup(id)
	if !ok {
		return nil, fmt.Errorf("upstream group not found: %s", id)
	}

	return group.Upstreams, nil
}

// GetFallbackUpstreams retrieves the fallback upstream addresses for a group ID.
func (m *UpstreamGroupManager) GetFallbackUpstreams(id string) ([]string, error) {
	group, ok := m.GetGroup(id)
	if !ok {
		return nil, fmt.Errorf("upstream group not found: %s", id)
	}

	return group.FallbackUpstreams, nil
}

// GetAllUpstreams retrieves both primary and fallback upstreams for a group ID.
// Returns (primaryUpstreams, fallbackUpstreams, error).
func (m *UpstreamGroupManager) GetAllUpstreams(id string) ([]string, []string, error) {
	group, ok := m.GetGroup(id)
	if !ok {
		return nil, nil, fmt.Errorf("upstream group not found: %s", id)
	}

	return group.Upstreams, group.FallbackUpstreams, nil
}

// ListGroups returns all upstream group IDs.
func (m *UpstreamGroupManager) ListGroups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.groups))
	for id := range m.groups {
		ids = append(ids, id)
	}

	return ids
}

// RemoveGroup removes an upstream group by ID.
func (m *UpstreamGroupManager) RemoveGroup(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.groups[id]; !ok {
		return false
	}

	delete(m.groups, id)
	return true
}

// ResolveUpstreams resolves upstream addresses from either a group ID or direct addresses.
// If groupID is not empty, it looks up the group. Otherwise, it returns the direct addresses.
// Returns (primaryUpstreams, fallbackUpstreams, error).
func (m *UpstreamGroupManager) ResolveUpstreams(groupID string, directUpstreams []string) ([]string, []string, error) {
	if groupID != "" {
		primary, fallback, err := m.GetAllUpstreams(groupID)
		return primary, fallback, err
	}

	if len(directUpstreams) == 0 {
		return nil, nil, fmt.Errorf("no upstream group ID or direct upstreams specified")
	}

	// Direct upstreams have no fallback
	return directUpstreams, nil, nil
}

// CreateUpstreamConfig creates an UpstreamConfig from resolved upstream addresses.
// Note: This is a helper function. The actual UpstreamConfig creation should be done
// using the existing ParseUpstreamsConfig or similar methods from the proxy package.
func CreateUpstreamConfig(upstreamAddrs []string) ([]string, error) {
	if len(upstreamAddrs) == 0 {
		return nil, fmt.Errorf("no upstream addresses provided")
	}

	return upstreamAddrs, nil
}
