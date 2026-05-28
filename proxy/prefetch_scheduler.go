package proxy

import (
	"log/slog"
	"sync/atomic"
)

// prefetchScheduler manages on-demand prefetch tasks.
// No periodic scanning - prefetch is triggered only when domains are accessed.
// activeTasks uses atomic int32 to track concurrent prefetch operations.
type prefetchScheduler struct {
	config *PrefetchConfig
	logger *slog.Logger

	// executor performs the actual DNS refresh for a domain+qtype.
	executor func(domain string, qtype uint16)

	running     atomic.Bool
	activeTasks atomic.Int32
}

func newPrefetchScheduler(
	config *PrefetchConfig,
	logger *slog.Logger,
) *prefetchScheduler {
	return &prefetchScheduler{
		config: config,
		logger: logger,
	}
}

func (ps *prefetchScheduler) start() {
	if !ps.running.CompareAndSwap(false, true) {
		return
	}
	ps.logger.Info("prefetch scheduler started (on-demand mode)")
}

func (ps *prefetchScheduler) stop() {
	ps.running.Store(false)
	ps.logger.Info("prefetch scheduler stopped")
}

// triggerPrefetch triggers an on-demand prefetch for a specific domain+qtype.
// Returns false if max concurrent limit is reached.
func (ps *prefetchScheduler) triggerPrefetch(domain string, qtype uint16) bool {
	if ps.executor == nil {
		return false
	}

	// Check concurrent limit
	current := ps.activeTasks.Load()
	if current >= int32(ps.config.MaxConcurrent) {
		ps.logger.Debug("prefetch skipped: max concurrent reached",
			"domain", domain,
			"qtype", qtype,
			"active", current)
		return false
	}

	ps.activeTasks.Add(1)
	go func() {
		defer ps.activeTasks.Add(-1)
		ps.logger.Debug("prefetch triggered", "domain", domain, "qtype", qtype)
		ps.executor(domain, qtype)
		ps.logger.Debug("prefetch completed", "domain", domain, "qtype", qtype)
	}()

	return true
}

// ActiveTasks returns the number of in-flight prefetch goroutines.
func (ps *prefetchScheduler) ActiveTasks() int {
	return int(ps.activeTasks.Load())
}
