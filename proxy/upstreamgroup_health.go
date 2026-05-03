package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// HealthCheckConfig contains configuration for upstream health checking.
type HealthCheckConfig struct {
	// Enabled indicates whether health checking is enabled.
	Enabled bool

	// Interval is the time between health checks.
	Interval time.Duration

	// Timeout is the timeout for each health check.
	Timeout time.Duration

	// FailureThreshold is the number of consecutive failures before marking unhealthy.
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes before marking healthy.
	SuccessThreshold int

	// TestDomain is the domain to query for health checks.
	TestDomain string
}

// DefaultHealthCheckConfig returns the default health check configuration.
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Enabled:          false,
		Interval:         30 * time.Second,
		Timeout:          5 * time.Second,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		TestDomain:       "dns.google.",
	}
}

// UpstreamHealth tracks the health status of an upstream server.
type UpstreamHealth struct {
	// Upstream is the upstream server being monitored.
	Upstream upstream.Upstream

	// Healthy indicates if the upstream is currently healthy.
	Healthy bool

	// LastCheck is the time of the last health check.
	LastCheck time.Time

	// LastSuccess is the time of the last successful check.
	LastSuccess time.Time

	// LastFailure is the time of the last failed check.
	LastFailure time.Time

	// ConsecutiveFailures is the number of consecutive failures.
	ConsecutiveFailures int

	// ConsecutiveSuccesses is the number of consecutive successes.
	ConsecutiveSuccesses int

	// TotalChecks is the total number of health checks performed.
	TotalChecks int

	// TotalFailures is the total number of failed checks.
	TotalFailures int

	// AverageLatency is the average response time in milliseconds.
	AverageLatency float64

	mu sync.RWMutex
}

// NewUpstreamHealth creates a new UpstreamHealth instance.
func NewUpstreamHealth(u upstream.Upstream) *UpstreamHealth {
	return &UpstreamHealth{
		Upstream: u,
		Healthy:  true, // Start as healthy
	}
}

// IsHealthy returns whether the upstream is currently healthy.
func (uh *UpstreamHealth) IsHealthy() bool {
	uh.mu.RLock()
	defer uh.mu.RUnlock()
	return uh.Healthy
}

// GetStats returns the current health statistics.
func (uh *UpstreamHealth) GetStats() HealthStats {
	uh.mu.RLock()
	defer uh.mu.RUnlock()

	return HealthStats{
		Address:              uh.Upstream.Address(),
		Healthy:              uh.Healthy,
		LastCheck:            uh.LastCheck,
		LastSuccess:          uh.LastSuccess,
		LastFailure:          uh.LastFailure,
		ConsecutiveFailures:  uh.ConsecutiveFailures,
		ConsecutiveSuccesses: uh.ConsecutiveSuccesses,
		TotalChecks:          uh.TotalChecks,
		TotalFailures:        uh.TotalFailures,
		AverageLatency:       uh.AverageLatency,
	}
}

// HealthStats contains health statistics for an upstream.
type HealthStats struct {
	Address              string
	Healthy              bool
	LastCheck            time.Time
	LastSuccess          time.Time
	LastFailure          time.Time
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	TotalChecks          int
	TotalFailures        int
	AverageLatency       float64
}

// HealthChecker performs health checks on upstream servers.
type HealthChecker struct {
	config  *HealthCheckConfig
	logger  *slog.Logger
	healths map[string]*UpstreamHealth
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(config *HealthCheckConfig, logger *slog.Logger) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HealthChecker{
		config:  config,
		logger:  logger,
		healths: make(map[string]*UpstreamHealth),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AddUpstream adds an upstream to be monitored.
func (hc *HealthChecker) AddUpstream(u upstream.Upstream) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	addr := u.Address()
	if _, exists := hc.healths[addr]; !exists {
		hc.healths[addr] = NewUpstreamHealth(u)
		hc.logger.Debug("added upstream to health checker", "address", addr)
	}
}

// Start begins health checking.
func (hc *HealthChecker) Start() {
	if !hc.config.Enabled {
		hc.logger.Info("health checking is disabled")
		return
	}

	hc.wg.Add(1)
	go hc.run()

	hc.logger.Info(
		"health checker started",
		"interval", hc.config.Interval,
		"timeout", hc.config.Timeout,
	)
}

// Stop stops health checking.
func (hc *HealthChecker) Stop() {
	hc.cancel()
	hc.wg.Wait()
	hc.logger.Info("health checker stopped")
}

// run is the main health checking loop.
func (hc *HealthChecker) run() {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	// Perform initial check
	hc.checkAll()

	for {
		select {
		case <-hc.ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll()
		}
	}
}

// checkAll performs health checks on all upstreams.
func (hc *HealthChecker) checkAll() {
	hc.mu.RLock()
	upstreams := make([]*UpstreamHealth, 0, len(hc.healths))
	for _, uh := range hc.healths {
		upstreams = append(upstreams, uh)
	}
	hc.mu.RUnlock()

	for _, uh := range upstreams {
		hc.check(uh)
	}
}

// check performs a health check on a single upstream.
func (hc *HealthChecker) check(uh *UpstreamHealth) {
	start := time.Now()

	// Create DNS query
	req := &dns.Msg{}
	req.SetQuestion(hc.config.TestDomain, dns.TypeA)
	req.RecursionDesired = true

	// Perform query with timeout
	ctx, cancel := context.WithTimeout(hc.ctx, hc.config.Timeout)
	defer cancel()

	resp, err := uh.Upstream.Exchange(req)
	latency := time.Since(start)
	_ = ctx // Suppress unused variable warning

	uh.mu.Lock()
	defer uh.mu.Unlock()

	uh.LastCheck = time.Now()
	uh.TotalChecks++

	// Update average latency
	if uh.AverageLatency == 0 {
		uh.AverageLatency = float64(latency.Milliseconds())
	} else {
		uh.AverageLatency = (uh.AverageLatency*0.8 + float64(latency.Milliseconds())*0.2)
	}

	// Check if successful
	success := err == nil && resp != nil && resp.Rcode == dns.RcodeSuccess

	if success {
		uh.LastSuccess = time.Now()
		uh.ConsecutiveSuccesses++
		uh.ConsecutiveFailures = 0

		// Mark as healthy if threshold met
		if !uh.Healthy && uh.ConsecutiveSuccesses >= hc.config.SuccessThreshold {
			uh.Healthy = true
			hc.logger.Info(
				"upstream marked healthy",
				"address", uh.Upstream.Address(),
				"latency_ms", latency.Milliseconds(),
			)
		}
	} else {
		uh.LastFailure = time.Now()
		uh.ConsecutiveFailures++
		uh.ConsecutiveSuccesses = 0
		uh.TotalFailures++

		// Mark as unhealthy if threshold met
		if uh.Healthy && uh.ConsecutiveFailures >= hc.config.FailureThreshold {
			uh.Healthy = false
			hc.logger.Warn(
				"upstream marked unhealthy",
				"address", uh.Upstream.Address(),
				"error", err,
			)
		}
	}
}

// GetHealthyUpstreams returns only the healthy upstreams from the given list.
func (hc *HealthChecker) GetHealthyUpstreams(upstreams []upstream.Upstream) []upstream.Upstream {
	if !hc.config.Enabled {
		return upstreams
	}

	hc.mu.RLock()
	defer hc.mu.RUnlock()

	healthy := make([]upstream.Upstream, 0, len(upstreams))
	for _, u := range upstreams {
		if uh, exists := hc.healths[u.Address()]; exists {
			if uh.IsHealthy() {
				healthy = append(healthy, u)
			}
		} else {
			// If not monitored, assume healthy
			healthy = append(healthy, u)
		}
	}

	// If all are unhealthy, return all (fallback)
	if len(healthy) == 0 {
		hc.logger.Warn("all upstreams unhealthy, using all as fallback")
		return upstreams
	}

	return healthy
}

// GetAllStats returns health statistics for all upstreams.
func (hc *HealthChecker) GetAllStats() []HealthStats {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	stats := make([]HealthStats, 0, len(hc.healths))
	for _, uh := range hc.healths {
		stats = append(stats, uh.GetStats())
	}

	return stats
}

// GetStats returns health statistics for a specific upstream.
func (hc *HealthChecker) GetStats(address string) (HealthStats, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if uh, exists := hc.healths[address]; exists {
		return uh.GetStats(), nil
	}

	return HealthStats{}, fmt.Errorf("upstream %q not found", address)
}
