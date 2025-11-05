package asap

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CachingTokenEvent defines a type to represent different events from the cache
type CachingTokenEvent int

const (
	// CachingTokenEventNone is default uninitialized state
	CachingTokenEventNone CachingTokenEvent = iota

	// CachingTokenEventHit denotes a cache hit event
	CachingTokenEventHit

	// CachingTokenEventMiss denotes a cache miss
	CachingTokenEventMiss

	// CachingTokenEventPurge denotes a cache purge
	CachingTokenEventPurge
)

// TokenCache is a higher level ASAP token cache
type TokenCache interface {
	Get(string) Token
	Store(string, Token)
}

// CachingTokenCallBack defines type for a callback function to get notified on
// various cache events
type CachingTokenCallBack func(CachingTokenEvent)

// CachingToken caches parsed tokens in memory
// Can be used on ingress to avoid parsing tokens & validating them if reused
// Can be used on egress to reuse tokens
type cachingToken struct {
	purge             chan struct{}
	callbackFunc      CachingTokenCallBack
	tokenCache        sync.Map
	tokenCacheSize    int64
	maxTokenCacheSize int64
}

// NewTokenCache returns a token cache
func NewTokenCache(ctx context.Context, maxTokenCacheSize int64,
	callbackFunc CachingTokenCallBack) TokenCache {
	c := &cachingToken{
		purge:             make(chan struct{}, 1),
		callbackFunc:      callbackFunc,
		maxTokenCacheSize: maxTokenCacheSize,
	}

	if c.maxTokenCacheSize == 0 {
		c.maxTokenCacheSize = defaultMaxTokenCacheSize
	}

	// Initiate a background cleanup of expired cached entries
	go c.purgeStaleEntries(ctx)

	return c
}

// invokeCallBack is a helper function to relay cache events
func (v *cachingToken) invokeCallBack(e CachingTokenEvent) {
	if v.callbackFunc != nil {
		v.callbackFunc(e)
	}
}

// purgeStaleEntries clears up expired tokens using a 5 minute timer
func (v *cachingToken) purgeStaleEntries(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			break
		case <-v.purge:
			break
		case <-ctx.Done():
			return
		}

		// Visit all entries in the cache and check for expired tokens & delete
		v.tokenCache.Range(func(key, value interface{}) bool {
			if expiration, ok := value.(time.Time); !ok || expiration.Before(time.Now()) {
				v.tokenCache.Delete(key)
				atomic.AddInt64(&v.tokenCacheSize, -1)
			}

			return true
		})

		// If a callback is registered, invoke it
		go v.invokeCallBack(CachingTokenEventPurge)
	}
}

func (v *cachingToken) Get(token string) Token {
	if val, ok := v.tokenCache.Load(token); ok {
		// Fetch the token expiration from cache for the token
		if tok, ok := val.(Token); ok {
			// Check if token in cache is still valid
			if exp, ok := tok.Claims().Expiration(); ok {
				if exp.After(time.Now()) {
					go v.invokeCallBack(CachingTokenEventHit)
					return tok
				}
			}

			// If the token has expired, evict it from cache
			v.tokenCache.Delete(token)
			atomic.AddInt64(&v.tokenCacheSize, -1)
		}
	}

	go v.invokeCallBack(CachingTokenEventMiss)
	return nil
}

func (v *cachingToken) Store(jwt string, token Token) {
	// Do we have a token that has not yet expired
	expiration, _ := token.Claims().Expiration()
	if expiration.After(time.Now()) {
		// Check if we have enough room to cache the token
		if atomic.LoadInt64(&v.tokenCacheSize) < v.maxTokenCacheSize {
			v.tokenCache.Store(jwt, token)
			atomic.AddInt64(&v.tokenCacheSize, 1)
		} else if len(v.purge) < cap(v.purge) {
			// Initiate a purge of stale entries to make room in the background
			var trigger struct{}
			v.purge <- trigger
		}
	}
}
