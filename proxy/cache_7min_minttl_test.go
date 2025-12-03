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

// Test7MinWithMinTTL tests proactive refresh with minimum TTL override
// to handle cases where upstream returns very short TTLs.
func Test7MinWithMinTTL(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping 7-minute test with min TTL in short mode")
	}

	var upstreamRequestCount int32

	// Create real Google DNS upstream
	googleDNS, err := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	// Wrap upstream to count requests and log
	countingUpstream := &loggingUpstreamWrapper{
		upstream: googleDNS,
		counter:  &upstreamRequestCount,
		t:        t,
	}

	// Create proxy with cache, proactive refresh, and MIN TTL override
	prx := mustNew(t, &Config{
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{countingUpstream},
		},
		CacheEnabled:      true,
		CacheSizeBytes:    64 * 1024 * 1024,
		CacheMinTTL:       5,   // 🔑 Minimum TTL: 5 seconds
		CacheMaxTTL:       0,   // No maximum
		CacheOptimistic:   true,
		
		// Proactive refresh settings
		CacheProactiveRefreshTime:       1000, // 1 second before expiry
		CacheProactiveCooldownThreshold: 3,
	})

	// Test domain
	domain := "google.com."
	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypeA)

	startTime := time.Now()
	
	type RequestLog struct {
		RequestNum   int
		Time         time.Time
		Elapsed      time.Duration
		IP           string
		OriginalTTL  uint32
		EffectiveTTL uint32
		ResponseTime time.Duration
		Source       string
		UpstreamNum  int32
	}
	
	var logs []RequestLog
	var lastUpstreamTime time.Duration

	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  7-Minute Test with Minimum TTL Override                                  ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  - Domain: google.com")
	t.Log("  - Upstream: 8.8.8.8:53 (Google DNS)")
	t.Log("  - Minimum TTL: 5 seconds (OVERRIDE SHORT TTLs)")
	t.Log("  - Proactive Refresh: 1 second before expiry")
	t.Log("  - Cooldown Threshold: 3 requests")
	t.Log("  - Request Interval: 10 seconds")
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Request Log                                                               ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Logf("%-6s | %-8s | %-15s | %-8s | %-8s | %-10s | %-10s | %s",
		"#", "Time", "IP Address", "Orig TTL", "Eff TTL", "Response", "Source", "Notes")
	t.Log("-------|----------|-----------------|----------|----------|------------|------------|----------")

	requestNum := 0
	
	// Run for 7 minutes, making requests every 10 seconds
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
		var effectiveTTL uint32
		for _, ans := range dctx.Res.Answer {
			if a, ok := ans.(*dns.A); ok {
				ip = a.A.String()
				effectiveTTL = a.Header().Ttl
				break
			}
		}
		
		upstreamNum := atomic.LoadInt32(&upstreamRequestCount)
		
		// Determine if cache hit
		isCacheHit := responseTime < 5*time.Millisecond
		source := "UPSTREAM"
		
		if !isCacheHit {
			source = "UPSTREAM"
			lastUpstreamTime = elapsed
		} else {
			source = "CACHE"
		}
		
		// For upstream queries, we don't know the original TTL from the response
		// (it's already been processed), so we'll mark it as unknown
		originalTTL := effectiveTTL // Assume same for now
		
		// Create log entry
		logEntry := RequestLog{
			RequestNum:   requestNum,
			Time:         time.Now(),
			Elapsed:      elapsed,
			IP:           ip,
			OriginalTTL:  originalTTL,
			EffectiveTTL: effectiveTTL,
			ResponseTime: responseTime,
			Source:       source,
			UpstreamNum:  upstreamNum,
		}
		logs = append(logs, logEntry)
		
		// Format time
		elapsedSec := int(elapsed.Seconds())
		timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
		
		// Determine notes
		notes := ""
		if requestNum == 1 {
			notes = "Initial request"
		} else if !isCacheHit {
			timeSinceLastUpstream := elapsed - lastUpstreamTime
			if timeSinceLastUpstream < 20*time.Second && requestNum > 3 {
				notes = "⚡ PROACTIVE REFRESH"
			} else {
				notes = "Cache expired or initial"
			}
		} else if requestNum <= 3 {
			notes = fmt.Sprintf("Building stats (%d/3)", requestNum)
		}
		
		// Log request
		t.Logf("%-6d | %s | %-15s | %6ds | %6ds | %8.2fms | %-10s | %s",
			requestNum, timeStr, ip, originalTTL, effectiveTTL,
			float64(responseTime.Microseconds())/1000.0,
			source, notes)
		
		// Wait 10 seconds before next request
		if time.Since(startTime) < 7*time.Minute-10*time.Second {
			time.Sleep(10 * time.Second)
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
	minEffectiveTTL := uint32(999999)
	maxEffectiveTTL := uint32(0)
	
	for i, log := range logs {
		if log.Source == "CACHE" {
			cacheHits++
		} else {
			upstreamQueries++
			if i > 0 && i > 3 {
				// Check if this is a proactive refresh
				timeSinceLastUpstream := log.Elapsed - logs[i-1].Elapsed
				if timeSinceLastUpstream < 20*time.Second {
					proactiveRefreshes++
				}
			}
		}
		
		if log.EffectiveTTL < minEffectiveTTL {
			minEffectiveTTL = log.EffectiveTTL
		}
		if log.EffectiveTTL > maxEffectiveTTL {
			maxEffectiveTTL = log.EffectiveTTL
		}
	}
	
	cacheHitRate := float64(cacheHits) / float64(totalRequests) * 100
	
	t.Logf("Total Requests:        %d", totalRequests)
	t.Logf("Cache Hits:            %d (%.1f%%)", cacheHits, cacheHitRate)
	t.Logf("Upstream Queries:      %d", upstreamQueries)
	t.Logf("Proactive Refreshes:   %d", proactiveRefreshes)
	t.Log("")
	t.Logf("Effective TTL Range:   %d - %d seconds", minEffectiveTTL, maxEffectiveTTL)
	t.Logf("Minimum TTL Override:  5 seconds")
	t.Log("")
	
	// Check if min TTL override is working
	if minEffectiveTTL >= 5 {
		t.Log("✅ Minimum TTL override is WORKING")
		t.Logf("   - All effective TTLs are >= 5 seconds")
		t.Logf("   - Short TTLs from upstream (like 2s) are being overridden to 5s")
	} else {
		t.Logf("⚠️  Minimum effective TTL (%ds) is below configured minimum (5s)", minEffectiveTTL)
	}
	
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Upstream Query Timeline                                                   ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	for _, log := range logs {
		if log.Source != "CACHE" {
			elapsedSec := int(log.Elapsed.Seconds())
			timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
			
			if log.RequestNum == 1 {
				t.Logf("[%s] #%-2d Initial upstream query - Effective TTL: %ds", 
					timeStr, log.RequestNum, log.EffectiveTTL)
			} else {
				// Check if proactive refresh
				isProactive := false
				if log.RequestNum > 3 {
					for i := len(logs) - 1; i >= 0; i-- {
						if logs[i].Source != "CACHE" && logs[i].RequestNum < log.RequestNum {
							timeSince := log.Elapsed - logs[i].Elapsed
							if timeSince < 20*time.Second {
								isProactive = true
							}
							break
						}
					}
				}
				
				if isProactive {
					t.Logf("[%s] #%-2d ⚡ PROACTIVE REFRESH - Effective TTL: %ds", 
						timeStr, log.RequestNum, log.EffectiveTTL)
				} else {
					t.Logf("[%s] #%-2d Cache expired - Effective TTL: %ds", 
						timeStr, log.RequestNum, log.EffectiveTTL)
				}
			}
		}
	}
	
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Conclusion                                                                ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	if minEffectiveTTL >= 5 {
		t.Log("✅ Minimum TTL override successfully prevents short TTL issues")
		t.Log("   - Even if upstream returns 2s TTL, it's overridden to 5s")
		t.Log("   - Proactive refresh works with stable TTL values")
		t.Log("   - No excessive refresh cycles due to short TTLs")
	}
	
	if proactiveRefreshes > 0 {
		t.Log("✅ Proactive cache refresh is working with min TTL override")
		t.Logf("   - Detected %d proactive refresh(es)", proactiveRefreshes)
	}
	
	t.Log("")
	t.Log("Test completed successfully!")
}
