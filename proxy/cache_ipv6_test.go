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

// IPv6 test upstream
type ipv6TestUpstream struct {
	requestCount atomic.Int32
	ttl          uint32
}

func (u *ipv6TestUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	u.requestCount.Add(1)

	resp := &dns.Msg{}
	resp.SetReply(req)
	
	if len(req.Question) > 0 {
		q := req.Question[0]
		
		if q.Qtype == dns.TypeAAAA {
			// 返回 IPv6 地址 (2001:4860:4860::8888 - Google Public DNS IPv6)
			resp.Answer = []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    u.ttl,
					},
					AAAA: net.ParseIP("2001:4860:4860::8888"),
				},
			}
		}
	}
	
	return resp, nil
}

func (u *ipv6TestUpstream) Address() string {
	return "ipv6-test-upstream"
}

func (u *ipv6TestUpstream) Close() error {
	return nil
}

// TestIPv6CacheAndRefresh 测试 IPv6 (AAAA记录) 的缓存和主动刷新机制
func TestIPv6CacheAndRefresh(t *testing.T) {
	ups := &ipv6TestUpstream{ttl: 3} // 3秒 TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500, // 500ms 主动刷新
		CacheProactiveCooldownThreshold: -1,  // 禁用冷却，方便测试
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	testDomain := "ipv6.google.com."

	t.Log("=== 阶段1: 首次查询 IPv6 地址 ===")
	dctx1 := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	// 修改查询类型为 AAAA
	dctx1.Req.Question[0].Qtype = dns.TypeAAAA
	
	err = proxy.Resolve(dctx1)
	require.NoError(t, err)
	require.NotNil(t, dctx1.Res)
	assert.Len(t, dctx1.Res.Answer, 1, "应该返回1个AAAA记录")
	
	if len(dctx1.Res.Answer) > 0 {
		aaaa, ok := dctx1.Res.Answer[0].(*dns.AAAA)
		require.True(t, ok, "应该是AAAA记录")
		t.Logf("首次查询结果: %s", aaaa.AAAA.String())
	}
	
	initialCount := ups.requestCount.Load()
	assert.Equal(t, int32(1), initialCount, "首次查询应该调用上游")

	t.Log("\n=== 阶段2: 立即再次查询（应该命中缓存）===")
	dctx2 := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctx2.Req.Question[0].Qtype = dns.TypeAAAA
	
	err = proxy.Resolve(dctx2)
	require.NoError(t, err)
	
	cacheCount := ups.requestCount.Load()
	assert.Equal(t, initialCount, cacheCount, "缓存命中，不应该调用上游")

	t.Log("\n=== 阶段3: 等待主动刷新执行 ===")
	// 等待主动刷新时间 (3s - 500ms = 2.5s)
	time.Sleep(2700 * time.Millisecond)
	
	refreshCount := ups.requestCount.Load()
	assert.Greater(t, refreshCount, initialCount, "应该发生了主动刷新")
	t.Logf("主动刷新后，上游调用次数: %d", refreshCount)

	t.Log("\n=== 阶段4: 验证刷新后的缓存 ===")
	dctx3 := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctx3.Req.Question[0].Qtype = dns.TypeAAAA
	
	err = proxy.Resolve(dctx3)
	require.NoError(t, err)
	assert.Len(t, dctx3.Res.Answer, 1, "应该返回刷新后的AAAA记录")
	
	previousCallCount := refreshCount
	
	t.Log("\n=== 阶段5: 验证循环刷新 ===")
	// 再等待一个刷新周期
	time.Sleep(2700 * time.Millisecond)
	
	loopCount := ups.requestCount.Load()
	assert.Greater(t, loopCount, previousCallCount, "应该发生了循环刷新")
	t.Logf("循环刷新后，上游调用次数: %d", loopCount)

	t.Log("\n=== 测试总结 ===")
	t.Logf("✓ IPv6 (AAAA) 记录缓存正常")
	t.Logf("✓ 主动刷新机制工作正常")
	t.Logf("✓ 循环刷新机制工作正常")
	t.Logf("✓ 总上游调用次数: %d", loopCount)
}

// dualStackUpstream 支持 A 和 AAAA 记录的上游
type dualStackUpstream struct {
	aCount    atomic.Int32
	aaaaCount atomic.Int32
	ttl       uint32
}

func (u *dualStackUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	resp := &dns.Msg{}
	resp.SetReply(req)
	
	if len(req.Question) > 0 {
		q := req.Question[0]
		
		if q.Qtype == dns.TypeA {
			u.aCount.Add(1)
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    u.ttl,
					},
					A: net.ParseIP("8.8.8.8"),
				},
			}
		} else if q.Qtype == dns.TypeAAAA {
			u.aaaaCount.Add(1)
			resp.Answer = []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    u.ttl,
					},
					AAAA: net.ParseIP("2001:4860:4860::8888"),
				},
			}
		}
	}
	
	return resp, nil
}

func (u *dualStackUpstream) Address() string {
	return "dual-stack-upstream"
}

func (u *dualStackUpstream) Close() error {
	return nil
}

// TestIPv6AndIPv4Separate 测试 IPv6 和 IPv4 记录分别缓存
func TestIPv6AndIPv4Separate(t *testing.T) {
	ups := &dualStackUpstream{ttl: 3}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownThreshold: -1, // 禁用冷却
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	testDomain := "dual.google.com."

	t.Log("=== 测试 A 和 AAAA 记录分别缓存 ===")
	
	// 查询 A 记录
	dctxA := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctxA.Req.Question[0].Qtype = dns.TypeA
	
	err = proxy.Resolve(dctxA)
	require.NoError(t, err)
	assert.Equal(t, int32(1), ups.aCount.Load(), "A记录首次查询")
	assert.Equal(t, int32(0), ups.aaaaCount.Load(), "不应该查询AAAA")
	assert.Len(t, dctxA.Res.Answer, 1)

	// 查询 AAAA 记录
	dctxAAAA := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctxAAAA.Req.Question[0].Qtype = dns.TypeAAAA
	
	err = proxy.Resolve(dctxAAAA)
	require.NoError(t, err)
	assert.Equal(t, int32(1), ups.aCount.Load(), "A记录不应该再次查询")
	assert.Equal(t, int32(1), ups.aaaaCount.Load(), "AAAA记录首次查询")
	assert.Len(t, dctxAAAA.Res.Answer, 1)

	// 再次查询 A 记录（应该命中缓存）
	dctxA2 := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctxA2.Req.Question[0].Qtype = dns.TypeA
	
	err = proxy.Resolve(dctxA2)
	require.NoError(t, err)
	assert.Equal(t, int32(1), ups.aCount.Load(), "A记录缓存命中")

	// 再次查询 AAAA 记录（应该命中缓存）
	dctxAAAA2 := &DNSContext{
		Req:  createTestMsg(testDomain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	dctxAAAA2.Req.Question[0].Qtype = dns.TypeAAAA
	
	err = proxy.Resolve(dctxAAAA2)
	require.NoError(t, err)
	assert.Equal(t, int32(1), ups.aaaaCount.Load(), "AAAA记录缓存命中")

	t.Log("\n=== 等待主动刷新 ===")
	// 等待主动刷新 (3s - 500ms = 2.5s)
	time.Sleep(2700 * time.Millisecond)

	aAfterRefresh := ups.aCount.Load()
	aaaaAfterRefresh := ups.aaaaCount.Load()

	t.Logf("\n=== 测试总结 ===")
	t.Logf("✓ A记录上游调用: %d次", aAfterRefresh)
	t.Logf("✓ AAAA记录上游调用: %d次", aaaaAfterRefresh)
	t.Logf("✓ A和AAAA记录分别缓存和刷新")
	
	assert.Greater(t, aAfterRefresh, int32(1), "A记录应该发生主动刷新")
	assert.Greater(t, aaaaAfterRefresh, int32(1), "AAAA记录应该发生主动刷新")
}
