package proxy

// globalClock has been removed.
//
// Previously it was a shared monotonic counter used to track TTL expiry across
// all cache entries.  The design was flawed: resetting the clock on a successful
// prefetch affected every other entry's remaining-TTL calculation, causing
// incorrect prefetch timing for all other domains.
//
// TTL tracking is now done per-entry using real Unix timestamps stored in
// cacheEntryExt.cachedAt.  See cache_entry_ext.go.
