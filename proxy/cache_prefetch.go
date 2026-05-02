package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/logutil/slogutil"
	"github.com/miekg/dns"
)

// cachePrefetch wraps the base cache with smart prefetch capabilities.
//
// Design invariants:
//   - extEntries is sharded (16 shards) to reduce lock contention.
//   - heatTracker owns all heat state; cachePrefetch never holds both
//     shard.mu and heatTracker.mu simultaneously.
//   - TTL tracking uses real Unix timestamps per entry (no global clock).
//   - extEntryShard entries are cleaned up via onEvict callback.
type cachePrefetch struct {
	*cache

	config      *PrefetchConfig
	heatTracker *heatTracker
	scheduler   *prefetchScheduler
	proxy       *Proxy
	logger      *slog.Logger

	shardCount int
	extEntries []*extEntryShard

	ctx    context.Context
	cancel context.CancelFunc
}

// extEntryShard is a shard of the extEntries map.
type extEntryShard struct {
	entries map[string]*cacheEntryExt
	mu      sync.RWMutex
}

func newExtEntryShard() *extEntryShard {
	return &extEntryShard{
		entries: make(map[string]*cacheEntryExt),
	}
}

// validatePrefetchConfig validates the prefetch configuration values.
func validatePrefetchConfig(c *PrefetchConfig) error {
	if c.ThresholdSeconds == 0 && c.ThresholdPercent == 0 {
		return fmt.Errorf("prefetch: at least one threshold must be > 0")
	}
	if c.ThresholdPercent > 100 {
		return fmt.Errorf("prefetch: threshold_percent must be <= 100")
	}
	if c.MaxConcurrent <= 0 {
		return fmt.Errorf("prefetch: max_concurrent must be > 0")
	}
	if c.MinHeatThreshold <= 0 {
		return fmt.Errorf("prefetch: min_heat_threshold must be > 0")
	}
	if c.ScanInterval <= 0 {
		return fmt.Errorf("prefetch: scan_interval must be > 0")
	}
	if c.TimeWindow <= 0 {
		return fmt.Errorf("prefetch: time_window must be > 0")
	}
	return nil
}

func newCachePrefetch(
	baseCache *cache,
	config *PrefetchConfig,
	proxy *Proxy,
	logger *slog.Logger,
) *cachePrefetch {
	if config == nil || !config.Enabled {
		return nil
	}
	if err := validatePrefetchConfig(config); err != nil {
		logger.Error("invalid prefetch config", slogutil.KeyError, err)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	shardCount := 16
	shards := make([]*extEntryShard, shardCount)
	for i := range shards {
		shards[i] = newExtEntryShard()
	}

	cp := &cachePrefetch{
		cache:      baseCache,
		config:     config,
		proxy:      proxy,
		logger:     logger,
		extEntries: shards,
		shardCount: shardCount,
		ctx:        ctx,
		cancel:     cancel,
	}

	cp.heatTracker = newHeatTracker(config.MinHeatThreshold, config.TimeWindow)

	cp.scheduler = newPrefetchScheduler(config, cp.heatTracker, logger)
	cp.scheduler.executor = cp.executePrefetchQuery
	cp.scheduler.shouldPrefetch = cp.shouldPrefetchDomain
	cp.scheduler.onEvict = cp.onDomainEvicted

	cp.scheduler.start(ctx)

	logger.Info("cache prefetch enabled",
		"threshold_seconds", config.ThresholdSeconds,
		"threshold_percent", config.ThresholdPercent,
		"min_heat", config.MinHeatThreshold,
		"max_concurrent", config.MaxConcurrent)

	return cp
}

// ────────────────────────────────────────────────────────────────────────────
// ── shard helpers ───────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) getShard(key string) *extEntryShard {
	h := uint32(0)
	for i := 0; i < len(key); i++ {
		h = h*31 + uint32(key[i])
	}
	return cp.extEntries[h%uint32(cp.shardCount)]
}

// ────────────────────────────────────────────────────────────────────────────
// ── cacheInterface implementation ───────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) get(req *dns.Msg) (*cacheItem, bool, []byte) {
	ci, expired, key := cp.cache.get(req)
	if ci != nil && len(req.Question) > 0 {
		q := req.Question[0]
		cp.recordAccess(q.Name, q.Qtype)
	}
	return ci, expired, key
}

func (cp *cachePrefetch) getWithSubnet(req *dns.Msg, subnet *net.IPNet) (*cacheItem, bool, []byte) {
	ci, expired, key := cp.cache.getWithSubnet(req, subnet)
	if ci != nil && len(req.Question) > 0 {
		q := req.Question[0]
		cp.recordAccess(q.Name, q.Qtype)
	}
	return ci, expired, key
}

func (cp *cachePrefetch) set(m *dns.Msg, u upstream.Upstream, l *slog.Logger) {
	cp.cache.set(m, u, l)
}

func (cp *cachePrefetch) setWithSubnet(m *dns.Msg, u upstream.Upstream, subnet *net.IPNet, l *slog.Logger) {
	cp.cache.setWithSubnet(m, u, subnet, l)
}

func (cp *cachePrefetch) clearItems()           { cp.cache.clearItems() }
func (cp *cachePrefetch) clearItemsWithSubnet() { cp.cache.clearItemsWithSubnet() }
func (cp *cachePrefetch) isOptimistic() bool    { return cp.cache.isOptimistic() }

// ────────────────────────────────────────────────────────────────────────────
// ── heat tracking ───────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

// recordAccess records an access to a domain+qtype and updates heat.
// shard.mu and heatTracker.mu are never held simultaneously.
func (cp *cachePrefetch) recordAccess(domain string, qtype uint16) {
	key := makeKey(domain, qtype)
	shard := cp.getShard(key)

	// Fast path: check if entry exists.
	shard.mu.RLock()
	_, exists := shard.entries[key]
	shard.mu.RUnlock()

	if !exists {
		// Slow path: create entry if needed.
		item := cp.cache.respToItem(&dns.Msg{}, nil, cp.logger)
		if item == nil {
			return
		}
		shard.mu.Lock()
		// Double-check after acquiring write lock.
		if _, exists := shard.entries[key]; !exists {
			shard.entries[key] = newCacheEntryExt(domain, qtype, 0, time.Now())
		}
		shard.mu.Unlock()
	}

	// Update heat — heatTracker has its own lock; no shard lock held here.
	joined := cp.heatTracker.onAccess(domain, qtype, time.Now())
	if joined {
		cp.logger.Debug("domain joined prefetch queue", "domain", domain, "qtype", qtype)
	}
}

// onDomainEvicted is called by the scheduler when a domain+qtype is evicted
// from the prefetch queue due to inactivity. This is registered as the
// onEvict hook so extEntries stays in sync with heatTracker.
func (cp *cachePrefetch) onDomainEvicted(domain string, qtype uint16) {
	key := makeKey(domain, qtype)
	shard := cp.getShard(key)

	shard.mu.Lock()
	delete(shard.entries, key)
	shard.mu.Unlock()

	cp.logger.Debug("domain evicted from prefetch",
		"domain", domain,
		"qtype", qtype)
}

// ────────────────────────────────────────────────────────────────────────────
// ── TTL helpers ─────────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

// cacheTTLFromMsg extracts the minimum TTL from DNS response records.
// cacheItem.ttl is 0 when returned from unpackItem(expired=true),
// so we recompute from Answer/Ns records.
func cacheTTLFromMsg(m *dns.Msg) uint32 {
	if m == nil {
		return 0
	}
	min := uint32(0)
	for _, rr := range m.Answer {
		if h := rr.Header(); h != nil && (min == 0 || h.Ttl < min) {
			min = h.Ttl
		}
	}
	if min == 0 {
		// Fall back to SOA for negative responses.
		for _, rr := range m.Ns {
			if soa, ok := rr.(*dns.SOA); ok && (min == 0 || soa.Hdr.Ttl < min) {
				min = soa.Hdr.Ttl
			}
		}
	}
	return min
}

// ────────────────────────────────────────────────────────────────────────────
// ── prefetch logic ──────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

// shouldPrefetchDomain checks if a domain+qtype needs prefetching now.
// Read-only operation, uses shard RLock.
func (cp *cachePrefetch) shouldPrefetchDomain(domain string, qtype uint16) bool {
	key := makeKey(domain, qtype)
	shard := cp.getShard(key)

	shard.mu.RLock()
	entry, ok := shard.entries[key]
	shard.mu.RUnlock()

	if !ok {
		return false
	}

	nowUnix := time.Now().Unix()
	remaining := entry.remainingTTL(nowUnix)
	if remaining == 0 {
		return false
	}

	if cp.config.ThresholdSeconds > 0 {
		if remaining < cp.config.ThresholdSeconds {
			return true
		}
	}

	if cp.config.ThresholdPercent > 0 {
		pct := entry.originalTTL * cp.config.ThresholdPercent / 100
		if remaining < pct {
			return true
		}
	}
	return false
}

// executePrefetchQuery performs the actual DNS query for prefetch with retry logic.
// The second shouldPrefetchDomain check prevents race conditions between
// the scheduler's check and the actual execution (TOCTOU).
func (cp *cachePrefetch) executePrefetchQuery(domain string, qtype uint16) {
	if !cp.shouldPrefetchDomain(domain, qtype) {
		return
	}

	maxRetries := cp.config.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Apply exponential backoff delay for retries
		if attempt > 0 {
			delay := cp.config.RetryDelay * time.Duration(attempt)
			cp.logger.Debug("retrying prefetch query",
				"domain", domain,
				"qtype", qtype,
				"attempt", attempt,
				"delay", delay)
			
			select {
			case <-time.After(delay):
				// Continue with retry
			case <-cp.ctx.Done():
				// Context cancelled, abort retry
				return
			}
		}

		// Attempt the prefetch query
		success, err := cp.tryPrefetchQuery(domain, qtype)
		if success {
			if attempt > 0 {
				cp.logger.Info("prefetch query succeeded after retry",
					"domain", domain,
					"qtype", qtype,
					"attempts", attempt+1)
			}
			return
		}

		lastErr = err
		
		// Don't retry if context is cancelled
		if cp.ctx.Err() != nil {
			return
		}
	}

	// All attempts failed
	if maxRetries > 0 {
		cp.logger.Debug("prefetch query failed after all retries",
			"domain", domain,
			"qtype", qtype,
			"attempts", maxRetries+1,
			"error", lastErr)
	}
}

// tryPrefetchQuery attempts a single prefetch query.
// Returns (success, error) where success indicates if the query completed successfully.
func (cp *cachePrefetch) tryPrefetchQuery(domain string, qtype uint16) (bool, error) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), qtype)
	req.RecursionDesired = true

	ctx, cancel := context.WithTimeout(cp.ctx, 5*time.Second)
	defer cancel()

	dctx := &DNSContext{Req: req}
	err := cp.proxy.Resolve(ctx, dctx)
	if err != nil {
		return false, err
	}
	if dctx.Res == nil {
		return false, fmt.Errorf("no response received")
	}

	newTTL := cacheTTLFromMsg(dctx.Res)
	if newTTL == 0 {
		return false, fmt.Errorf("zero TTL in response")
	}

	cp.onPrefetchSuccess(domain, qtype, dctx.Res, dctx.Upstream, newTTL)
	return true, nil
}

// onPrefetchSuccess updates the cache and extEntry after a successful prefetch.
func (cp *cachePrefetch) onPrefetchSuccess(
	domain string,
	qtype uint16,
	m *dns.Msg,
	u upstream.Upstream,
	newTTL uint32,
) {
	// Update base cache
	cp.cache.set(m, u, cp.logger)

	// Update extEntry TTL
	key := makeKey(domain, qtype)
	shard := cp.getShard(key)
	now := time.Now()

	shard.mu.Lock()
	if entry, ok := shard.entries[key]; ok {
		entry.refreshTTL(newTTL, now)
	}
	shard.mu.Unlock()

	cp.logger.Info("prefetch completed",
		"domain", domain,
		"qtype", qtype,
		"new_ttl", newTTL)
}

// ────────────────────────────────────────────────────────────────────────────
// ── stats ───────────────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) getStats() map[string]interface{} {
	candidates := cp.heatTracker.getPrefetchCandidates()
	total := 0
	for _, shard := range cp.extEntries {
		shard.mu.RLock()
		total += len(shard.entries)
		shard.mu.RUnlock()
	}
	return map[string]interface{}{
		"shard_count":         cp.shardCount,
		"active_tasks":        cp.scheduler.ActiveTasks(),
		"prefetch_queue_size": len(candidates),
		"total_entries":       total,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// ── lifecycle ───────────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) stop() {
	if cp.cancel != nil {
		cp.cancel()
	}
	if cp.scheduler != nil {
		cp.scheduler.stop()
	}
	cp.logger.Info("cache prefetch stopped")
}
