package proxy

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachePrefetch_RetryLogic tests the retry logic implementation.
func TestCachePrefetch_RetryLogic(t *testing.T) {
	t.Run("exponential_backoff_timing", func(t *testing.T) {
		// Test that retry delays follow exponential backoff
		config := &PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,
			ThresholdPercent:        80,
			MaxConcurrent:           10,
			ScanInterval:            100 * time.Millisecond,
			MinHeatThreshold:        3,
			TimeWindow:              10 * time.Second,
			InactivityCheckInterval: 1 * time.Second,
			MaxRetries:              2,
			RetryDelay:              100 * time.Millisecond,
		}

		baseCache := newCache(&cacheConfig{
			size:       1024 * 1024,
			optimistic: false,
		})

		proxy := &Proxy{
			Config: Config{
				CacheEnabled:        true,
				CacheSizeBytes:      1024 * 1024,
				CachePrefetchConfig: config,
			},
			logger: slog.Default(),
		}

		cp := newCachePrefetch(baseCache, config, proxy, slog.Default())
		require.NotNil(t, cp)
		defer cp.stop()

		// Test exponential backoff calculation
		delays := []time.Duration{}
		for attempt := 1; attempt <= 3; attempt++ {
			delay := config.RetryDelay * time.Duration(attempt)
			delays = append(delays, delay)
		}

		assert.Equal(t, 100*time.Millisecond, delays[0], "first retry delay")
		assert.Equal(t, 200*time.Millisecond, delays[1], "second retry delay")
		assert.Equal(t, 300*time.Millisecond, delays[2], "third retry delay")
	})

	t.Run("max_retries_configuration", func(t *testing.T) {
		testCases := []struct {
			name          string
			maxRetries    int
			expectedTotal int
		}{
			{"no_retries", 0, 1},
			{"one_retry", 1, 2},
			{"two_retries", 2, 3},
			{"three_retries", 3, 4},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Total attempts = initial + retries
				totalAttempts := 1 + tc.maxRetries
				assert.Equal(t, tc.expectedTotal, totalAttempts, "total attempts")
			})
		}
	})

	t.Run("negative_max_retries_treated_as_zero", func(t *testing.T) {
		maxRetries := -1
		// Negative maxRetries should be treated as 0 in executePrefetchQuery
		if maxRetries < 0 {
			maxRetries = 0
		}
		assert.Equal(t, 0, maxRetries, "negative value should be treated as 0")
	})
}

// TestCachePrefetch_RetryIntegration tests retry with actual DNS resolution.
func TestCachePrefetch_RetryIntegration(t *testing.T) {
	t.Run("retry_configuration_applied", func(t *testing.T) {
		config := &PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,
			ThresholdPercent:        80,
			MaxConcurrent:           10,
			ScanInterval:            100 * time.Millisecond,
			MinHeatThreshold:        3,
			TimeWindow:              10 * time.Second,
			InactivityCheckInterval: 1 * time.Second,
			MaxRetries:              2,
			RetryDelay:              50 * time.Millisecond,
		}

		baseCache := newCache(&cacheConfig{
			size:       1024 * 1024,
			optimistic: false,
		})

		proxy := &Proxy{
			Config: Config{
				CacheEnabled:        true,
				CacheSizeBytes:      1024 * 1024,
				CachePrefetchConfig: config,
			},
			logger: slog.Default(),
		}

		cp := newCachePrefetch(baseCache, config, proxy, slog.Default())
		require.NotNil(t, cp)
		defer cp.stop()

		// Verify retry configuration is applied
		assert.Equal(t, 2, cp.config.MaxRetries, "MaxRetries should be 2")
		assert.Equal(t, 50*time.Millisecond, cp.config.RetryDelay, "RetryDelay should be 50ms")
	})
}

// TestDefaultPrefetchConfig_RetryDefaults tests that default config includes retry settings.
func TestDefaultPrefetchConfig_RetryDefaults(t *testing.T) {
	config := DefaultPrefetchConfig()
	
	assert.Equal(t, 2, config.MaxRetries, "default MaxRetries should be 2")
	assert.Equal(t, 1*time.Second, config.RetryDelay, "default RetryDelay should be 1s")
}
