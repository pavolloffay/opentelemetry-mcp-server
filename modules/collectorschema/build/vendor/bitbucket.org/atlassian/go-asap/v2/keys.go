package asap

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pquerna/cachecontrol/cacheobject"
	"github.com/vincent-petithory/dataurl"
)

// KeyFetcher takes in an ASAP compliant kid and returns the public key
// associated with it for use in verifying tokens.
type KeyFetcher interface {
	Fetch(keyID string) (interface{}, error)
}

// NewPrivateKey attempts to decode the given bytes into a valid private key
// of some type and return something suitable for signing a token.
func NewPrivateKey(privateKeyData []byte) (interface{}, error) {
	var e error
	var privateKey interface{}
	var dataURL *dataurl.DataURL
	// PEM files are typically multi-line, which makes the raw form difficult to be stored in evnironment variables.
	// We first attempt to decode the data. If we fail, then proceed with the original input.
	if dataURL, e = dataurl.DecodeString(string(privateKeyData)); e == nil {
		privateKeyData = dataURL.Data
	}

	privateKey, e = x509.ParsePKCS8PrivateKey(privateKeyData)
	if e == nil {
		return privateKey, nil
	}

	var block, _ = pem.Decode(privateKeyData)
	if block == nil {
		return nil, fmt.Errorf("No valid PEM data found")
	}

	privateKey, e = x509.ParsePKCS1PrivateKey(block.Bytes)
	if e == nil {
		return privateKey, nil
	}

	return x509.ParseECPrivateKey(block.Bytes)
}

// NewMicrosPrivateKey plucks the key from the contracted ENV vars documented
// here: https://extranet.atlassian.com/pages/viewpage.action?pageId=2763562051
func NewMicrosPrivateKey() (interface{}, error) {
	return NewPrivateKey([]byte(os.Getenv("ASAP_PRIVATE_KEY")))
}

// NewPublicKey attempts to decode the given bytes into a valid public key of
// some type and return something suitable for verifying a token signature.
func NewPublicKey(publicKeyData []byte) (interface{}, error) {

	var block, _ = pem.Decode(publicKeyData)
	if block == nil {
		return nil, errors.New("No valid PEM data found")
	}

	return x509.ParsePKIXPublicKey(block.Bytes)
}

type httpFetcher struct {
	baseURL string
	client  *http.Client
}

// NewHTTPKeyFetcher pulls public keys from an HTTP accessible source.
func NewHTTPKeyFetcher(baseURL string, client *http.Client) KeyFetcher {
	return &httpFetcher{baseURL, client}
}

// NewMultiFetcher will return the first non error fetch result
func NewMultiFetcher(fetchers ...KeyFetcher) KeyFetcher {
	return MultiKeyFetcher(fetchers)
}

// NewMicrosKeyFetcher pulls public keys from the shared s3 bucket given as
// part of the ASAP env var contract in Micros. Documentation for contract:
// https://extranet.atlassian.com/pages/viewpage.action?pageId=2763562051
func NewMicrosKeyFetcher(client *http.Client) KeyFetcher {
	return NewMultiFetcher(
		&httpFetcher{
			baseURL: os.Getenv("ASAP_PUBLIC_KEY_REPOSITORY_URL"),
			client:  client,
		},
		&httpFetcher{
			baseURL: os.Getenv("ASAP_PUBLIC_KEY_FALLBACK_REPOSITORY_URL"),
			client:  client,
		},
	)
}

func (f *httpFetcher) Fetch(keyID string) (interface{}, error) {
	var pkURL, e = url.Parse(f.baseURL)
	if e != nil {
		return nil, e
	}
	pkURL.Path = path.Join(pkURL.Path, keyID)

	var resp *http.Response
	resp, e = f.client.Get(pkURL.String())
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body, _ = ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("error fetching %s via HTTP. Code: %d Body: %s", pkURL.String(), resp.StatusCode, string(body))
	}

	var keyBytes []byte
	keyBytes, e = ioutil.ReadAll(resp.Body)
	if e != nil {
		return nil, e
	}

	return NewPublicKey(keyBytes)
}

type cacheFetcher struct {
	lock    sync.RWMutex
	wrapped KeyFetcher
	cache   map[string]interface{}
}

// NewCachingFetcher wraps a given KeyFetcher implementation with an in-memory
// cache for returned keys.
func NewCachingFetcher(wrapped KeyFetcher) KeyFetcher {
	return &cacheFetcher{sync.RWMutex{}, wrapped, make(map[string]interface{})}
}

func (f *cacheFetcher) Fetch(keyID string) (interface{}, error) {
	f.lock.RLock()
	var cached, ok = f.cache[keyID]
	f.lock.RUnlock()
	if ok {
		return cached, nil
	}
	var result, e = f.wrapped.Fetch(keyID)
	if e == nil {
		f.lock.Lock()
		defer f.lock.Unlock()
		f.cache[keyID] = result
	}
	return result, e
}

// MultiKeyFetcher returns the first non error result from its list of fetchers
type MultiKeyFetcher []KeyFetcher

// Fetch iterates through the list of fetchers returning first fetch result that
// succeeds
func (f MultiKeyFetcher) Fetch(key string) (interface{}, error) {
	var pk interface{}
	var errs []string
	var err error
	for _, fetcher := range f {
		pk, err = fetcher.Fetch(key)
		if err == nil {
			return pk, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, errors.New(strings.Join(errs, ", "))
}

// keyExpirationPair contains a public key along with that key's cache expiration time
type keyExpirationPair struct {
	key                  interface{}
	expiration           time.Time
	staleWhileRevalidate time.Duration
}

type keyLookupError struct {
	error
}

type keyLookupMissError struct {
	error
	expiration time.Time
}

type keyLookupBadResponseError struct {
	error
}

var (
	cachedKey           = "asap.key.cache.hit"
	expiredKey          = "asap.key.cache.expired"
	cachedLookupMiss    = "asap.key.cache.lookup_miss"
	cacheMiss           = "asap.key.cache.miss"
	cacheRefreshSuccess = "asap.key.cache.refresh.success"
	cacheForceReload    = "asap.key.cache.refresh.force_reload"
)

const defaultMaxKeyCacheSize = 10000

type expiringCacheFetcher struct {
	purge        chan struct{}
	keyLocks     sync.Map
	baseURL      string
	client       *http.Client
	cache        sync.Map
	timeNow      func() time.Time
	cacheSize    int64
	maxCacheSize int64
	cacheStats   func(stat string, count float64, tags ...string)
}

// NewExpiringCacheFetcher wraps a given KeyFetcher implementation that returns a keyExpirationPair with an in-memory
// cache for returned keys.
func NewExpiringCacheFetcher(baseURL string, client *http.Client, _ time.Duration) (KeyFetcher, error) {
	var _, e = url.Parse(baseURL)
	if e != nil {
		return nil, fmt.Errorf("cannot parse baseURL: %s", e)
	}

	if !strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL + "/"
	}

	ec := &expiringCacheFetcher{
		purge:   make(chan struct{}, 1),
		baseURL: baseURL,
		client:  client,
		timeNow: time.Now,
	}

	if ec.maxCacheSize == 0 {
		ec.maxCacheSize = defaultMaxKeyCacheSize
	}

	return ec, nil
}

func NewExpiringCacheFetcherWithStats(baseURL string, client *http.Client, stats func(stat string, count float64, tags ...string)) (KeyFetcher, error) {
	f, e := NewExpiringCacheFetcher(baseURL, client, 0)
	if e != nil {
		return nil, e
	}
	return f.(*expiringCacheFetcher).WithCacheStats(stats), nil
}

// WithCacheStats adds a "count" statsd function which can increment
// stats related to the expiring cache
func (c *expiringCacheFetcher) WithCacheStats(count func(stat string, count float64, tags ...string)) *expiringCacheFetcher {
	c.cacheStats = count
	return c
}

func (c *expiringCacheFetcher) WithMaxCacheSize(maxCacheSize int64) *expiringCacheFetcher {
	c.maxCacheSize = maxCacheSize
	return c
}

func (c *expiringCacheFetcher) incr(stat string) {
	if c.cacheStats == nil {
		return
	}

	c.cacheStats(stat, 1)
}

// getExpiryAndStaleOk parses the Cache Control header and returns the time when the key expires AND
// its stale-while-revalidate duration, (i.e. how long after max-age, it's ok to use a cached key but
// needs to be "revalidated")
func getExpiryAndStaleOk(header http.Header, timeNow func() time.Time) (time.Time, time.Duration) {
	cacheControl, ok := header["Cache-Control"]
	if !ok {
		return getExpiresTime(header, timeNow)
	}
	responseDirs, err := cacheobject.ParseResponseCacheControl(strings.Join(cacheControl, ","))
	if err != nil {
		// this implies malformed cache-control header (e.g. non-number values) => don't cache (i.e. expires now)
		return getExpiresTime(header, timeNow)
	}

	if responseDirs.MaxAge != -1 {
		if responseDirs.StaleWhileRevalidate != -1 {
			// don't consider response expired until after max-age + stale-while-revalidate
			// if after max-age but less than max-age + stale-while-revalidate, still good to use
			// but need to "revalidate" async
			return timeNow().Add(time.Duration(responseDirs.MaxAge) * time.Second),
				time.Duration(responseDirs.StaleWhileRevalidate) * time.Second
		}
		return timeNow().Add(time.Duration(responseDirs.MaxAge) * time.Second), time.Duration(0)
	}

	return getExpiresTime(header, timeNow)
}

func getExpiresTime(header http.Header, timeNow func() time.Time) (time.Time, time.Duration) {
	expires, e := http.ParseTime(header.Get("Expires"))
	if e != nil {
		// this implies malformed date in expires header => don't cache (i.e. expires now)
		return timeNow().Add(time.Minute * 10), time.Duration(time.Minute * 20)
	}

	return expires, time.Duration(0)
}

func (f *expiringCacheFetcher) fetchHTTPKey(keyID string) (keyExpirationPair, error) {
	// keyIDs cannot be prefixed with a leading slash, and baseURL includes a trailing /.
	// using path.Join here turns the base url from http://example.com to http:/example.com, which is invalid.
	httpURL := f.baseURL + keyID
	resp, err := f.client.Get(httpURL)
	if err != nil {
		return keyExpirationPair{}, keyLookupError{fmt.Errorf("failed obtaining http response: %s", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body, _ = ioutil.ReadAll(resp.Body)
		err := fmt.Errorf("error fetching %s via HTTP. Code: %d Body: %s", httpURL, resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			return keyExpirationPair{}, keyLookupMissError{err, f.timeNow().Add(time.Second)}
		}
		return keyExpirationPair{}, keyLookupError{err}
	}

	expiry, staleOk := getExpiryAndStaleOk(resp.Header, f.timeNow)
	var keyBytes []byte
	keyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return keyExpirationPair{}, keyLookupBadResponseError{fmt.Errorf("failure reading response body: %s", err)}
	}

	key, e := NewPublicKey(keyBytes)
	if e != nil {
		return keyExpirationPair{}, keyLookupBadResponseError{fmt.Errorf("failure parsing response body as public key: %s", e)}
	}

	return keyExpirationPair{key, expiry, staleOk}, nil
}

func (f *expiringCacheFetcher) Fetch(keyID string) (interface{}, error) {
	var value, ok = f.cache.Load(keyID)
	if ok {
		var cached, convertCheck = value.(keyExpirationPair)
		if convertCheck && cached.expiration.After(f.timeNow()) {
			f.incr(cachedKey)
			return cached.key, nil
		} else if convertCheck && cached.expiration.Add(cached.staleWhileRevalidate).After(f.timeNow()) {
			go f.reloadOrPurge(keyID)
			f.incr(expiredKey)
			return cached.key, nil
		}

		if failed, ok := value.(keyLookupMissError); ok && failed.expiration.After(f.timeNow()) {
			f.incr(cachedLookupMiss)
			return nil, failed
		}
	}

	f.incr(cacheMiss)
	return f.reload(keyID)
}

func (f *expiringCacheFetcher) reloadOrPurge(keyID string) {
	_, err := f.reload(keyID)
	if _, ok := err.(keyLookupBadResponseError); ok {
		f.cache.Delete(keyID)
		atomic.AddInt64(&f.cacheSize, -1)
	}
}

func (f *expiringCacheFetcher) reload(keyID string) (interface{}, error) {
	// Cache miss identified. Wait for (potentially) another cache refresh
	lock, loaded := f.keyLocks.LoadOrStore(keyID, &sync.Mutex{})

	if !loaded {
		atomic.AddInt64(&f.cacheSize, 1)
	}

	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	// Check if cache has been repopulated
	if value, ok := f.cache.Load(keyID); ok {
		var cached, ok = value.(keyExpirationPair)
		if ok && cached.expiration.After(f.timeNow()) {
			f.incr(cacheRefreshSuccess)
			return cached.key, nil
		}

		if failed, ok := value.(keyLookupMissError); ok && failed.expiration.After(f.timeNow()) {
			return nil, failed
		}
	}

	// Repopulate key
	result, err := f.fetchHTTPKey(keyID)
	if err != nil {
		if v, ok := err.(keyLookupMissError); ok {
			f.Store(keyID, v)
		}
		return nil, err
	}
	f.incr(cacheForceReload)
	f.Store(keyID, result)
	return result.key, nil
}

func (f *expiringCacheFetcher) Close() error {
	return nil
}

func (f *expiringCacheFetcher) Store(k string, v interface{}) {
	if atomic.LoadInt64(&f.cacheSize) < f.maxCacheSize {
		f.cache.Store(k, v)
		atomic.AddInt64(&f.cacheSize, 1)
	} else if len(f.purge) < cap(f.purge) {
		var trigger struct{}
		f.purge <- trigger
	}
}
