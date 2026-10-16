package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// CacheTTL is how long a fetched remote resource stays fresh. It mirrors
// Sub-Store's DEFAULT_CACHE_TTL of one hour.
const CacheTTL = time.Hour

// cacheMaxBytes caps the bodies eligible for caching, mirroring Sub-Store's
// cache threshold. Larger resources are never cached.
const cacheMaxBytes = 1024 << 10 // 1 MiB

// cacheMaxEntries bounds the in-memory cache so a long-running server cannot
// grow it without limit.
const cacheMaxEntries = 512

type cacheEntry struct {
	expires time.Time
	result  Result
}

// inflightFetch lets concurrent callers for the same resource share a single
// upstream request.
type inflightFetch struct {
	done   chan struct{}
	result Result
	err    error
}

// resourceCacheKey derives the cache key from the User-Agent and URL, matching
// Sub-Store's `md5(userAgent + url)` scheme.
func resourceCacheKey(rawURL, userAgent string) string {
	sum := sha256.Sum256([]byte(userAgent + rawURL))
	return hex.EncodeToString(sum[:16])
}

// GetCached behaves like Get but reuses a fresh cached response for the same
// URL and User-Agent. A non-empty userAgent overrides the fetcher default for
// this request only.
//
// When noCache is true the cache is bypassed and a fresh request is always
// made. Concurrent callers for the same resource share one upstream request,
// which prevents a burst of client downloads from tripping upstream rate limits
// such as HTTP 429.
func (f *Fetcher) GetCached(ctx context.Context, rawURL, userAgent string, noCache bool) (Result, error) {
	ua := userAgent
	if ua == "" {
		ua = f.userAgent
	}
	key := resourceCacheKey(rawURL, ua)

	if !noCache {
		if res, ok := f.lookupCache(key); ok {
			return res, nil
		}
	}

	f.cacheMu.Lock()
	if f.inflight == nil {
		f.inflight = map[string]*inflightFetch{}
	}
	if in, ok := f.inflight[key]; ok {
		f.cacheMu.Unlock()
		<-in.done
		return in.result, in.err
	}
	in := &inflightFetch{done: make(chan struct{})}
	f.inflight[key] = in
	f.cacheMu.Unlock()

	res, err := f.Get(ctx, rawURL, userAgent)

	f.cacheMu.Lock()
	delete(f.inflight, key)
	in.result, in.err = res, err
	close(in.done)
	if err == nil && len(res.Content) <= cacheMaxBytes {
		f.storeCacheLocked(key, res)
	}
	f.cacheMu.Unlock()

	return res, err
}

func (f *Fetcher) lookupCache(key string) (Result, bool) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	entry, ok := f.cache[key]
	if !ok {
		return Result{}, false
	}
	if time.Now().After(entry.expires) {
		delete(f.cache, key)
		return Result{}, false
	}
	return entry.result, true
}

// storeCacheLocked records a fresh result, pruning the cache first when it is
// full. The caller must hold cacheMu.
func (f *Fetcher) storeCacheLocked(key string, res Result) {
	if f.cache == nil {
		f.cache = map[string]cacheEntry{}
	}
	if _, exists := f.cache[key]; !exists && len(f.cache) >= cacheMaxEntries {
		now := time.Now()
		for k, entry := range f.cache {
			if now.After(entry.expires) {
				delete(f.cache, k)
			}
		}
		// If everything is still fresh, drop arbitrary entries until there is
		// room. Map iteration order is unspecified, which is fine for an
		// eviction policy that has no correctness requirement.
		for k := range f.cache {
			if len(f.cache) < cacheMaxEntries {
				break
			}
			delete(f.cache, k)
		}
	}
	f.cache[key] = cacheEntry{expires: time.Now().Add(CacheTTL), result: res}
}
