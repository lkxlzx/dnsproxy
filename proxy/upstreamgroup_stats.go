package proxy

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// GroupStats contains statistics for an upstream group.
type GroupStats struct {
	// Name is the group name.
	Name string

	// TotalQueries is the total number of queries sent to this group.
	TotalQueries uint64

	// SuccessfulQueries is the number of successful queries.
	SuccessfulQueries uint64

	// FailedQueries is the number of failed queries.
	FailedQueries uint64

	// TotalLatency is the cumulative latency in milliseconds.
	TotalLatency uint64

	// AverageLatency is the average query latency in milliseconds.
	AverageLatency float64

	// MinLatency is the minimum query latency in milliseconds.
	MinLatency uint64

	// MaxLatency is the maximum query latency in milliseconds.
	MaxLatency uint64

	// LastQueryTime is the time of the last query.
	LastQueryTime time.Time

	// UpstreamStats contains per-upstream statistics.
	UpstreamStats map[string]*UpstreamStats

	mu sync.RWMutex
}

// UpstreamStats contains statistics for a single upstream server.
type UpstreamStats struct {
	// Address is the upstream server address.
	Address string

	// TotalQueries is the total number of queries.
	TotalQueries uint64

	// SuccessfulQueries is the number of successful queries.
	SuccessfulQueries uint64

	// FailedQueries is the number of failed queries.
	FailedQueries uint64

	// TotalLatency is the cumulative latency.
	TotalLatency uint64

	// AverageLatency is the average latency.
	AverageLatency float64

	// LastUsed is the time this upstream was last used.
	LastUsed time.Time
}

// NewGroupStats creates a new GroupStats instance.
func NewGroupStats(name string) *GroupStats {
	return &GroupStats{
		Name:          name,
		UpstreamStats: make(map[string]*UpstreamStats),
		MinLatency:    ^uint64(0), // Max uint64
	}
}

// RecordQuery records a query to the group.
func (gs *GroupStats) RecordQuery(upstreamAddr string, latency time.Duration, success bool) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	atomic.AddUint64(&gs.TotalQueries, 1)
	latencyMs := uint64(latency.Milliseconds())
	atomic.AddUint64(&gs.TotalLatency, latencyMs)

	if success {
		atomic.AddUint64(&gs.SuccessfulQueries, 1)
	} else {
		atomic.AddUint64(&gs.FailedQueries, 1)
	}

	// Update min/max latency
	if latencyMs < gs.MinLatency {
		gs.MinLatency = latencyMs
	}
	if latencyMs > gs.MaxLatency {
		gs.MaxLatency = latencyMs
	}

	gs.LastQueryTime = time.Now()

	// Calculate average latency
	if gs.TotalQueries > 0 {
		gs.AverageLatency = float64(gs.TotalLatency) / float64(gs.TotalQueries)
	}

	// Update upstream stats
	if _, exists := gs.UpstreamStats[upstreamAddr]; !exists {
		gs.UpstreamStats[upstreamAddr] = &UpstreamStats{
			Address: upstreamAddr,
		}
	}

	us := gs.UpstreamStats[upstreamAddr]
	atomic.AddUint64(&us.TotalQueries, 1)
	atomic.AddUint64(&us.TotalLatency, latencyMs)

	if success {
		atomic.AddUint64(&us.SuccessfulQueries, 1)
	} else {
		atomic.AddUint64(&us.FailedQueries, 1)
	}

	us.LastUsed = time.Now()

	if us.TotalQueries > 0 {
		us.AverageLatency = float64(us.TotalLatency) / float64(us.TotalQueries)
	}
}

// GetSnapshot returns a snapshot of the current statistics.
func (gs *GroupStats) GetSnapshot() GroupStatsSnapshot {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	upstreamSnapshots := make(map[string]UpstreamStatsSnapshot)
	for addr, us := range gs.UpstreamStats {
		upstreamSnapshots[addr] = UpstreamStatsSnapshot{
			Address:           us.Address,
			TotalQueries:      atomic.LoadUint64(&us.TotalQueries),
			SuccessfulQueries: atomic.LoadUint64(&us.SuccessfulQueries),
			FailedQueries:     atomic.LoadUint64(&us.FailedQueries),
			AverageLatency:    us.AverageLatency,
			LastUsed:          us.LastUsed,
		}
	}

	return GroupStatsSnapshot{
		Name:              gs.Name,
		TotalQueries:      atomic.LoadUint64(&gs.TotalQueries),
		SuccessfulQueries: atomic.LoadUint64(&gs.SuccessfulQueries),
		FailedQueries:     atomic.LoadUint64(&gs.FailedQueries),
		AverageLatency:    gs.AverageLatency,
		MinLatency:        gs.MinLatency,
		MaxLatency:        gs.MaxLatency,
		LastQueryTime:     gs.LastQueryTime,
		UpstreamStats:     upstreamSnapshots,
	}
}

// Reset resets all statistics.
func (gs *GroupStats) Reset() {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	atomic.StoreUint64(&gs.TotalQueries, 0)
	atomic.StoreUint64(&gs.SuccessfulQueries, 0)
	atomic.StoreUint64(&gs.FailedQueries, 0)
	atomic.StoreUint64(&gs.TotalLatency, 0)
	gs.AverageLatency = 0
	gs.MinLatency = ^uint64(0)
	gs.MaxLatency = 0
	gs.UpstreamStats = make(map[string]*UpstreamStats)
}

// GroupStatsSnapshot is an immutable snapshot of group statistics.
type GroupStatsSnapshot struct {
	Name              string
	TotalQueries      uint64
	SuccessfulQueries uint64
	FailedQueries     uint64
	AverageLatency    float64
	MinLatency        uint64
	MaxLatency        uint64
	LastQueryTime     time.Time
	UpstreamStats     map[string]UpstreamStatsSnapshot
}

// UpstreamStatsSnapshot is an immutable snapshot of upstream statistics.
type UpstreamStatsSnapshot struct {
	Address           string
	TotalQueries      uint64
	SuccessfulQueries uint64
	FailedQueries     uint64
	AverageLatency    float64
	LastUsed          time.Time
}

// SuccessRate returns the success rate as a percentage.
func (s GroupStatsSnapshot) SuccessRate() float64 {
	if s.TotalQueries == 0 {
		return 0
	}
	return float64(s.SuccessfulQueries) / float64(s.TotalQueries) * 100
}

// FailureRate returns the failure rate as a percentage.
func (s GroupStatsSnapshot) FailureRate() float64 {
	if s.TotalQueries == 0 {
		return 0
	}
	return float64(s.FailedQueries) / float64(s.TotalQueries) * 100
}

// String returns a human-readable representation of the statistics.
func (s GroupStatsSnapshot) String() string {
	return fmt.Sprintf(
		"Group: %s, Queries: %d, Success: %d (%.1f%%), Failed: %d (%.1f%%), Avg Latency: %.2fms",
		s.Name,
		s.TotalQueries,
		s.SuccessfulQueries,
		s.SuccessRate(),
		s.FailedQueries,
		s.FailureRate(),
		s.AverageLatency,
	)
}

// StatsCollector collects statistics for all groups.
type StatsCollector struct {
	groups map[string]*GroupStats
	mu     sync.RWMutex
}

// NewStatsCollector creates a new StatsCollector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		groups: make(map[string]*GroupStats),
	}
}

// GetOrCreateGroupStats gets or creates statistics for a group.
func (sc *StatsCollector) GetOrCreateGroupStats(groupName string) *GroupStats {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if stats, exists := sc.groups[groupName]; exists {
		return stats
	}

	stats := NewGroupStats(groupName)
	sc.groups[groupName] = stats
	return stats
}

// GetGroupStats returns statistics for a specific group.
func (sc *StatsCollector) GetGroupStats(groupName string) (*GroupStats, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if stats, exists := sc.groups[groupName]; exists {
		return stats, nil
	}

	return nil, fmt.Errorf("group %q not found", groupName)
}

// GetAllStats returns statistics for all groups.
func (sc *StatsCollector) GetAllStats() map[string]GroupStatsSnapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	result := make(map[string]GroupStatsSnapshot)
	for name, stats := range sc.groups {
		result[name] = stats.GetSnapshot()
	}

	return result
}

// ResetAll resets statistics for all groups.
func (sc *StatsCollector) ResetAll() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for _, stats := range sc.groups {
		stats.Reset()
	}
}

// ResetGroup resets statistics for a specific group.
func (sc *StatsCollector) ResetGroup(groupName string) error {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if stats, exists := sc.groups[groupName]; exists {
		stats.Reset()
		return nil
	}

	return fmt.Errorf("group %q not found", groupName)
}

// WeightedUpstream represents an upstream with a weight for load balancing.
type WeightedUpstream struct {
	Upstream upstream.Upstream
	Weight   int
	Stats    *UpstreamStats
}

// WeightedLoadBalancer performs weighted load balancing across upstreams.
type WeightedLoadBalancer struct {
	upstreams []*WeightedUpstream
	totalWeight int
	current   int
	mu        sync.Mutex
}

// NewWeightedLoadBalancer creates a new weighted load balancer.
func NewWeightedLoadBalancer(upstreams []upstream.Upstream, weights []int) *WeightedLoadBalancer {
	if len(weights) != len(upstreams) {
		// Default to equal weights
		weights = make([]int, len(upstreams))
		for i := range weights {
			weights[i] = 1
		}
	}

	wlb := &WeightedLoadBalancer{
		upstreams: make([]*WeightedUpstream, len(upstreams)),
	}

	for i, u := range upstreams {
		wlb.upstreams[i] = &WeightedUpstream{
			Upstream: u,
			Weight:   weights[i],
			Stats:    &UpstreamStats{Address: u.Address()},
		}
		wlb.totalWeight += weights[i]
	}

	return wlb
}

// Next returns the next upstream according to weighted round-robin.
func (wlb *WeightedLoadBalancer) Next() upstream.Upstream {
	wlb.mu.Lock()
	defer wlb.mu.Unlock()

	if len(wlb.upstreams) == 0 {
		return nil
	}

	if len(wlb.upstreams) == 1 {
		return wlb.upstreams[0].Upstream
	}

	// Weighted round-robin
	wlb.current = (wlb.current + 1) % wlb.totalWeight

	cumulative := 0
	for _, wu := range wlb.upstreams {
		cumulative += wu.Weight
		if wlb.current < cumulative {
			return wu.Upstream
		}
	}

	return wlb.upstreams[0].Upstream
}

// GetAll returns all upstreams.
func (wlb *WeightedLoadBalancer) GetAll() []upstream.Upstream {
	wlb.mu.Lock()
	defer wlb.mu.Unlock()

	result := make([]upstream.Upstream, len(wlb.upstreams))
	for i, wu := range wlb.upstreams {
		result[i] = wu.Upstream
	}

	return result
}
