package proxy

import "time"

// PrefetchConfig contains configuration for smart cache prefetching mechanism.
type PrefetchConfig struct {
	// Enabled enables the smart prefetch mechanism.
	// When disabled, the cache falls back to the original passive expiration behavior.
	Enabled bool

	// ThresholdSeconds is the fixed threshold in seconds.
	// Prefetch is triggered when remaining TTL < ThresholdSeconds.
	// Default: 5 seconds
	ThresholdSeconds uint32

	// ThresholdPercent is the percentage threshold (0-100).
	// Prefetch is triggered when remaining TTL < OriginalTTL * ThresholdPercent / 100.
	// Default: 80 (means 80%)
	ThresholdPercent uint32

	// MaxConcurrent is the maximum number of concurrent background refresh tasks.
	// Default: 10
	MaxConcurrent int

	// MinHeatThreshold is the minimum number of accesses required during the time window
	// for a domain to join the prefetch queue (cold-start phase).
	// Default: 6
	MinHeatThreshold int

	// TimeWindow is the time window for cold-start judgment and inactivity detection.
	// Default: 180 seconds
	TimeWindow time.Duration

	// MaxRetries is the maximum number of retry attempts for failed prefetch queries.
	// 0 means no retries (fail immediately).
	// Default: 2
	MaxRetries int

	// RetryDelay is the base delay between retry attempts.
	// Actual delay uses exponential backoff: RetryDelay * (attempt + 1)
	// Default: 1 second
	RetryDelay time.Duration
}

// DefaultPrefetchConfig returns the default prefetch configuration.
func DefaultPrefetchConfig() *PrefetchConfig {
	return &PrefetchConfig{
		Enabled:          false, // Disabled by default for backward compatibility
		ThresholdSeconds: 5,
		ThresholdPercent: 80,
		MaxConcurrent:    10,
		MinHeatThreshold: 6,
		TimeWindow:       180 * time.Second,
		MaxRetries:       2,
		RetryDelay:       1 * time.Second,
	}
}
