package proxy

import (
"context"
"log/slog"
"net"
"strings"
"sync"
"time"

"github.com/AdguardTeam/dnsproxy/upstream"
"github.com/miekg/dns"
)

// cachePrefetch wraps the base cache with smart prefetch capabilities.
//
// Design notes:
//   - extEntries is sharded (16 shards) to reduce lock contention.
//   - heatTracker owns its own data; cachePrefetch never holds both
//     shard.mu and heatTracker.mu simultaneously.
//   - TTL tracking uses real Unix timestamps per entry (no global clock).
type cachePrefetch struct {
*cache

config      *PrefetchConfig
heatTracker *heatTracker
scheduler   *prefetchScheduler
proxy       *Proxy
logger      *slog.Logger

extEntries []*extEntryShard
shardCount int

ctx    context.Context
cancel context.CancelFunc
}

type extEntryShard struct {
entries map[string]*cacheEntryExt
mu      sync.RWMutex
}

func newExtEntryShard() *extEntryShard {
return &extEntryShard{entries: make(map[string]*cacheEntryExt)}
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

ctx, cancel := context.WithCancel(context.Background())

const shardCount = 16
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
"shards", shardCount)

return cp
}

// ── shard helpers ─────────────────────────────────────────────────────────────

func (cp *cachePrefetch) getShard(key string) *extEntryShard {
h := uint32(0)
for i := 0; i < len(key); i++ {
h = h*31 + uint32(key[i])
}
return cp.extEntries[h%uint32(cp.shardCount)]
}

// ── cacheInterface implementation ─────────────────────────────────────────────

func (cp *cachePrefetch) get(req *dns.Msg) (ci *cacheItem, expired bool, key []byte) {
ci, expired, key = cp.cache.get(req)
if ci != nil && len(req.Question) > 0 {
q := req.Question[0]
cp.recordAccess(q.Name, q.Qtype, ci, time.Now())
}
return ci, expired, key
}

func (cp *cachePrefetch) getWithSubnet(req *dns.Msg, clientIP *net.IPNet) (ci *cacheItem, expired bool, key []byte) {
ci, expired, key = cp.cache.getWithSubnet(req, clientIP)
if ci != nil && len(req.Question) > 0 {
q := req.Question[0]
cp.recordAccess(q.Name, q.Qtype, ci, time.Now())
}
return ci, expired, key
}

func (cp *cachePrefetch) set(m *dns.Msg, u upstream.Upstream, l *slog.Logger) {
cp.cache.set(m, u, l)
}

func (cp *cachePrefetch) setWithSubnet(m *dns.Msg, u upstream.Upstream, clientIP *net.IPNet, l *slog.Logger) {
cp.cache.setWithSubnet(m, u, clientIP, l)
}

func (cp *cachePrefetch) clearItems()           { cp.cache.clearItems() }
func (cp *cachePrefetch) clearItemsWithSubnet() { cp.cache.clearItemsWithSubnet() }
func (cp *cachePrefetch) isOptimistic() bool    { return cp.cache.isOptimistic() }

// ── heat tracking ─────────────────────────────────────────────────────────────

// recordAccess updates heat tracking for a cache hit.
// shard.mu and heatTracker.mu are never held simultaneously.
func (cp *cachePrefetch) recordAccess(domain string, qtype uint16, item *cacheItem, now time.Time) {
key := makeKey(domain, qtype)
shard := cp.getShard(key)

// Fast path: check existence with RLock.
shard.mu.RLock()
_, exists := shard.entries[key]
shard.mu.RUnlock()

if !exists {
ttl := extractTTLFromMsg(item.m)
if ttl == 0 {
return
}
shard.mu.Lock()
// Double-check after acquiring write lock.
if _, exists = shard.entries[key]; !exists {
shard.entries[key] = newCacheEntryExt(domain, qtype, ttl, now)
}
shard.mu.Unlock()
}

// Update heat — heatTracker has its own lock; no shard lock held here.
joined := cp.heatTracker.onAccess(domain, qtype, now)
if joined {
cp.logger.Debug("domain joined prefetch queue",
"domain", domain, "qtype", qtype)
}
}

// onDomainEvicted is called when a domain is evicted from the prefetch queue.
// It cleans up the corresponding extEntry to prevent memory leaks.
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

// ── TTL helpers ───────────────────────────────────────────────────────────────

// extractTTLFromMsg returns the minimum TTL from DNS answer records.
func extractTTLFromMsg(m *dns.Msg) uint32 {
if m == nil {
return 0
}
var min uint32
for _, rr := range m.Answer {
if h := rr.Header(); h != nil && (min == 0 || h.Ttl < min) {
min = h.Ttl
}
}
if min == 0 {
for _, rr := range m.Ns {
if soa, ok := rr.(*dns.SOA); ok && (min == 0 || soa.Minttl < min) {
min = soa.Minttl
}
}
}
return min
}

// ── prefetch execution ────────────────────────────────────────────────────────

// shouldPrefetchDomain checks whether domain+qtype needs prefetching now.
// Reads extEntry with RLock only — no write lock, no heatTracker lock.
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
if remaining < uint32(cp.config.ThresholdSeconds) {
return true
}
pct := entry.originalTTL * cp.config.ThresholdPercent / 100
return remaining < pct
}

// executePrefetchQuery is the executor injected into the scheduler.
func (cp *cachePrefetch) executePrefetchQuery(domain string, qtype uint16) {
if !cp.shouldPrefetchDomain(domain, qtype) {
return
}

req := &dns.Msg{}
req.SetQuestion(dns.Fqdn(domain), qtype)
req.RecursionDesired = true

ctx, cancel := context.WithTimeout(cp.ctx, cp.config.ScanInterval*5)
defer cancel()

dctx := &DNSContext{Req: req}
if err := cp.proxy.Resolve(ctx, dctx); err != nil {
cp.logger.Debug("prefetch query failed", "domain", domain, "error", err)
return
}
if dctx.Res == nil {
return
}

newTTL := cacheTTL(dctx.Res, cp.logger)
if newTTL == 0 {
return
}

cp.onPrefetchSuccess(domain, qtype, dctx.Res, dctx.Upstream, newTTL)
}

// onPrefetchSuccess stores the refreshed response and updates the extEntry.
// No global clock reset — each entry tracks its own cachedAt timestamp.
func (cp *cachePrefetch) onPrefetchSuccess(
domain string,
qtype uint16,
m *dns.Msg,
u upstream.Upstream,
newTTL uint32,
) {
cp.cache.set(m, u, cp.logger)

key := makeKey(domain, qtype)
shard := cp.getShard(key)
now := time.Now()

shard.mu.Lock()
if entry, ok := shard.entries[key]; ok {
entry.refreshTTL(newTTL, now)
}
shard.mu.Unlock()

cp.logger.Info("prefetch completed",
"domain", domain, "qtype", qtype, "new_ttl", newTTL)
}

// ── lifecycle ─────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) stop() {
if cp.cancel != nil {
cp.cancel()
}
if cp.scheduler != nil {
cp.scheduler.stop()
}
cp.logger.Info("cache prefetch stopped")
}

// ── stats ─────────────────────────────────────────────────────────────────────

func (cp *cachePrefetch) getPrefetchStats() map[string]interface{} {
total := 0
for _, shard := range cp.extEntries {
shard.mu.RLock()
total += len(shard.entries)
shard.mu.RUnlock()
}
candidates := cp.heatTracker.getPrefetchCandidates()
return map[string]interface{}{
"total_entries":       total,
"prefetch_queue_size": len(candidates),
"active_tasks":        cp.scheduler.ActiveTasks(),
"shard_count":         cp.shardCount,
}
}

func normalizeDomain(domain string) string {
return strings.ToLower(dns.Fqdn(domain))
}
