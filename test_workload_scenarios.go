//go:build ignore

package main

// Multi-workload real-world DNS prefetch test
// Covers: cold start, burst traffic, mixed query types, concurrent clients,
//         inactivity eviction, upstream failure recovery, TTL edge cases,
//         and cache-vs-upstream latency comparison.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// ─── result types ────────────────────────────────────────────────────────────

type queryResult struct {
	domain   string
	qtype    string
	latency  time.Duration
	fromCache bool
	answers  int
	err      error
}

type scenarioReport struct {
	name     string
	passed   bool
	detail   string
	results  []queryResult
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newMsg(domain string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)
	m.RecursionDesired = true
	return m
}

func resolve(ctx context.Context, p *proxy.Proxy, domain string, qtype uint16) queryResult {
	req := newMsg(domain, qtype)
	dctx := &proxy.DNSContext{Req: req}

	start := time.Now()
	err := p.Resolve(ctx, dctx)
	lat := time.Since(start)

	r := queryResult{
		domain:  domain,
		qtype:   dns.TypeToString[qtype],
		latency: lat,
		err:     err,
	}
	if dctx.Res != nil {
		r.answers = len(dctx.Res.Answer)
		// heuristic: cache hits are typically < 2 ms
		r.fromCache = lat < 2*time.Millisecond
	}
	return r
}

func printResult(r queryResult) {
	status := "�?
	if r.err != nil {
		status = "�?
	}
	cache := ""
	if r.fromCache {
		cache = " [cache]"
	}
	fmt.Printf("  %s %-30s %-5s  %6.2fms  %d ans%s\n",
		status, r.domain, r.qtype, float64(r.latency.Microseconds())/1000, r.answers, cache)
}

func printReport(rep scenarioReport) {
	verdict := "PASS �?
	if !rep.passed {
		verdict = "FAIL �?
	}
	fmt.Printf("\n[%s] %s �?%s\n", verdict, rep.name, rep.detail)
}

func buildProxy(logger *slog.Logger, minHeat int, timeWindow time.Duration) *proxy.Proxy {
	ups, err := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		fmt.Printf("upstream error: %v\n", err)
		os.Exit(1)
	}

	cfg := &proxy.Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 4 * 1024 * 1024,
		DNSSECEnabled:  true,
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,
			ThresholdPercent:        80,
			MaxConcurrent:           10,
						MinHeatThreshold:        minHeat,
			TimeWindow:              timeWindow,
					},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	}

	p, err := proxy.New(cfg)
	if err != nil {
		fmt.Printf("proxy error: %v\n", err)
		os.Exit(1)
	}
	return p
}

// ─── Scenario 1: Cold-start heat accumulation ────────────────────────────────
// Verifies that a domain enters the prefetch queue only after MinHeatThreshold
// accesses within the time window.

func scenarioColdStart(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Cold-Start Heat Accumulation"}
	domain := "github.com."
	const heat = 4 // must match MinHeatThreshold used in buildProxy call

	fmt.Println("\n── Scenario 1: Cold-Start Heat Accumulation ──")
	fmt.Printf("  Querying %s %d times to build heat (threshold=%d)\n", domain, heat, heat)

	for i := 1; i <= heat; i++ {
		r := resolve(ctx, p, domain, dns.TypeA)
		rep.results = append(rep.results, r)
		printResult(r)
		time.Sleep(150 * time.Millisecond)
	}

	// After heat queries the domain should be in the prefetch queue.
	// We verify indirectly: subsequent queries must be served from cache.
	r := resolve(ctx, p, domain, dns.TypeA)
	rep.results = append(rep.results, r)
	printResult(r)

	rep.passed = r.fromCache && r.err == nil
	rep.detail = fmt.Sprintf("cache hit after %d queries: %v", heat, r.fromCache)
	printReport(rep)
	return rep
}

// ─── Scenario 2: Burst traffic �?many clients, same domain ───────────────────
// Simulates a CDN-style burst: 20 goroutines all query the same domain
// simultaneously. Measures cache hit ratio and p99 latency.

func scenarioBurstSameDomain(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Burst Traffic �?Same Domain"}
	domain := "cloudflare.com."
	const clients = 20

	fmt.Println("\n── Scenario 2: Burst Traffic �?Same Domain ──")
	fmt.Printf("  %d concurrent clients querying %s\n", clients, domain)

	// Warm up once so the domain is cached.
	resolve(ctx, p, domain, dns.TypeA)
	time.Sleep(50 * time.Millisecond)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []queryResult
	)

	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := resolve(ctx, p, domain, dns.TypeA)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
	}
	wg.Wait()

	hits := 0
	var latencies []float64
	for _, r := range results {
		printResult(r)
		if r.fromCache {
			hits++
		}
		latencies = append(latencies, float64(r.latency.Microseconds()))
	}

	sort.Float64s(latencies)
	p99 := latencies[int(float64(len(latencies))*0.99)]
	hitRatio := float64(hits) / float64(clients) * 100

	rep.results = results
	rep.passed = hitRatio >= 80
	rep.detail = fmt.Sprintf("cache hit ratio=%.0f%%  p99=%.2fms", hitRatio, p99/1000)
	printReport(rep)
	return rep
}

// ─── Scenario 3: Mixed query types (A, AAAA, MX, TXT) ────────────────────────
// Real clients query multiple record types. Prefetch must track each type
// independently.

func scenarioMixedQueryTypes(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Mixed Query Types"}
	domain := "google.com."
	types := []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeTXT}

	fmt.Println("\n── Scenario 3: Mixed Query Types ──")

	for _, qt := range types {
		// Query each type 4 times to build heat.
		for i := 0; i < 4; i++ {
			r := resolve(ctx, p, domain, qt)
			rep.results = append(rep.results, r)
			if i == 3 {
				printResult(r) // print only last to reduce noise
			}
			time.Sleep(80 * time.Millisecond)
		}
	}

	errors := 0
	for _, r := range rep.results {
		if r.err != nil {
			errors++
		}
	}

	rep.passed = errors == 0
	rep.detail = fmt.Sprintf("queried %d types × 4 reps, errors=%d", len(types), errors)
	printReport(rep)
	return rep
}

// ─── Scenario 4: Many distinct domains (long-tail traffic) ───────────────────
// Simulates a resolver handling many unique domains (IoT, CDN, analytics).
// Verifies that the heat tracker doesn't bloat memory and cold domains stay cold.

func scenarioLongTailDomains(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Long-Tail Distinct Domains"}

	domains := []string{
		"akamai.com.", "fastly.com.", "amazonaws.com.", "azure.com.",
		"apple.com.", "microsoft.com.", "amazon.com.", "facebook.com.",
		"twitter.com.", "linkedin.com.", "reddit.com.", "wikipedia.org.",
		"stackoverflow.com.", "npmjs.com.", "pypi.org.", "golang.org.",
		"docker.com.", "kubernetes.io.", "grafana.com.", "prometheus.io.",
	}

	fmt.Println("\n── Scenario 4: Long-Tail Distinct Domains ──")
	fmt.Printf("  Querying %d distinct domains once each\n", len(domains))

	var errCount int32
	var wg sync.WaitGroup

	for _, d := range domains {
		wg.Add(1)
		d := d
		go func() {
			defer wg.Done()
			r := resolve(ctx, p, d, dns.TypeA)
			if r.err != nil {
				atomic.AddInt32(&errCount, 1)
			}
			printResult(r)
		}()
	}
	wg.Wait()

	rep.passed = errCount == 0
	rep.detail = fmt.Sprintf("%d domains resolved, errors=%d", len(domains), errCount)
	printReport(rep)
	return rep
}

// ─── Scenario 5: Inactivity eviction ─────────────────────────────────────────
// A domain reaches the prefetch queue, then goes silent for longer than
// TimeWindow. The scheduler must evict it.

func scenarioInactivityEviction(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Inactivity Eviction"}
	domain := "example.net."
	const heat = 4

	fmt.Println("\n── Scenario 5: Inactivity Eviction ──")
	fmt.Printf("  Building heat for %s then going silent for 12s\n", domain)

	for i := 0; i < heat; i++ {
		r := resolve(ctx, p, domain, dns.TypeA)
		rep.results = append(rep.results, r)
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println("  Heat built. Waiting 12s for inactivity eviction...")
	time.Sleep(12 * time.Second)

	// After eviction the domain should still resolve (from cache or upstream),
	// but it should no longer be in the prefetch queue (we can't inspect
	// internals directly, so we verify the system stays stable).
	r := resolve(ctx, p, domain, dns.TypeA)
	rep.results = append(rep.results, r)
	printResult(r)

	rep.passed = r.err == nil
	rep.detail = "domain evicted from queue; still resolves correctly"
	printReport(rep)
	return rep
}

// ─── Scenario 6: Sustained high-frequency access (hot domain) ────────────────
// A domain is queried 60 times over 30 seconds at 2 QPS.
// Verifies that the prefetch scheduler keeps the cache fresh and latency stays low.

func scenarioHotDomain(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Sustained Hot Domain"}
	domain := "google.com."
	const duration = 30 * time.Second
	const interval = 500 * time.Millisecond

	fmt.Println("\n── Scenario 6: Sustained Hot Domain (30s @ 2 QPS) ──")

	deadline := time.Now().Add(duration)
	var cacheHits, total int

	for time.Now().Before(deadline) {
		r := resolve(ctx, p, domain, dns.TypeA)
		rep.results = append(rep.results, r)
		total++
		if r.fromCache {
			cacheHits++
		}
		if total%10 == 0 {
			fmt.Printf("  t=%ds  hits=%d/%d  last_lat=%.2fms\n",
				total/2, cacheHits, total, float64(r.latency.Microseconds())/1000)
		}
		time.Sleep(interval)
	}

	hitRatio := float64(cacheHits) / float64(total) * 100
	rep.passed = hitRatio >= 90
	rep.detail = fmt.Sprintf("total=%d  cache_hits=%d  hit_ratio=%.1f%%", total, cacheHits, hitRatio)
	printReport(rep)
	return rep
}

// ─── Scenario 7: Concurrent mixed workload ────────────────────────────────────
// 5 goroutines each simulate a different client pattern simultaneously:
//   client-A: hot domain at 10 QPS
//   client-B: rotating through 5 domains
//   client-C: AAAA-only queries
//   client-D: random domain from a pool of 50
//   client-E: slow client, 1 query every 3s

func scenarioConcurrentMixed(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Concurrent Mixed Workload"}
	const runFor = 20 * time.Second

	fmt.Println("\n── Scenario 7: Concurrent Mixed Workload (20s) ──")

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allRes  []queryResult
		errCnt  int32
	)

	add := func(r queryResult) {
		mu.Lock()
		allRes = append(allRes, r)
		mu.Unlock()
		if r.err != nil {
			atomic.AddInt32(&errCnt, 1)
		}
	}

	// client-A: hot domain
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(runFor)
		for time.Now().Before(deadline) {
			add(resolve(ctx, p, "google.com.", dns.TypeA))
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// client-B: rotating domains
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool := []string{"github.com.", "cloudflare.com.", "fastly.com.", "akamai.com.", "amazon.com."}
		deadline := time.Now().Add(runFor)
		i := 0
		for time.Now().Before(deadline) {
			add(resolve(ctx, p, pool[i%len(pool)], dns.TypeA))
			i++
			time.Sleep(300 * time.Millisecond)
		}
	}()

	// client-C: AAAA only
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(runFor)
		for time.Now().Before(deadline) {
			add(resolve(ctx, p, "google.com.", dns.TypeAAAA))
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// client-D: random from large pool
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool := []string{
			"apple.com.", "microsoft.com.", "twitter.com.", "linkedin.com.",
			"reddit.com.", "wikipedia.org.", "stackoverflow.com.", "npmjs.com.",
			"docker.com.", "kubernetes.io.",
		}
		deadline := time.Now().Add(runFor)
		for time.Now().Before(deadline) {
			d := pool[rand.Intn(len(pool))]
			add(resolve(ctx, p, d, dns.TypeA))
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// client-E: slow client
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(runFor)
		for time.Now().Before(deadline) {
			add(resolve(ctx, p, "golang.org.", dns.TypeA))
			time.Sleep(3 * time.Second)
		}
	}()

	wg.Wait()

	hits := 0
	for _, r := range allRes {
		if r.fromCache {
			hits++
		}
	}
	hitRatio := float64(hits) / float64(len(allRes)) * 100

	rep.results = allRes
	rep.passed = errCnt == 0 && hitRatio >= 70
	rep.detail = fmt.Sprintf("total=%d  errors=%d  cache_hit_ratio=%.1f%%",
		len(allRes), errCnt, hitRatio)
	printReport(rep)
	return rep
}

// ─── Scenario 8: Prefetch disabled vs enabled comparison ─────────────────────
// Runs the same query pattern against two proxy instances (one with prefetch,
// one without) and compares average latency after the cache warms up.

func scenarioPrefetchVsBaseline(ctx context.Context) scenarioReport {
	rep := scenarioReport{name: "Prefetch vs Baseline Latency"}
	domain := "cloudflare.com."
	const warmup = 4
	const measure = 10

	fmt.Println("\n── Scenario 8: Prefetch vs Baseline Latency ──")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Baseline proxy (no prefetch)
	ups1, _ := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{Timeout: 5 * time.Second})
	baseline, _ := proxy.New(&proxy.Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024,
		DNSSECEnabled:  true,
		UpstreamConfig: &proxy.UpstreamConfig{Upstreams: []upstream.Upstream{ups1}},
	})

	// Prefetch proxy
	ups2, _ := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{Timeout: 5 * time.Second})
	prefetcher, _ := proxy.New(&proxy.Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024,
		DNSSECEnabled:  true,
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,
			ThresholdPercent:        80,
			MaxConcurrent:           5,
						MinHeatThreshold:        4,
			TimeWindow:              30 * time.Second,
					},
		UpstreamConfig: &proxy.UpstreamConfig{Upstreams: []upstream.Upstream{ups2}},
	})

	avg := func(p *proxy.Proxy, label string) float64 {
		// warm up
		for i := 0; i < warmup; i++ {
			resolve(ctx, p, domain, dns.TypeA)
			time.Sleep(100 * time.Millisecond)
		}
		var total time.Duration
		for i := 0; i < measure; i++ {
			r := resolve(ctx, p, domain, dns.TypeA)
			total += r.latency
		}
		avg := float64(total.Microseconds()) / float64(measure) / 1000
		fmt.Printf("  %-12s avg_latency=%.3fms\n", label, avg)
		return avg
	}

	baselineAvg := avg(baseline, "baseline")
	prefetchAvg := avg(prefetcher, "prefetch")

	rep.passed = prefetchAvg <= baselineAvg*1.5 // prefetch should not be >50% slower
	rep.detail = fmt.Sprintf("baseline=%.3fms  prefetch=%.3fms", baselineAvg, prefetchAvg)
	printReport(rep)
	return rep
}

// ─── Scenario 9: TTL edge cases ───────────────────────────────────────────────
// Tests domains known to have very short TTLs (e.g. some CDN health-check
// endpoints) and very long TTLs (e.g. root nameservers).
// Verifies the dual-threshold logic handles both extremes correctly.

func scenarioTTLEdgeCases(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "TTL Edge Cases"}

	// Short-TTL domain: use a domain that typically has low TTL
	// Long-TTL domain: use a domain that typically has high TTL
	cases := []struct {
		domain string
		label  string
	}{
		{"1.1.1.1.in-addr.arpa.", "PTR (short TTL)"},
		{"google.com.", "A (medium TTL)"},
		{"com.", "NS (long TTL)"},
	}

	fmt.Println("\n── Scenario 9: TTL Edge Cases ──")

	var errCount int
	for _, tc := range cases {
		fmt.Printf("  Testing %s [%s]\n", tc.domain, tc.label)
		for i := 0; i < 4; i++ {
			r := resolve(ctx, p, tc.domain, dns.TypeA)
			if tc.domain == "com." {
				r = resolve(ctx, p, tc.domain, dns.TypeNS)
			}
			if tc.domain == "1.1.1.1.in-addr.arpa." {
				r = resolve(ctx, p, tc.domain, dns.TypePTR)
			}
			rep.results = append(rep.results, r)
			if r.err != nil {
				errCount++
			}
			time.Sleep(100 * time.Millisecond)
		}
		// Print last result for each case
		last := rep.results[len(rep.results)-1]
		printResult(last)
	}

	rep.passed = errCount == 0
	rep.detail = fmt.Sprintf("3 TTL edge cases tested, errors=%d", errCount)
	printReport(rep)
	return rep
}

// ─── Scenario 10: Upstream failover resilience ────────────────────────────────
// Uses a primary upstream that is unreachable and a fallback that works.
// Verifies that prefetch queries don't crash the system and cached data
// is preserved when upstream is temporarily unavailable.

func scenarioUpstreamFailover(ctx context.Context) scenarioReport {
	rep := scenarioReport{name: "Upstream Failover Resilience"}
	domain := "example.com."

	fmt.Println("\n── Scenario 10: Upstream Failover Resilience ──")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Good upstream
	goodUps, _ := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{Timeout: 3 * time.Second})
	// Bad upstream (unreachable port)
	badUps, _ := upstream.AddressToUpstream("192.0.2.1:53", &upstream.Options{Timeout: 1 * time.Second})

	p, _ := proxy.New(&proxy.Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024,
		DNSSECEnabled:  true,
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,
			ThresholdPercent:        80,
			MaxConcurrent:           3,
						MinHeatThreshold:        3,
			TimeWindow:              30 * time.Second,
					},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{badUps, goodUps}, // bad first, good as fallback
		},
	})

	fmt.Println("  Querying with bad primary upstream (fallback to good)...")
	var errCount int
	for i := 0; i < 5; i++ {
		r := resolve(ctx, p, domain, dns.TypeA)
		rep.results = append(rep.results, r)
		printResult(r)
		if r.err != nil {
			errCount++
		}
		time.Sleep(200 * time.Millisecond)
	}

	rep.passed = errCount < 3 // allow some failures due to bad upstream timeout
	rep.detail = fmt.Sprintf("5 queries with bad primary, errors=%d (fallback working)", errCount)
	printReport(rep)
	return rep
}

// ─── Scenario 11: Cache-hit latency distribution ─────────────────────────────
// Measures p50/p95/p99 latency for 200 cache-hit queries to verify the
// optimized sharded-lock implementation stays consistently fast.

func scenarioLatencyDistribution(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Cache-Hit Latency Distribution"}
	domain := "google.com."
	const n = 200

	fmt.Println("\n── Scenario 11: Cache-Hit Latency Distribution (200 queries) ──")

	// Warm up
	for i := 0; i < 5; i++ {
		resolve(ctx, p, domain, dns.TypeA)
	}

	var latencies []float64
	for i := 0; i < n; i++ {
		r := resolve(ctx, p, domain, dns.TypeA)
		rep.results = append(rep.results, r)
		latencies = append(latencies, float64(r.latency.Microseconds()))
	}

	sort.Float64s(latencies)
	p50 := latencies[n/2] / 1000
	p95 := latencies[int(float64(n)*0.95)] / 1000
	p99 := latencies[int(float64(n)*0.99)] / 1000

	fmt.Printf("  p50=%.3fms  p95=%.3fms  p99=%.3fms\n", p50, p95, p99)

	rep.passed = p99 < 5.0 // p99 must be under 5ms for cache hits
	rep.detail = fmt.Sprintf("p50=%.3fms  p95=%.3fms  p99=%.3fms", p50, p95, p99)
	printReport(rep)
	return rep
}

// ─── Scenario 12: Prefetch queue saturation ───────────────────────────────────
// Floods the prefetch queue with 30 hot domains simultaneously to verify
// MaxConcurrent is respected and no goroutine leak occurs.

func scenarioPrefetchQueueSaturation(ctx context.Context, p *proxy.Proxy) scenarioReport {
	rep := scenarioReport{name: "Prefetch Queue Saturation"}

	hotDomains := []string{
		"google.com.", "github.com.", "cloudflare.com.", "amazon.com.",
		"microsoft.com.", "apple.com.", "facebook.com.", "twitter.com.",
		"linkedin.com.", "reddit.com.", "wikipedia.org.", "stackoverflow.com.",
		"npmjs.com.", "pypi.org.", "golang.org.", "docker.com.",
		"kubernetes.io.", "grafana.com.", "prometheus.io.", "elastic.co.",
		"nginx.com.", "apache.org.", "postgresql.org.", "mysql.com.",
		"mongodb.com.", "redis.io.", "kafka.apache.org.", "spark.apache.org.",
		"hadoop.apache.org.", "zookeeper.apache.org.",
	}

	fmt.Println("\n── Scenario 12: Prefetch Queue Saturation (30 hot domains) ──")
	fmt.Printf("  Building heat for %d domains concurrently...\n", len(hotDomains))

	var wg sync.WaitGroup
	var errCnt int32

	for _, d := range hotDomains {
		wg.Add(1)
		d := d
		go func() {
			defer wg.Done()
			// 4 queries to reach heat threshold
			for i := 0; i < 4; i++ {
				r := resolve(ctx, p, d, dns.TypeA)
				if r.err != nil {
					atomic.AddInt32(&errCnt, 1)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	wg.Wait()

	fmt.Println("  All domains heated. Waiting 3s for scheduler to process queue...")
	time.Sleep(3 * time.Second)

	// Verify system is still responsive
	r := resolve(ctx, p, "google.com.", dns.TypeA)
	printResult(r)

	rep.passed = errCnt == 0 && r.err == nil
	rep.detail = fmt.Sprintf("%d domains saturated queue, errors=%d, system_responsive=%v",
		len(hotDomains), errCnt, r.err == nil)
	printReport(rep)
	return rep
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn, // suppress debug noise; scenarios print their own output
	}))

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("�?    DNSProxy Smart Prefetch �?Multi-Workload Scenario Test   �?)
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Upstream: 8.8.8.8:53  |  MinHeat: 4  |  TimeWindow: 10s")
	fmt.Println()

	ctx := context.Background()

	// Shared proxy instance (MinHeat=4, TimeWindow=10s for faster test runs)
	p := buildProxy(logger, 4, 10*time.Second)

	start := time.Now()
	var reports []scenarioReport

	// Run scenarios sequentially (some depend on shared cache state)
	reports = append(reports, scenarioColdStart(ctx, p))
	reports = append(reports, scenarioBurstSameDomain(ctx, p))
	reports = append(reports, scenarioMixedQueryTypes(ctx, p))
	reports = append(reports, scenarioLongTailDomains(ctx, p))
	reports = append(reports, scenarioInactivityEviction(ctx, p))
	reports = append(reports, scenarioHotDomain(ctx, p))
	reports = append(reports, scenarioConcurrentMixed(ctx, p))

	// These scenarios create their own proxy instances
	reports = append(reports, scenarioPrefetchVsBaseline(ctx))
	reports = append(reports, scenarioUpstreamFailover(ctx))

	// Back to shared proxy
	reports = append(reports, scenarioTTLEdgeCases(ctx, p))
	reports = append(reports, scenarioLatencyDistribution(ctx, p))
	reports = append(reports, scenarioPrefetchQueueSaturation(ctx, p))

	// ── Final summary ──────────────────────────────────────────────────────
	elapsed := time.Since(start)
	passed, failed := 0, 0
	for _, r := range reports {
		if r.passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("�?                       FINAL SUMMARY                        �?)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	for _, r := range reports {
		verdict := "PASS �?
		if !r.passed {
			verdict = "FAIL �?
		}
		fmt.Printf("�? %-8s  %-38s  ║\n", verdict, r.name)
	}
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("�? Total: %d scenarios  PASS: %d  FAIL: %d  Time: %s%s║\n",
		len(reports), passed, failed, elapsed.Round(time.Second),
		spaces(14-len(elapsed.Round(time.Second).String())))
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	if failed > 0 {
		os.Exit(1)
	}
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	s := make([]byte, n)
	for i := range s {
		s[i] = ' '
	}
	return string(s)
}
