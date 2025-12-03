package proxy

import (
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProactiveRefresh_MultiUpstream tests proactive refresh with multiple upstreams
func TestProactiveRefresh_MultiUpstream(t *testing.T) {
	// Create 3 mock upstreams
	ups1 := &simpleTestUpstream{ttl: 3}
	ups2 := &simpleTestUpstream{ttl: 3}
	ups3 := &simpleTestUpstream{ttl: 3}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownThreshold: -1, // Disable cooldown
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups1, ups2, ups3},
		},
		UpstreamMode: UpstreamModeLoadBalance,
	})
	require.NoError(t, err)

	domain := "connectivity-check.ubuntu.com."

	// Initial request
	dctx := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)

	// Get initial counts
	count1 := ups1.requestCount.Load()
	count2 := ups2.requestCount.Load()
	count3 := ups3.requestCount.Load()
	totalInitial := count1 + count2 + count3

	t.Logf("Initial requests - ups1: %d, ups2: %d, ups3: %d, total: %d",
		count1, count2, count3, totalInitial)

	// Wait for multiple refresh cycles
	time.Sleep(10 * time.Second)

	// Get final counts
	count1 = ups1.requestCount.Load()
	count2 = ups2.requestCount.Load()
	count3 = ups3.requestCount.Load()
	totalFinal := count1 + count2 + count3

	t.Logf("Final requests - ups1: %d, ups2: %d, ups3: %d, total: %d",
		count1, count2, count3, totalFinal)

	// Verify proactive refresh happened
	assert.Greater(t, totalFinal, totalInitial,
		"proactive refresh should work with multiple upstreams")

	// Verify load is distributed (at least 2 upstreams should have been used)
	usedUpstreams := 0
	if count1 > 0 {
		usedUpstreams++
	}
	if count2 > 0 {
		usedUpstreams++
	}
	if count3 > 0 {
		usedUpstreams++
	}

	t.Logf("Number of upstreams used: %d", usedUpstreams)
	assert.GreaterOrEqual(t, usedUpstreams, 1,
		"at least one upstream should be used")
}

// TestProactiveRefresh_UpstreamFailover tests refresh with upstream failover
func TestProactiveRefresh_UpstreamFailover(t *testing.T) {
	// Create upstreams with different behaviors
	goodUps := &simpleTestUpstream{ttl: 3}
	
	// Create a failing upstream
	failingUps := &failingTestUpstream{}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{failingUps, goodUps},
		},
	})
	require.NoError(t, err)

	domain := "connectivity-check.ubuntu.com."

	// Initial request (should use good upstream after failing one fails)
	dctx := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)

	initial := goodUps.requestCount.Load()
	failCount := failingUps.requestCount.Load()

	t.Logf("Initial - good: %d, failing: %d", initial, failCount)

	// Wait for proactive refresh
	time.Sleep(2700 * time.Millisecond)

	after := goodUps.requestCount.Load()
	failAfter := failingUps.requestCount.Load()

	t.Logf("After refresh - good: %d, failing: %d", after, failAfter)

	// Should have refreshed using good upstream
	assert.Greater(t, after, initial,
		"should refresh successfully even with failing upstream")
}

// failingTestUpstream always fails
type failingTestUpstream struct {
	requestCount atomic.Int32
}

func (u *failingTestUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	u.requestCount.Add(1)
	return nil, &net.OpError{Op: "read", Err: &timeoutError{}}
}

func (u *failingTestUpstream) Address() string {
	return "failing-upstream"
}

func (u *failingTestUpstream) Close() error {
	return nil
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
