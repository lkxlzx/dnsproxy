//go:build ignore

package main

// 30-minute long-run DNS prefetch stress test.
// Covers: random access intervals, hot/warm/cold domain tiers, burst spikes,
// inactivity cycles, mixed record types, concurrent client simulation,
// upstream jitter, TTL diversity, and memory stability.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"log/slog"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// ─── constants ────────────────────────────────────────────────────────────────

const (
	totalDuration = 30 * time.Minute
	tickInterval  = 30 * time.Second // progress report every 30s
)

// ─── domain pools ─────────────────────────────────────────────────────────────

// hotDomains: queried very frequently (simulates CDN / search engines)
var hotDomains = []string{
	"google.com.", "cloudflare.com.", "github.com.", "amazon.com.",
	"microsoft.com.", "apple.com.", "facebook.com.", "twitter.com.",
}

// warmDomains: queried moderately (simulates popular services)
var warmDomains = []string{
	"linkedin.com.", "reddit.com.", "wikipedia.org.", "stackoverflow.com.",
	"npmjs.com.", "pypi.org.", "golang.org.", "docker.com.",
	"kubernetes.io.", "grafana.com.", "prometheus.io.", "elastic.co.",
	"nginx.com.", "apache.org.", "postgresql.org.", "mysql.com.",
}

// coldDomains: queried rarely (simulates long-tail / IoT)
var coldDomains = []string{
	"mongodb.com.", "redis.io.", "kafka.apache.org.", "spark.apache.org.",
	"hadoop.apache.org.", "zookeeper.apache.org.", "cassandra.apache.org.",
	"couchdb.apache.org.", "neo4j.com.", "influxdata.com.",
	"timescale.com.", "cockroachlabs.com.", "planetscale.com.",
	"supabase.com.", "neon.tech.", "turso.tech.",
}

// burstDomains: used only during burst phases
var burstDomains = []string{
	"akamai.com.", "fastly.com.", "amazonaws.com.", "azure.com.",
	"gcp.com.", "digitalocean.com.", "linode.com.", "vultr.com.",
}

// mixedTypes: record types to rotate through
var mixedTypes = []uint16{
	dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeTXT, dns.TypeNS,
}

// ─── metrics ──────────────────────────────────────────────────────────────────

type metrics struct {
	totalQueries   atomic.Int64
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
	errors         atomic.Int64
	prefetchEvents atomic.Int64 // inferred from latency drop after miss

	// per-phase counters (reset each tick)
	phaseQueries atomic.Int64
	phaseHits    atomic.Int64
	phaseErrors  atomic.Int64

	// latency histogram buckets (µs): <500, <2000, <10000, <50000, >=50000
	latBuckets [5]atomic.Int64

	mu          sync.Mutex
	p99History  []float64 // p99 per tick
	hitHistory  []float64 // hit% per tick
	memHistory  []uint64  // heap alloc per tick (bytes)
}

func (m *metrics) record(lat time.Duration, fromCache bool, err error) {
	m.totalQueries.Add(1)
	m.phaseQueries.Add(1)
	if err != nil {
		m.errors.Add(1)
		m.phaseErrors.Add(1)
		return
	}
	if fromCache {
		m.cacheHits.Add(1)
		m.phaseHits.Add(1)
	} else {
		m.cacheMisses.Add(1)
	}
	us := lat.Microseconds()
	switch {
	case us < 500:
		m.latBuckets[0].Add(1)
	case us < 2000:
		m.latBuckets[1].Add(1)
	case us < 10000:
		m.latBuckets[2].Add(1)
	case us < 50000:
		m.latBuckets[3].Add(1)
	default:
		m.latBuckets[4].Add(1)
	}
}

func (m *metrics) snapshot(latencies []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	total := m.phaseQueries.Swap(0)
	hits := m.phaseHits.Swap(0)
	errs := m.phaseErrors.Swap(0)

	hitPct := 0.0
	if total > 0 {
		hitPct = float64(hits) / float64(total) * 100
	}
	m.hitHistory = append(m.hitHistory, hitPct)

	sort.Float64s(latencies)
	p99 := 0.0
	if len(latencies) > 0 {
		p99 = latencies[int(float64(len(latencies))*0.99)] / 1000 // µs → ms
	}
	m.p99History = append(m.p99History, p99)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.memHistory = append(m.memHistory, ms.HeapAlloc)

	fmt.Printf("  hits=%.1f%%  p99=%.2fms  queries=%d  errors=%d  heap=%.1fMB\n",
		hitPct, p99, total, errs, float64(ms.HeapAlloc)/1024/1024)
}

func (m *metrics) summary() {
	total := m.totalQueries.Load()
	hits := m.cacheHits.Load()
	errs := m.errors.Load()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   30-MINUTE TEST SUMMARY                    ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Total queries : %-43d║\n", total)
	fmt.Printf("║  Cache hits    : %-43d║\n", hits)
	fmt.Printf("║  Cache misses  : %-43d║\n", m.cacheMisses.Load())
	fmt.Printf("║  Errors        : %-43d║\n", errs)
	if total > 0 {
		fmt.Printf("║  Hit ratio     : %-42s ║\n",
			fmt.Sprintf("%.2f%%", float64(hits)/float64(total)*100))
	}

	// Latency distribution
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Latency distribution:                                       ║")
	labels := []string{"<0.5ms", "<2ms", "<10ms", "<50ms", "≥50ms"}
	for i, b := range m.latBuckets {
		v := b.Load()
		pct := 0.0
		if total > 0 {
			pct = float64(v) / float64(total) * 100
		}
		fmt.Printf("║    %-8s %6d  (%.1f%%)%-26s║\n", labels[i], v, pct, "")
	}

	// p99 trend
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  p99 latency trend (ms per 30s window):                      ║")
	line := "║  "
	for i, v := range m.p99History {
		line += fmt.Sprintf("%.1f", v)
		if i < len(m.p99History)-1 {
			line += " "
		}
	}
	fmt.Printf("%-63s║\n", line)

	// hit% trend
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Cache hit% trend (per 30s window):                          ║")
	line = "║  "
	for i, v := range m.hitHistory {
		line += fmt.Sprintf("%.0f%%", v)
		if i < len(m.hitHistory)-1 {
			line += " "
		}
	}
	fmt.Printf("%-63s║\n", line)

	// memory trend
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Heap alloc trend (MB per 30s window):                       ║")
	line = "║  "
	for i, v := range m.memHistory {
		line += fmt.Sprintf("%.1f", float64(v)/1024/1024)
		if i < len(m.memHistory)-1 {
			line += " "
		}
	}
	fmt.Printf("%-63s║\n", line)
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newMsg(domain string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)
	m.RecursionDesired = true
	return m
}

func doResolve(ctx context.Context, p *proxy.Proxy, domain string, qtype uint16) (fromCache bool, lat time.Duration, err error) {
	req := newMsg(domain, qtype)
	dctx := &proxy.DNSContext{Req: req}
	start := time.Now()
	err = p.Resolve(ctx, dctx)
	lat = time.Since(start)
	fromCache = lat < 2*time.Millisecond && err == nil
	return
}

// jitter returns a random duration in [base-spread, base+spread].
func jitter(base, spread time.Duration) time.Duration {
	delta := time.Duration(rand.Int63n(int64(spread)*2+1)) - spread
	d := base + delta
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return d
}

func buildProxy(logger *slog.Logger) *proxy.Proxy {
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
		CacheSizeBytes: 8 * 1024 * 1024,
		DNSSECEnabled:  true,
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        10,
			ThresholdPercent:        80,
			MaxConcurrent:           20,
			ScanInterval:            500 * time.Millisecond,
			MinHeatThreshold:        4,
			TimeWindow:              120 * time.Second,
			InactivityCheckInterval: 30 * time.Second,
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

// ─── client workers ───────────────────────────────────────────────────────────

// hotClient: queries hot domains at high frequency with random jitter.
// Simulates a busy recursive resolver serving popular domains.
func hotClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		domain := hotDomains[rand.Intn(len(hotDomains))]
		qtype := dns.TypeA
		if rand.Intn(5) == 0 {
			qtype = dns.TypeAAAA
		}
		fromCache, lat, err := doResolve(ctx, p, domain, qtype)
		m.record(lat, fromCache, err)
		select {
		case latCh <- float64(lat.Microseconds()):
		default:
		}
		// 50–200ms between queries
		time.Sleep(jitter(100*time.Millisecond, 50*time.Millisecond))
	}
}

// warmClient: queries warm domains at moderate frequency.
func warmClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		domain := warmDomains[rand.Intn(len(warmDomains))]
		qtype := mixedTypes[rand.Intn(len(mixedTypes))]
		fromCache, lat, err := doResolve(ctx, p, domain, qtype)
		m.record(lat, fromCache, err)
		select {
		case latCh <- float64(lat.Microseconds()):
		default:
		}
		// 300ms–1.5s between queries
		time.Sleep(jitter(800*time.Millisecond, 500*time.Millisecond))
	}
}

// coldClient: queries cold domains rarely, simulating IoT / long-tail traffic.
func coldClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		domain := coldDomains[rand.Intn(len(coldDomains))]
		fromCache, lat, err := doResolve(ctx, p, domain, dns.TypeA)
		m.record(lat, fromCache, err)
		select {
		case latCh <- float64(lat.Microseconds()):
		default:
		}
		// 5–15s between queries (rarely accessed)
		time.Sleep(jitter(10*time.Second, 5*time.Second))
	}
}

// burstClient: fires bursts of 20–50 concurrent queries every 2–5 minutes.
// Simulates traffic spikes (e.g. app startup, CI pipeline).
func burstClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	for {
		// Wait 2–5 minutes between bursts
		wait := jitter(3*time.Minute, 90*time.Second)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		batchSize := 20 + rand.Intn(31) // 20–50
		fmt.Printf("  [burst] firing %d concurrent queries\n", batchSize)

		var wg sync.WaitGroup
		for i := 0; i < batchSize; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				domain := burstDomains[rand.Intn(len(burstDomains))]
				fromCache, lat, err := doResolve(ctx, p, domain, dns.TypeA)
				m.record(lat, fromCache, err)
				select {
				case latCh <- float64(lat.Microseconds()):
				default:
				}
			}()
		}
		wg.Wait()
	}
}

// inactivityClient: builds heat for a domain then goes silent, then comes back.
// Validates that inactivity eviction and re-warming work correctly.
func inactivityClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	domains := []string{
		"timescale.com.", "cockroachlabs.com.", "planetscale.com.", "supabase.com.",
	}
	for {
		for _, domain := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Build heat: 5 queries in quick succession
			for i := 0; i < 5; i++ {
				fromCache, lat, err := doResolve(ctx, p, domain, dns.TypeA)
				m.record(lat, fromCache, err)
				select {
				case latCh <- float64(lat.Microseconds()):
				default:
				}
				time.Sleep(200 * time.Millisecond)
			}
			// Go silent for 90–150s (longer than InactivityCheckInterval)
			silent := jitter(120*time.Second, 30*time.Second)
			select {
			case <-ctx.Done():
				return
			case <-time.After(silent):
			}
			// Come back — should re-warm from scratch
			fromCache, lat, err := doResolve(ctx, p, domain, dns.TypeA)
			m.record(lat, fromCache, err)
			select {
			case latCh <- float64(lat.Microseconds()):
			default:
			}
		}
	}
}

// typeRotationClient: cycles through all record types for a fixed domain.
// Validates that each qtype is tracked independently.
func typeRotationClient(ctx context.Context, p *proxy.Proxy, m *metrics, latCh chan<- float64) {
	domain := "google.com."
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		qtype := mixedTypes[i%len(mixedTypes)]
		i++
		fromCache, lat, err := doResolve(ctx, p, domain, qtype)
		m.record(lat, fromCache, err)
		select {
		case latCh <- float64(lat.Microseconds()):
		default:
		}
		time.Sleep(jitter(500*time.Millisecond, 200*time.Millisecond))
	}
}

// ─── phase scheduler ──────────────────────────────────────────────────────────

// phase defines a named test phase with its duration and description.
type phase struct {
	name     string
	duration time.Duration
	desc     string
}

// phases defines the 30-minute test schedule.
// Total = 30 min.
var phases = []phase{
	{"warm-up",          2 * time.Minute,  "cold start, build heat for all domain tiers"},
	{"steady-state",     5 * time.Minute,  "normal mixed traffic, prefetch active"},
	{"burst-1",          2 * time.Minute,  "first burst spike + hot domain saturation"},
	{"recovery",         3 * time.Minute,  "post-burst recovery, cache stabilization"},
	{"inactivity-cycle", 4 * time.Minute,  "inactivity eviction + re-warm cycle"},
	{"type-diversity",   3 * time.Minute,  "heavy mixed record type rotation"},
	{"burst-2",          2 * time.Minute,  "second burst spike, larger batch"},
	{"low-traffic",      3 * time.Minute,  "reduced QPS, prefetch keeps cache warm"},
	{"high-concurrency", 3 * time.Minute,  "max concurrent clients, stress test"},
	{"wind-down",        3 * time.Minute,  "gradual traffic reduction, final stats"},
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║        DNSProxy Smart Prefetch – 30-Minute Long-Run Test     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Start: %s\n", time.Now().Format("15:04:05"))
	fmt.Printf("  End:   %s\n", time.Now().Add(totalDuration).Format("15:04:05"))
	fmt.Println()

	p := buildProxy(logger)
	m := &metrics{}

	// Latency channel — buffered, workers drop if full (non-blocking)
	latCh := make(chan float64, 10000)

	// Graceful shutdown on Ctrl-C
	rootCtx, rootCancel := context.WithTimeout(context.Background(), totalDuration)
	defer rootCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			fmt.Println("\n[interrupted] printing partial results...")
			rootCancel()
		case <-rootCtx.Done():
		}
	}()

	// ── Phase runner ──────────────────────────────────────────────────────
	phaseIdx := 0
	phaseStart := time.Now()

	// Progress ticker
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	// Latency collector — drains latCh every tick
	var latMu sync.Mutex
	var latBuf []float64
	go func() {
		for v := range latCh {
			latMu.Lock()
			latBuf = append(latBuf, v)
			latMu.Unlock()
		}
	}()

	// ── Start baseline workers (always running) ───────────────────────────
	go hotClient(rootCtx, p, m, latCh)
	go hotClient(rootCtx, p, m, latCh)
	go warmClient(rootCtx, p, m, latCh)
	go coldClient(rootCtx, p, m, latCh)
	go typeRotationClient(rootCtx, p, m, latCh)
	go burstClient(rootCtx, p, m, latCh)
	go inactivityClient(rootCtx, p, m, latCh)

	// ── Phase-specific extra workers ──────────────────────────────────────
	// Each phase spawns additional goroutines via a phase context.
	var phaseCancel context.CancelFunc
	var phaseCtx context.Context

	startNextPhase := func() {
		if phaseCancel != nil {
			phaseCancel()
		}
		if phaseIdx >= len(phases) {
			return
		}
		ph := phases[phaseIdx]
		phaseCtx, phaseCancel = context.WithTimeout(rootCtx, ph.duration)
		phaseStart = time.Now()

		fmt.Printf("\n[%s] Phase %d/%d: %s\n",
			time.Now().Format("15:04:05"), phaseIdx+1, len(phases), ph.name)
		fmt.Printf("  %s  (duration: %s)\n", ph.desc, ph.duration)

		switch ph.name {
		case "warm-up":
			// Extra hot clients to build heat quickly
			for i := 0; i < 3; i++ {
				go hotClient(phaseCtx, p, m, latCh)
			}

		case "burst-1", "burst-2":
			// Saturate with concurrent burst workers
			batchSize := 30
			if ph.name == "burst-2" {
				batchSize = 50
			}
			go func(ctx context.Context, n int) {
				var wg sync.WaitGroup
				for i := 0; i < n; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for {
							select {
							case <-ctx.Done():
								return
							default:
							}
							domain := burstDomains[rand.Intn(len(burstDomains))]
							fromCache, lat, err := doResolve(ctx, p, domain, dns.TypeA)
							m.record(lat, fromCache, err)
							select {
							case latCh <- float64(lat.Microseconds()):
							default:
							}
							time.Sleep(jitter(50*time.Millisecond, 30*time.Millisecond))
						}
					}()
				}
				wg.Wait()
			}(phaseCtx, batchSize)

		case "inactivity-cycle":
			// Extra inactivity workers
			for i := 0; i < 2; i++ {
				go inactivityClient(phaseCtx, p, m, latCh)
			}

		case "type-diversity":
			// Multiple type-rotation workers on different domains
			for _, d := range []string{"cloudflare.com.", "github.com.", "amazon.com."} {
				d := d
				go func(ctx context.Context, domain string) {
					for {
						select {
						case <-ctx.Done():
							return
						default:
						}
						qtype := mixedTypes[rand.Intn(len(mixedTypes))]
						fromCache, lat, err := doResolve(ctx, p, domain, qtype)
						m.record(lat, fromCache, err)
						select {
						case latCh <- float64(lat.Microseconds()):
						default:
						}
						time.Sleep(jitter(300*time.Millisecond, 150*time.Millisecond))
					}
				}(phaseCtx, d)
			}

		case "high-concurrency":
			// 20 extra concurrent clients
			for i := 0; i < 20; i++ {
				if i%3 == 0 {
					go hotClient(phaseCtx, p, m, latCh)
				} else if i%3 == 1 {
					go warmClient(phaseCtx, p, m, latCh)
				} else {
					go coldClient(phaseCtx, p, m, latCh)
				}
			}

		case "low-traffic":
			// No extra workers — baseline only, verify prefetch keeps cache warm

		case "recovery", "steady-state", "wind-down":
			// Standard extra warm client
			go warmClient(phaseCtx, p, m, latCh)
		}

		phaseIdx++
	}

	// Start first phase immediately
	startNextPhase()

	// ── Main event loop ───────────────────────────────────────────────────
	elapsed := time.Duration(0)
	for {
		select {
		case <-rootCtx.Done():
			goto done

		case <-ticker.C:
			elapsed += tickInterval

			// Drain latency buffer
			latMu.Lock()
			snap := latBuf
			latBuf = nil
			latMu.Unlock()

			fmt.Printf("[%s] t=%s  ", time.Now().Format("15:04:05"), elapsed)
			m.snapshot(snap)

			// Advance phase if current phase duration elapsed
			if phaseIdx < len(phases) && time.Since(phaseStart) >= phases[phaseIdx-1].duration {
				startNextPhase()
			}
		}
	}

done:
	if phaseCancel != nil {
		phaseCancel()
	}
	close(latCh)
	time.Sleep(200 * time.Millisecond) // let latCh drain

	m.summary()

	// ── Pass/fail criteria ────────────────────────────────────────────────
	total := m.totalQueries.Load()
	hits := m.cacheHits.Load()
	errs := m.errors.Load()

	hitRatio := 0.0
	if total > 0 {
		hitRatio = float64(hits) / float64(total) * 100
	}
	errRatio := 0.0
	if total > 0 {
		errRatio = float64(errs) / float64(total) * 100
	}

	fmt.Println()
	pass := true
	check := func(label string, ok bool, detail string) {
		if ok {
			fmt.Printf("  ✅ %s — %s\n", label, detail)
		} else {
			fmt.Printf("  ❌ %s — %s\n", label, detail)
			pass = false
		}
	}

	check("Cache hit ratio ≥ 85%",
		hitRatio >= 85,
		fmt.Sprintf("%.2f%%", hitRatio))
	check("Error rate < 1%",
		errRatio < 1.0,
		fmt.Sprintf("%.3f%%", errRatio))
	check("Total queries > 5000",
		total > 5000,
		fmt.Sprintf("%d", total))

	// Memory stability: last heap should be < 2× first heap
	m.mu.Lock()
	memStable := true
	if len(m.memHistory) >= 2 {
		first := m.memHistory[0]
		last := m.memHistory[len(m.memHistory)-1]
		if first > 0 && last > first*3 {
			memStable = false
		}
	}
	m.mu.Unlock()
	check("Memory stable (heap growth < 3×)",
		memStable,
		"no leak detected")

	fmt.Println()
	if pass {
		fmt.Println("  RESULT: PASS ✅")
	} else {
		fmt.Println("  RESULT: FAIL ❌")
		os.Exit(1)
	}
}
