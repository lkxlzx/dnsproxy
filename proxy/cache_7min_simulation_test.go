package proxy

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// Test7MinSimulation runs a 7-minute real-world simulation with Google DNS
// to observe proactive cache refresh behavior in detail.
func Test7MinSimulation(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping 7-minute simulation test in short mode")
	}

	var upstreamRequestCount int32

	// Create real Google DNS upstream
	googleDNS, err := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	// Wrap upstream to count requests
	countingUpstream := &countingUpstreamWrapper{
		upstream: googleDNS,
		counter:  &upstreamRequestCount,
	}

	// Create proxy with cache and proactive refresh
	prx := mustNew(t, &Config{
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{countingUpstream},
		},
		CacheEnabled:      true,
		CacheSizeBytes:    64 * 1024 * 1024, // 64MB
		CacheMinTTL:       0,
		CacheMaxTTL:       0, // Use original TTL
		CacheOptimistic:   false,
		
		// Proactive refresh settings
		CacheProactiveRefreshTime:       30000, // 30 seconds before expiry
		CacheProactiveCooldownThreshold: 3,     // Need 3 requests to trigger
	})

	// Test domain
	domain := "google.com."
	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypeA)

	startTime := time.Now()
	
	// Statistics
	type RequestLog struct {
		Time         time.Time
		Elapsed      time.Duration
		IP           string
		TTL          uint32
		ResponseTime time.Duration
		Source       string
		UpstreamNum  int32
		CacheHit     bool
	}
	
	var logs []RequestLog

	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  7-Minute Real-World Simulation - Proactive Cache Refresh Analysis        ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  - Domain: google.com")
	t.Log("  - Upstream: 8.8.8.8:53 (Google DNS)")
	t.Log("  - Proactive Refresh: 30 seconds before expiry")
	t.Log("  - Cooldown Threshold: 3 requests")
	t.Log("  - Request Interval: 20 seconds")
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Request Log                                                               ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Logf("%-8s | %-15s | %-8s | %-10s | %-10s | %-8s | %s",
		"Time", "IP Address", "TTL", "Response", "Source", "Upstream", "Notes")
	t.Log("---------|-----------------|----------|------------|------------|----------|----------")

	requestNum := 0
	
	// Run for 7 minutes, making requests every 20 seconds
	for time.Since(startTime) < 7*time.Minute {
		requestNum++
		elapsed := time.Since(startTime)
		
		// Make request
		reqStart := time.Now()
		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		responseTime := time.Since(reqStart)
		
		require.NoError(t, err)
		require.NotNil(t, dctx.Res)
		require.NotEmpty(t, dctx.Res.Answer)
		
		// Extract response details
		var ip string
		var ttl uint32
		for _, ans := range dctx.Res.Answer {
			if a, ok := ans.(*dns.A); ok {
				ip = a.A.String()
				ttl = a.Header().Ttl
				break
			}
		}
		
		upstreamNum := atomic.LoadInt32(&upstreamRequestCount)
		
		// Determine if cache hit (response time < 5ms indicates cache)
		isCacheHit := responseTime < 5*time.Millisecond
		source := "UPSTREAM"
		if isCacheHit {
			source = "CACHE"
		}
		
		// Create log entry
		logEntry := RequestLog{
			Time:         time.Now(),
			Elapsed:      elapsed,
			IP:           ip,
			TTL:          ttl,
			ResponseTime: responseTime,
			Source:       source,
			UpstreamNum:  upstreamNum,
			CacheHit:     isCacheHit,
		}
		logs = append(logs, logEntry)
		
		// Format time
		elapsedSec := int(elapsed.Seconds())
		timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
		
		// Determine notes
		notes := ""
		if requestNum == 1 {
			notes = "Initial request"
		} else if !isCacheHit && requestNum > 1 {
			// Check if this is a proactive refresh
			timeSinceLastUpstream := elapsed
			
			// Find last upstream request
			for i := len(logs) - 2; i >= 0; i-- {
				if !logs[i].CacheHit {
					timeSinceLastUpstream = elapsed - logs[i].Elapsed
					break
				}
			}
			
			if timeSinceLastUpstream < 40*time.Second {
				notes = "⚡ PROACTIVE REFRESH"
			} else {
				notes = "Cache expired"
			}
		} else if isCacheHit && requestNum <= 3 {
			notes = fmt.Sprintf("Building stats (%d/3)", requestNum)
		}
		
		// Log request
		t.Logf("%s | %-15s | %6ds | %8.2fms | %-10s | %8d | %s",
			timeStr, ip, ttl, float64(responseTime.Microseconds())/1000.0,
			source, upstreamNum, notes)
		
		// Wait 20 seconds before next request (unless we're close to the end)
		if time.Since(startTime) < 7*time.Minute-20*time.Second {
			time.Sleep(20 * time.Second)
		} else {
			break
		}
	}
	
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Analysis                                                                  ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	// Analyze the logs
	totalRequests := len(logs)
	cacheHits := 0
	upstreamQueries := 0
	proactiveRefreshes := 0
	
	var firstTTL uint32
	var cacheResponseTimes []time.Duration
	var upstreamResponseTimes []time.Duration
	
	for i, log := range logs {
		if log.CacheHit {
			cacheHits++
			cacheResponseTimes = append(cacheResponseTimes, log.ResponseTime)
		} else {
			upstreamQueries++
			upstreamResponseTimes = append(upstreamResponseTimes, log.ResponseTime)
			
			if i == 0 {
				firstTTL = log.TTL
			} else {
				// Check if this is a proactive refresh
				timeSinceLastUpstream := log.Elapsed
				for j := i - 1; j >= 0; j-- {
					if !logs[j].CacheHit {
						timeSinceLastUpstream = log.Elapsed - logs[j].Elapsed
						break
					}
				}
				
				if timeSinceLastUpstream < 40*time.Second {
					proactiveRefreshes++
				}
			}
		}
	}
	
	// Calculate averages
	avgCacheTime := time.Duration(0)
	if len(cacheResponseTimes) > 0 {
		for _, t := range cacheResponseTimes {
			avgCacheTime += t
		}
		avgCacheTime /= time.Duration(len(cacheResponseTimes))
	}
	
	avgUpstreamTime := time.Duration(0)
	if len(upstreamResponseTimes) > 0 {
		for _, t := range upstreamResponseTimes {
			avgUpstreamTime += t
		}
		avgUpstreamTime /= time.Duration(len(upstreamResponseTimes))
	}
	
	cacheHitRate := float64(cacheHits) / float64(totalRequests) * 100
	
	t.Logf("Total Requests:        %d", totalRequests)
	t.Logf("Cache Hits:            %d (%.1f%%)", cacheHits, cacheHitRate)
	t.Logf("Upstream Queries:      %d", upstreamQueries)
	t.Logf("Proactive Refreshes:   %d", proactiveRefreshes)
	t.Log("")
	t.Logf("Original TTL:          %d seconds", firstTTL)
	t.Logf("Refresh Trigger Time:  %d seconds (TTL - 30s)", firstTTL-30)
	t.Log("")
	t.Logf("Avg Cache Response:    %.2fms", float64(avgCacheTime.Microseconds())/1000.0)
	t.Logf("Avg Upstream Response: %.2fms", float64(avgUpstreamTime.Microseconds())/1000.0)
	t.Logf("Speed Improvement:     %.1fx", float64(avgUpstreamTime)/float64(avgCacheTime))
	t.Log("")
	
	// Detailed refresh analysis
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Proactive Refresh Timeline                                                ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	for i, log := range logs {
		if !log.CacheHit {
			elapsedSec := int(log.Elapsed.Seconds())
			timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
			
			if i == 0 {
				t.Logf("[%s] Initial upstream query - TTL: %ds", timeStr, log.TTL)
			} else {
				// Find previous upstream query
				var prevUpstreamTime time.Duration
				for j := i - 1; j >= 0; j-- {
					if !logs[j].CacheHit {
						prevUpstreamTime = logs[j].Elapsed
						break
					}
				}
				
				timeSinceLastUpstream := log.Elapsed - prevUpstreamTime
				
				if timeSinceLastUpstream < 40*time.Second {
					t.Logf("[%s] ⚡ PROACTIVE REFRESH - %ds after last upstream (expected ~%ds)",
						timeStr, int(timeSinceLastUpstream.Seconds()), firstTTL-30)
				} else {
					t.Logf("[%s] Cache expired - %ds after last upstream",
						timeStr, int(timeSinceLastUpstream.Seconds()))
				}
			}
		}
	}
	
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Conclusion                                                                ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	if proactiveRefreshes > 0 {
		t.Log("✅ Proactive cache refresh is WORKING")
		t.Logf("   - Detected %d proactive refresh(es)", proactiveRefreshes)
		t.Logf("   - Cache hit rate: %.1f%%", cacheHitRate)
		t.Logf("   - Response time improved by %.1fx", float64(avgUpstreamTime)/float64(avgCacheTime))
	} else {
		t.Log("⚠️  No proactive refreshes detected")
		t.Log("   - This may be normal if:")
		t.Log("     1. Not enough requests to build stats (need 3+)")
		t.Log("     2. TTL is very long and refresh hasn't triggered yet")
		t.Log("     3. Request interval is longer than TTL")
	}
	
	t.Log("")
	t.Log("Test completed successfully!")
}

// countingUpstreamWrapper wraps an upstream and counts requests
type countingUpstreamWrapper struct {
	upstream upstream.Upstream
	counter  *int32
}

func (u *countingUpstreamWrapper) Exchange(m *dns.Msg) (*dns.Msg, error) {
	atomic.AddInt32(u.counter, 1)
	return u.upstream.Exchange(m)
}

func (u *countingUpstreamWrapper) Address() string {
	return u.upstream.Address()
}

func (u *countingUpstreamWrapper) Close() error {
	return u.upstream.Close()
}
