package proxy

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func BenchmarkBasicCacheGet(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)
	resp := makeTestResp(req, 300)
	c.set(resp, nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.get(req)
	}
}

func BenchmarkBasicCacheSet(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.set(makeTestResp(req, 300), nil, testLogger)
	}
}

func BenchmarkHeatTrackerAccess(b *testing.B) {
	ht := newHeatTracker(6, 180*time.Second)
	now := time.Now()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ht.onAccess("example.com.", dns.TypeA, now)
	}
}

func BenchmarkHeatTrackerOnAccess(b *testing.B) {
	ht := newHeatTracker(6, 180*time.Second)
	now := time.Now()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ht.onAccess("bench.com.", 1, now)
	}
}

func BenchmarkCacheEntryExtIsExpired(b *testing.B) {
	now := time.Now()
	entry := newCacheEntryExt("example.com.", 1, 300, now)
	nowUnix := now.Unix()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = entry.isExpired(nowUnix + 100)
	}
}

func BenchmarkCacheEntryExtRemainingTTL(b *testing.B) {
	now := time.Now()
	entry := newCacheEntryExt("example.com.", 1, 300, now)
	nowUnix := now.Unix()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = entry.remainingTTL(nowUnix + 100)
	}
}

func BenchmarkConcurrentCacheRead(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)
	c.set(makeTestResp(req, 300), nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = c.get(req)
		}
	})
}

func BenchmarkConcurrentCacheAccess(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)
	c.set(makeTestResp(req, 300), nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = c.get(req)
		}
	})
}

func makeTestResp(req *dns.Msg, ttl uint32) *dns.Msg {
	resp := &dns.Msg{}
	resp.SetReply(req)
	resp.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   req.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			A: []byte{1, 2, 3, 4},
		},
	}
	return resp
}
