package proxy

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestPrefetchScheduler_ShouldPrefetch(t *testing.T) {
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Suppress logs during tests
	}))
	
	scheduler := &prefetchScheduler{
		config: config,
		logger: logger,
	}
	
	tests := []struct {
		name        string
		entry       *cacheEntryExt
		currentT    uint32
		wantPrefetch bool
	}{
		{
			name: "not in queue",
			entry: &cacheEntryExt{
				inPrefetchQueue: false,
				expiresAt:       300,
				originalTTL:     300,
			},
			currentT:    295,
			wantPrefetch: false,
		},
		{
			name: "already expired",
			entry: &cacheEntryExt{
				inPrefetchQueue: true,
				expiresAt:       300,
				originalTTL:     300,
			},
			currentT:    301,
			wantPrefetch: false,
		},
		{
			name: "fixed threshold triggered",
			entry: &cacheEntryExt{
				inPrefetchQueue: true,
				expiresAt:       300,
				originalTTL:     300,
			},
			currentT:    296, // remaining = 4s, less than 5s threshold
			wantPrefetch: true,
		},
		{
			name: "percentage threshold triggered (short TTL)",
			entry: &cacheEntryExt{
				inPrefetchQueue: true,
				expiresAt:       10,
				originalTTL:     10,
			},
			currentT:    8, // remaining = 2s, 10*80%=8, 2<8
			wantPrefetch: true,
		},
		{
			name: "not yet time to prefetch",
			entry: &cacheEntryExt{
				inPrefetchQueue: true,
				expiresAt:       300,
				originalTTL:     300,
			},
			currentT:    50, // remaining = 250s, above both thresholds (5s and 240s)
			wantPrefetch: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scheduler.shouldPrefetch(tt.entry, tt.currentT)
			if got != tt.wantPrefetch {
				t.Errorf("shouldPrefetch() = %v, want %v", got, tt.wantPrefetch)
			}
		})
	}
}

func TestPrefetchScheduler_ShortTTL(t *testing.T) {
	// Test case for very short TTL (1 second)
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	scheduler := &prefetchScheduler{
		config: config,
		logger: logger,
	}
	
	entry := &cacheEntryExt{
		inPrefetchQueue: true,
		expiresAt:       1,
		originalTTL:     1,
	}
	
	// At T=0, remaining=1s
	// Fixed threshold: 1 < 5 ✓
	// Percentage threshold: 1 < 1*80%=0.8 ✗
	// Should use percentage threshold (smaller value)
	if !scheduler.shouldPrefetch(entry, 0) {
		t.Error("should prefetch for short TTL at T=0")
	}
}

func TestPrefetchScheduler_LongTTL(t *testing.T) {
	// Test case for very long TTL (86400 seconds = 1 day)
	config := DefaultPrefetchConfig()
	config.ThresholdSeconds = 5
	config.ThresholdPercent = 80
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	scheduler := &prefetchScheduler{
		config: config,
		logger: logger,
	}
	
	entry := &cacheEntryExt{
		inPrefetchQueue: true,
		expiresAt:       86400,
		originalTTL:     86400,
	}
	
	// At T=17280, remaining=69120s (exactly 80% of 86400)
	// Fixed threshold: 69120 < 5 ✗
	// Percentage threshold: 69120 < 69120 ✗ (not less than)
	// Should not prefetch at exactly percentage threshold
	if scheduler.shouldPrefetch(entry, 17280) {
		t.Error("should not prefetch at exactly percentage threshold (69120s remaining)")
	}
	
	// At T=86396, remaining=4s
	// Fixed threshold: 4 < 5 ✓
	// Percentage threshold: 4 < 69120 ✓
	// Should prefetch (both thresholds triggered)
	if !scheduler.shouldPrefetch(entry, 86396) {
		t.Error("should prefetch when below fixed threshold (4s remaining)")
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
