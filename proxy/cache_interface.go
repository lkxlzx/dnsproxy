package proxy

import (
	"log/slog"
	"net"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// cacheInterface defines the interface for DNS cache implementations.
// This allows for different cache strategies (basic, prefetch, etc.)
// while maintaining backward compatibility.
type cacheInterface interface {
	// get retrieves a cached response for the given request.
	// Returns the cached item, whether it's expired, and the cache key.
	get(req *dns.Msg) (ci *cacheItem, expired bool, key []byte)

	// getWithSubnet retrieves a cached response considering ECS subnet.
	getWithSubnet(req *dns.Msg, clientIP *net.IPNet) (ci *cacheItem, expired bool, key []byte)

	// set stores a DNS response in the cache.
	set(m *dns.Msg, u upstream.Upstream, l *slog.Logger)

	// setWithSubnet stores a DNS response with ECS subnet information.
	setWithSubnet(m *dns.Msg, u upstream.Upstream, clientIP *net.IPNet, l *slog.Logger)
	
	// clearItems clears the general cache.
	clearItems()
	
	// clearItemsWithSubnet clears the ECS subnet cache.
	clearItemsWithSubnet()
	
	// isOptimistic returns whether the cache is in optimistic mode.
	isOptimistic() bool
}

// Ensure both cache types implement the interface
var (
	_ cacheInterface = (*cache)(nil)
	_ cacheInterface = (*cachePrefetch)(nil)
)


// getBaseCache returns the underlying *cache for testing purposes.
// This is a helper function for internal tests that need to access cache internals.
func getBaseCache(c cacheInterface) *cache {
	switch v := c.(type) {
	case *cache:
		return v
	case *cachePrefetch:
		return v.cache
	default:
		return nil
	}
}
