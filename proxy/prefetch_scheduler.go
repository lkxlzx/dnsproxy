package proxy

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// prefetchScheduler manages background prefetch tasks.
// activeTasks uses atomic int32 to avoid a separate mutex.
// The scheduler never touches extEntries directly; it delegates TTL checks
// to the shouldPrefetch callback injected by cachePrefetch.
type prefetchScheduler struct {
	config      *PrefetchConfig
	heatTracker *heatTracker
	logger      *slog.Logger

	// executor performs the actual DNS refresh for a domain+qtype.
	executor func(domain string, qtype uint16)

	// shouldPrefetch checks whether a domain+qtype needs refreshing now.
	// Injected by cachePrefetch so the scheduler never touches extEntries.
	shouldPrefetch func(domain string, qtype uint16) bool

	// onEvict is called when a domain+qtype is evicted from the prefetch
	// queue due to inactivity. May be nil.
	onEvict func(domain string, qtype uint16)

	running     atomic.Bool
	activeTasks atomic.Int32

	// scanBuf is reused across scan() calls to reduce per-scan allocations.
	scanBuf []heatSnapshot
	scanMu  sync.Mutex
}

func newPrefetchScheduler(
	config *PrefetchConfig,
	heatTracker *heatTracker,
	logger *slog.Logger,
) *prefetchScheduler {
	return &prefetchScheduler{
		config:      config,
		heatTracker: heatTracker,
		logger:      logger,
	}
}

func (ps *prefetchScheduler) start(ctx context.Context) {
	if !ps.running.CompareAndSwap(false, true) {
		return
	}
	go ps.scanLoop(ctx)
	go ps.inactivityCheckLoop(ctx)
	ps.logger.Info("prefetch scheduler started")
}

func (ps *prefetchScheduler) stop() {
	ps.running.Store(false)
	ps.logger.Info("prefetch scheduler stopped")
}

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

func (ps *prefetchScheduler) inactivityCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(ps.config.InactivityCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := ps.heatTracker.checkInactivity(time.Now())
			if len(removed) > 0 {
				ps.logger.Debug("removed inactive domains", "count", len(removed))
				// Notify cachePrefetch to clean up extEntries
				if ps.onEvict != nil {
					for _, key := range removed {
						d, q := splitKey(key)
						ps.onEvict(d, q)
					}
				}
			}
		}
	}
}

func (ps *prefetchScheduler) scan() {
	if ps.executor == nil || ps.shouldPrefetch == nil {
		return
	}

	candidates := ps.heatTracker.getPrefetchCandidates()
	if len(candidates) == 0 {
		return
	}

	ps.scanMu.Lock()
	needPrefetch := ps.scanBuf[:0]

	for i := range candidates {
		snap := &candidates[i]
		if ps.shouldPrefetch(snap.domain, snap.qtype) {
			needPrefetch = append(needPrefetch, *snap)
		}
	}

	if len(needPrefetch) == 0 {
		ps.scanBuf = needPrefetch
		ps.scanMu.Unlock()
		return
	}

	// Sort by heat score descending so hottest domains are refreshed first.
	sort.Slice(needPrefetch, func(i, j int) bool {
		return needPrefetch[i].heatScore > needPrefetch[j].heatScore
	})

	available := int(int32(ps.config.MaxConcurrent) - ps.activeTasks.Load())
	if available <= 0 {
		ps.scanBuf = needPrefetch
		ps.scanMu.Unlock()
		return
	}
	if len(needPrefetch) > available {
		needPrefetch = needPrefetch[:available]
	}

	// Copy before releasing lock so goroutines below have stable data.
	toFire := make([]heatSnapshot, len(needPrefetch))
	copy(toFire, needPrefetch)
	ps.scanBuf = needPrefetch
	ps.scanMu.Unlock()

	for _, snap := range toFire {
		ps.triggerPrefetch(snap.domain, snap.qtype)
	}
}

func (ps *prefetchScheduler) triggerPrefetch(domain string, qtype uint16) {
	ps.activeTasks.Add(1)
	go func() {
		defer ps.activeTasks.Add(-1)
		ps.logger.Debug("prefetch triggered", "domain", domain, "qtype", qtype)
		ps.executor(domain, qtype)
		ps.logger.Debug("prefetch completed", "domain", domain, "qtype", qtype)
	}()
}

// ActiveTasks returns the number of in-flight prefetch goroutines.
func (ps *prefetchScheduler) ActiveTasks() int {
	return int(ps.activeTasks.Load())
}
