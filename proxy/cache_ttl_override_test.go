package proxy

import (
	"net"
	"testing"

	"github.com/AdguardTeam/golibs/logutil/slogutil"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCache_RespToItem_MinTTL verifies that CacheMinTTL is applied when storing items in cache.
func TestCache_RespToItem_MinTTL(t *testing.T) {
	c := &cache{
		cacheMinTTL: 600, // 10 minutes
		cacheMaxTTL: 0,
	}

	// Create response with TTL=100 seconds
	m := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response: true,
			Rcode:    dns.RcodeSuccess,
		},
		Question: []dns.Question{{
			Name:   "example.com.",
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		}},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    100, // Original TTL = 100 seconds
				},
				A: net.ParseIP("1.2.3.4"),
			},
		},
	}

	logger := slogutil.NewDiscardLogger()
	item := c.respToItem(m, nil, logger)

	require.NotNil(t, item)
	assert.Equal(t, uint32(600), item.ttl, "TTL should be overridden to CacheMinTTL")
}

// TestCache_RespToItem_MaxTTL verifies that CacheMaxTTL is applied when storing items in cache.
func TestCache_RespToItem_MaxTTL(t *testing.T) {
	c := &cache{
		cacheMinTTL: 0,
		cacheMaxTTL: 3600, // 1 hour
	}

	// Create response with TTL=7200 seconds (2 hours)
	m := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response: true,
			Rcode:    dns.RcodeSuccess,
		},
		Question: []dns.Question{{
			Name:   "example.com.",
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		}},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    7200, // Original TTL = 2 hours
				},
				A: net.ParseIP("1.2.3.4"),
			},
		},
	}

	logger := slogutil.NewDiscardLogger()
	item := c.respToItem(m, nil, logger)

	require.NotNil(t, item)
	assert.Equal(t, uint32(3600), item.ttl, "TTL should be overridden to CacheMaxTTL")
}

// TestCache_RespToItem_TTLInRange verifies that TTL is not changed when it's within the range.
func TestCache_RespToItem_TTLInRange(t *testing.T) {
	c := &cache{
		cacheMinTTL: 300,  // 5 minutes
		cacheMaxTTL: 3600, // 1 hour
	}

	// Create response with TTL=600 seconds (10 minutes, within range)
	m := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response: true,
			Rcode:    dns.RcodeSuccess,
		},
		Question: []dns.Question{{
			Name:   "example.com.",
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		}},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    600, // Original TTL = 10 minutes
				},
				A: net.ParseIP("1.2.3.4"),
			},
		},
	}

	logger := slogutil.NewDiscardLogger()
	item := c.respToItem(m, nil, logger)

	require.NotNil(t, item)
	assert.Equal(t, uint32(600), item.ttl, "TTL should remain unchanged when in range")
}

// TestCache_RespToItem_NoOverride verifies that TTL is not changed when overrides are not set.
func TestCache_RespToItem_NoOverride(t *testing.T) {
	c := &cache{
		cacheMinTTL: 0,
		cacheMaxTTL: 0,
	}

	// Create response with TTL=237 seconds (typical Google response)
	m := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response: true,
			Rcode:    dns.RcodeSuccess,
		},
		Question: []dns.Question{{
			Name:   "www.google.com.",
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		}},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    237, // Original TTL = 237 seconds
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	logger := slogutil.NewDiscardLogger()
	item := c.respToItem(m, nil, logger)

	require.NotNil(t, item)
	assert.Equal(t, uint32(237), item.ttl, "TTL should remain unchanged when no overrides are set")
}

// TestNewCache_WithTTLOverrides verifies that newCache properly initializes TTL override fields.
func TestNewCache_WithTTLOverrides(t *testing.T) {
	conf := &cacheConfig{
		size:        1024,
		cacheMinTTL: 300,
		cacheMaxTTL: 3600,
	}

	c := newCache(conf)

	assert.Equal(t, uint32(300), c.cacheMinTTL, "cacheMinTTL should be set")
	assert.Equal(t, uint32(3600), c.cacheMaxTTL, "cacheMaxTTL should be set")
}

// TestNewCache_WithoutTTLOverrides verifies that newCache works without TTL overrides.
func TestNewCache_WithoutTTLOverrides(t *testing.T) {
	conf := &cacheConfig{
		size: 1024,
	}

	c := newCache(conf)

	assert.Equal(t, uint32(0), c.cacheMinTTL, "cacheMinTTL should be 0 by default")
	assert.Equal(t, uint32(0), c.cacheMaxTTL, "cacheMaxTTL should be 0 by default")
}
