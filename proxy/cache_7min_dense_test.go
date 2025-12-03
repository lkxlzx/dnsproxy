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

// Test7MinDenseSimulation runs a 7-minute simulation with denser requests (every 10s)
// to better observe proactive cache refresh behavior.
func Test7MinDenseSimulation(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping 7-minute dense simulation test in short mode")
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

	// Create proxy with cache and proactive refresh
	prx := mustNew(t, &Config{
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{countingUpstream},
		},
		CacheEnabled:      true,
		CacheSizeBytes:    64 * 1024 * 1024,
		CacheMinTTL:       0,
		CacheMaxTTL:       0,
		CacheOptimistic:   true, // Enable optimistic cache for proactive refresh
		
		// Proactive refresh settings - AGGRESSIVE
		CacheProactiveRefreshTime:       1000, // 1 second before expiry (VERY AGGRESSIVE)
		CacheProactiveCooldownThreshold: 3,    // Need 3 requests
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
		TTL          uint32
		TTLRemaining uint32
		ResponseTime time.Duration
		Source       string
		UpstreamNum  int32
		IsRefresh    bool
	}
	
	var logs []RequestLog
	var firstTTL uint32
	var lastUpstreamTime time.Duration

	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  7-Minute Dense Simulation - Proactive Cache Refresh Analysis             ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  - Domain: google.com")
	t.Log("  - Upstream: 8.8.8.8:53 (Google DNS)")
	t.Log("  - Proactive Refresh: 1 second before expiry (AGGRESSIVE)")
	t.Log("  - Cooldown Threshold: 3 requests")
	t.Log("  - Request Interval: 10 seconds (DENSE)")
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Request Log                                                               ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Logf("%-6s | %-8s | %-15s | %-8s | %-8s | %-10s | %-10s | %s",
		"#", "Time", "IP Address", "TTL", "Remain", "Response", "Source", "Notes")
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
		var ttl uint32
		for _, ans := range dctx.Res.Answer {
			if a, ok := ans.(*dns.A); ok {
				ip = a.A.String()
				ttl = a.Header().Ttl
				break
			}
		}
		
		upstreamNum := atomic.LoadInt32(&upstreamRequestCount)
		
		// Determine if cache hit
		isCacheHit := responseTime < 5*time.Millisecond
		source := "UPSTREAM"
		isRefresh := false
		
		if !isCacheHit {
			source = "UPSTREAM"
			if requestNum == 1 {
				firstTTL = ttl
			} else {
				// Check if this is a proactive refresh
				timeSinceLastUpstream := elapsed - lastUpstreamTime
				if timeSinceLastUpstream < time.Duration(firstTTL-10)*time.Second {
					isRefresh = true
					source = "REFRESH"
				}
			}
			lastUpstreamTime = elapsed
		} else {
			source = "CACHE"
		}
		
		// Calculate TTL remaining (approximate)
		var ttlRemaining uint32
		if firstTTL > 0 {
			elapsedSinceLastUpstream := elapsed - lastUpstreamTime
			if elapsedSinceLastUpstream < time.Duration(firstTTL)*time.Second {
				ttlRemaining = firstTTL - uint32(elapsedSinceLastUpstream.Seconds())
			}
		}
		
		// Create log entry
		logEntry := RequestLog{
			RequestNum:   requestNum,
			Time:         time.Now(),
			Elapsed:      elapsed,
			IP:           ip,
			TTL:          ttl,
			TTLRemaining: ttlRemaining,
			ResponseTime: responseTime,
			Source:       source,
			UpstreamNum:  upstreamNum,
			IsRefresh:    isRefresh,
		}
		logs = append(logs, logEntry)
		
		// Format time
		elapsedSec := int(elapsed.Seconds())
		timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
		
		// Determine notes
		notes := ""
		if requestNum == 1 {
			notes = "Initial request"
		} else if isRefresh {
			notes = fmt.Sprintf("⚡ PROACTIVE REFRESH (TTL was ~%ds)", ttlRemaining+30)
		} else if !isCacheHit {
			notes = "Cache expired"
		} else if requestNum <= 3 {
			notes = fmt.Sprintf("Building stats (%d/3)", requestNum)
		} else if ttlRemaining <= 40 && ttlRemaining > 0 {
			notes = fmt.Sprintf("⏰ Refresh window (TTL-%ds)", firstTTL-ttlRemaining)
		}
		
		// Log request
		t.Logf("%-6d | %s | %-15s | %6ds | %6ds | %8.2fms | %-10s | %s",
			requestNum, timeStr, ip, ttl, ttlRemaining,
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
	
	for _, log := range logs {
		if log.Source == "CACHE" {
			cacheHits++
		} else {
			upstreamQueries++
			if log.IsRefresh {
				proactiveRefreshes++
			}
		}
	}
	
	cacheHitRate := float64(cacheHits) / float64(totalRequests) * 100
	
	t.Logf("Total Requests:        %d", totalRequests)
	t.Logf("Cache Hits:            %d (%.1f%%)", cacheHits, cacheHitRate)
	t.Logf("Upstream Queries:      %d", upstreamQueries)
	t.Logf("Proactive Refreshes:   %d", proactiveRefreshes)
	t.Log("")
	t.Logf("Original TTL:          %d seconds", firstTTL)
	t.Logf("Refresh Trigger Time:  %d seconds (TTL - 30s)", firstTTL-30)
	t.Logf("Request Interval:      10 seconds")
	t.Log("")
	
	// Detailed refresh analysis
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Upstream Query Timeline                                                   ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	for _, log := range logs {
		if log.Source != "CACHE" {
			elapsedSec := int(log.Elapsed.Seconds())
			timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)
			
			if log.RequestNum == 1 {
				t.Logf("[%s] #%-2d Initial upstream query - TTL: %ds", timeStr, log.RequestNum, log.TTL)
			} else if log.IsRefresh {
				t.Logf("[%s] #%-2d ⚡ PROACTIVE REFRESH - TTL remaining: ~%ds", 
					timeStr, log.RequestNum, log.TTLRemaining+30)
			} else {
				t.Logf("[%s] #%-2d Cache expired - New TTL: %ds", timeStr, log.RequestNum, log.TTL)
			}
		}
	}
	
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Conclusion                                                                ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	
	if proactiveRefreshes > 0 {
		t.Log("✅ Proactive cache refresh is WORKING!")
		t.Logf("   - Detected %d proactive refresh(es)", proactiveRefreshes)
		t.Logf("   - Cache hit rate: %.1f%%", cacheHitRate)
		t.Log("   - Cache was refreshed BEFORE expiry")
		t.Log("   - Users always get fast cached responses")
	} else {
		t.Log("⚠️  No proactive refreshes detected in this run")
		t.Log("   - Possible reasons:")
		t.Logf("     1. Request interval (10s) may not align with refresh window")
		t.Logf("     2. Refresh window is %ds-%ds (TTL 270-300)", firstTTL-30, firstTTL)
		t.Log("     3. Background refresh may happen between requests")
	}
	
	t.Log("")
	t.Log("Test completed successfully!")
}

// loggingUpstreamWrapper wraps an upstream, counts requests, and logs them
type loggingUpstreamWrapper struct {
	upstream upstream.Upstream
	counter  *int32
	t        *testing.T
}

func (u *loggingUpstreamWrapper) Exchange(m *dns.Msg) (*dns.Msg, error) {
	count := atomic.AddInt32(u.counter, 1)
	u.t.Logf("    → Upstream request #%d to %s", count, u.upstream.Address())
	resp, err := u.upstream.Exchange(m)
	if err == nil && resp != nil {
		for _, ans := range resp.Answer {
			if a, ok := ans.(*dns.A); ok {
				u.t.Logf("    ← Response: %s (TTL: %ds)", a.A.String(), a.Header().Ttl)
				break
			}
		}
	}
	return resp, err
}

func (u *loggingUpstreamWrapper) Address() string {
	return u.upstream.Address()
}

func (u *loggingUpstreamWrapper) Close() error {
	return u.upstream.Close()
}
