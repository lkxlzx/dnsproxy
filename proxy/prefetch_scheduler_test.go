package proxy

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestPrefetchScheduler_ShouldPrefetchDomain(t *testing.T) {
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80

	ht := newHeatTracker(config.MinHeatThreshold, config.TimeWindow)
	ps := newPrefetchScheduler(config, ht, newTestLogger())

	// Build a minimal cachePrefetch to use shouldPrefetchDomain
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
	cp := &cachePrefetch{
		cache:       baseCache,
		config:      config,
		heatTracker: ht,
		scheduler:   ps,
		extEntries:  make([]*extEntryShard, 16),
		shardCount:  16,
	}
	for i := range cp.extEntries {
		cp.extEntries[i] = newExtEntryShard()
	}

	now := time.Now()

	// Entry with 300s TTL, cached now — remaining = 300s, no prefetch
	key := makeKey("google.com.", 1)
	shard := cp.getShard(key)
	shard.mu.Lock()
	shard.entries[key] = newCacheEntryExt("google.com.", 1, 300, now)
	shard.mu.Unlock()

	if cp.shouldPrefetchDomain("google.com.", 1) {
		t.Error("should not prefetch: 300s remaining")
	}

	// Simulate 295s elapsed — remaining = 5s, below fixed threshold
	shard.mu.Lock()
	shard.entries[key].cachedAt = now.Add(-295 * time.Second).Unix()
	shard.mu.Unlock()

	if !cp.shouldPrefetchDomain("google.com.", 1) {
		t.Error("should prefetch: 5s remaining < threshold 5s")
	}
}

func TestPrefetchScheduler_ShortTTL(t *testing.T) {
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80

	ht := newHeatTracker(config.MinHeatThreshold, config.TimeWindow)
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
	cp := &cachePrefetch{
		cache:       baseCache,
		config:      config,
		heatTracker: ht,
		extEntries:  make([]*extEntryShard, 16),
		shardCount:  16,
	}
	for i := range cp.extEntries {
		cp.extEntries[i] = newExtEntryShard()
	}

	now := time.Now()
	// TTL=6s, 5s elapsed → remaining=1s < threshold(5s)
	key := makeKey("short.com.", 1)
	shard := cp.getShard(key)
	shard.mu.Lock()
	shard.entries[key] = newCacheEntryExt("short.com.", 1, 6, now.Add(-5*time.Second))
	shard.mu.Unlock()

	if !cp.shouldPrefetchDomain("short.com.", 1) {
		t.Error("should prefetch short TTL domain")
	}
}

func TestPrefetchScheduler_LongTTL(t *testing.T) {
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80

	ht := newHeatTracker(config.MinHeatThreshold, config.TimeWindow)
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
	cp := &cachePrefetch{
		cache:       baseCache,
		config:      config,
		heatTracker: ht,
		extEntries:  make([]*extEntryShard, 16),
		shardCount:  16,
	}
	for i := range cp.extEntries {
		cp.extEntries[i] = newExtEntryShard()
	}

	now := time.Now()
	// TTL=86400s (1 day), just cached — remaining=86400s, no prefetch
	key := makeKey("long.com.", 1)
	shard := cp.getShard(key)
	shard.mu.Lock()
	shard.entries[key] = newCacheEntryExt("long.com.", 1, 86400, now)
	shard.mu.Unlock()

	if cp.shouldPrefetchDomain("long.com.", 1) {
		t.Error("should not prefetch: 86400s remaining")
	}

	// Simulate 86396s elapsed — remaining=4s < threshold(5s)
	shard.mu.Lock()
	shard.entries[key].cachedAt = now.Add(-86396 * time.Second).Unix()
	shard.mu.Unlock()

	if !cp.shouldPrefetchDomain("long.com.", 1) {
		t.Error("should prefetch: 4s remaining < threshold 5s")
	}
}

func TestPrefetchConfig_Defaults(t *testing.T) {
	config := DefaultPrefetchConfig()

	if config.Enabled {
		t.Error("Enabled should be false by default")
	}
	if config.ThresholdSeconds != 5 {
		t.Errorf("ThresholdSeconds = %d, want 5", config.ThresholdSeconds)
	}
	if config.ThresholdPercent != 80 {
		t.Errorf("ThresholdPercent = %d, want 80", config.ThresholdPercent)
	}
	if config.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want 10", config.MaxConcurrent)
	}
	if config.ScanInterval != 1*time.Second {
		t.Errorf("ScanInterval = %v, want 1s", config.ScanInterval)
	}
	if config.MinHeatThreshold != 6 {
		t.Errorf("MinHeatThreshold = %d, want 6", config.MinHeatThreshold)
	}
	if config.TimeWindow != 180*time.Second {
		t.Errorf("TimeWindow = %v, want 180s", config.TimeWindow)
	}
	if config.InactivityCheckInterval != 10*time.Second {
		t.Errorf("InactivityCheckInterval = %v, want 10s", config.InactivityCheckInterval)
	}
}
