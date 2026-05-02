package proxy

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// prefetchScheduler manages the background prefetch tasks.
type prefetchScheduler struct {
	// Configuration
	config *PrefetchConfig

	// Dependencies
	proxy        *Proxy
	globalClock  *globalClock
	heatTracker  *heatTracker
	logger       *slog.Logger
	
	// Executor function (injected)
	executor     func(*cacheEntryExt)

	// State
	running      bool
	runningMu    sync.RWMutex
	activeTasks  int
	activeTasksMu sync.Mutex
}

// newPrefetchScheduler creates a new prefetch scheduler.
func newPrefetchScheduler(
	config *PrefetchConfig,
	proxy *Proxy,
	globalClock *globalClock,
	heatTracker *heatTracker,
	logger *slog.Logger,
) *prefetchScheduler {
	return &prefetchScheduler{
		config:      config,
		proxy:       proxy,
		globalClock: globalClock,
		heatTracker: heatTracker,
		logger:      logger,
		running:     false,
		activeTasks: 0,
	}
}

// start begins the prefetch scheduler.
func (ps *prefetchScheduler) start(ctx context.Context) {
	ps.runningMu.Lock()
	if ps.running {
		ps.runningMu.Unlock()
		return
	}
	ps.running = true
	ps.runningMu.Unlock()

	// Start scan loop
	go ps.scanLoop(ctx)

	// Start inactivity check loop
	go ps.inactivityCheckLoop(ctx)

	ps.logger.Info("prefetch scheduler started")
}

// stop stops the prefetch scheduler.
func (ps *prefetchScheduler) stop() {
	ps.runningMu.Lock()
	ps.running = false
	ps.runningMu.Unlock()

	ps.logger.Info("prefetch scheduler stopped")
}

// scanLoop periodically scans for entries that need prefetching.
func (ps *prefetchScheduler) scanLoop(ctx context.Context) {
	ticker := time.NewTicker(ps.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ps.scan()
		}
	}
}

// inactivityCheckLoop periodically checks for inactive domains.
func (ps *prefetchScheduler) inactivityCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(ps.config.InactivityCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ps.checkInactivity()
		}
	}
}

// scan scans the prefetch queue and triggers prefetch for eligible entries.
func (ps *prefetchScheduler) scan() {
	currentT := ps.globalClock.get()
	candidates := ps.heatTracker.getPrefetchCandidates()

	if len(candidates) == 0 {
		return
	}

	// Filter candidates that need prefetching
	var needPrefetch []*cacheEntryExt
	for _, entry := range candidates {
		if ps.shouldPrefetch(entry, currentT) {
			needPrefetch = append(needPrefetch, entry)
		}
	}

	if len(needPrefetch) == 0 {
		return
	}

	// Sort by heat score (descending)
	sort.Slice(needPrefetch, func(i, j int) bool {
		return needPrefetch[i].heatScore > needPrefetch[j].heatScore
	})

	// Limit by max concurrent tasks
	ps.activeTasksMu.Lock()
	available := ps.config.MaxConcurrent - ps.activeTasks
	ps.activeTasksMu.Unlock()

	if available <= 0 {
		return
	}

	if len(needPrefetch) > available {
		needPrefetch = needPrefetch[:available]
	}

	// Trigger prefetch for selected entries
	for _, entry := range needPrefetch {
		ps.triggerPrefetch(entry)
	}
}

// shouldPrefetch determines if an entry should be prefetched.
func (ps *prefetchScheduler) shouldPrefetch(entry *cacheEntryExt, currentT uint32) bool {
	if !entry.inPrefetchQueue {
		return false
	}

	if entry.isExpired(currentT) {
		return false // Already expired, will be handled by normal query
	}

	remainingTTL := entry.remainingTTL(currentT)

	// Check fixed threshold (use < not <=)
	fixedThreshold := ps.config.ThresholdSeconds
	if remainingTTL < fixedThreshold {
		return true
	}

	// Check percentage threshold
	percentThreshold := entry.originalTTL * ps.config.ThresholdPercent / 100
	if remainingTTL < percentThreshold {
		return true
	}

	return false
}

// triggerPrefetch triggers a background prefetch for the given entry.
func (ps *prefetchScheduler) triggerPrefetch(entry *cacheEntryExt) {
	ps.activeTasksMu.Lock()
	ps.activeTasks++
	ps.activeTasksMu.Unlock()

	go func() {
		defer func() {
			ps.activeTasksMu.Lock()
			ps.activeTasks--
			ps.activeTasksMu.Unlock()
		}()

		ps.executePrefetch(entry)
	}()
}

// executePrefetch performs the actual prefetch operation.
func (ps *prefetchScheduler) executePrefetch(entry *cacheEntryExt) {
	ps.logger.Debug("prefetch triggered",
		"domain", entry.domain,
		"qtype", entry.qtype,
		"heat", entry.heatScore)

	// Call the injected executor function
	if ps.executor != nil {
		ps.executor(entry)
	} else {
		ps.logger.Warn("prefetch executor not set, skipping",
			"domain", entry.domain)
	}

	ps.logger.Debug("prefetch completed",
		"domain", entry.domain,
		"qtype", entry.qtype)
}

// checkInactivity checks for and removes inactive domains from the prefetch queue.
func (ps *prefetchScheduler) checkInactivity() {
	now := time.Now()
	removed := ps.heatTracker.checkInactivity(now)

	if len(removed) > 0 {
		ps.logger.Debug("removed inactive domains from prefetch queue",
			"count", len(removed))
	}
}
