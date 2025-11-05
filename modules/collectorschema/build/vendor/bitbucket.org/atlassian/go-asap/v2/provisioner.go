package asap

import (
	"os"
	"sync"
	"time"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/google/uuid"
)

// Provisioner is a component used to generate new ASAP tokens
// for outgoing requests.
type Provisioner interface {
	Provision() (Token, error)
}

type standardProvisioner struct {
	kid           string
	jitProvider   func() string
	ttl           time.Duration
	issuer        string
	audience      []string
	signingMethod crypto.SigningMethod
}

func (p *standardProvisioner) Provision() (Token, error) {
	var claims = jws.Claims{}
	claims.SetIssuer(p.issuer)
	claims.SetJWTID(p.jitProvider())
	claims.SetIssuedAt(time.Now())
	claims.SetExpiration(time.Now().Add(p.ttl))
	claims.SetAudience(p.audience...)
	var t = jws.NewJWT(claims, p.signingMethod)
	t.(jws.JWS).Protected().Set(ClaimKeyID, p.kid)
	return t, nil
}

// NewProvisioner generates a Provisioner implementation that sets all the
// required claims and headers for ASAP.
func NewProvisioner(kid string, ttl time.Duration, issuer string, audience []string, signingMethod crypto.SigningMethod) Provisioner {
	return &standardProvisioner{kid, func() string { return uuid.New().String() }, ttl, issuer, audience, signingMethod}
}

// NewMicrosProvisioner uses the contracted ASAP env var to populate the
// provisioner. Contract documentation: https://extranet.atlassian.com/pages/viewpage.action?pageId=2763562051
//
// Please note that as environment variables are not automatically
// available in Lambda functions you will need to use NewProvisioner instead,
// and provide values retrieved with the micros-serverless-platform-libs
// library.
func NewMicrosProvisioner(audience []string, ttl time.Duration) Provisioner {
	return NewProvisioner(os.Getenv("ASAP_KEY_ID"), ttl, os.Getenv("ASAP_ISSUER"), audience, crypto.SigningMethodRS256)
}

const minCacheLeeway = 1 * time.Second

type cacheToken struct {
	Token
	cache []byte
	lock  *sync.RWMutex
}

func (t *cacheToken) Serialize(k interface{}) ([]byte, error) {
	t.lock.RLock()
	if t.cache != nil {
		defer t.lock.RUnlock()
		return t.cache, nil
	}
	t.lock.RUnlock()
	t.lock.Lock()
	defer t.lock.Unlock()
	var value, e = t.Token.Serialize(k)
	if e != nil {
		return value, e
	}
	t.cache = value
	return value, nil
}

type cacheProvisioner struct {
	wrapped Provisioner
	cache   Token
	lock    *sync.RWMutex
}

// NewCachingProvisioner wraps any given provisioner in a time-based cache. It
// will only call the underlying provisioner when the cached token is expired.
func NewCachingProvisioner(wrapped Provisioner) Provisioner {
	return &cacheProvisioner{wrapped, nil, &sync.RWMutex{}}
}

func (p *cacheProvisioner) Provision() (Token, error) {
	p.lock.RLock()
	for p.cache == nil {
		p.lock.RUnlock()
		p.lock.Lock()
		if p.cache == nil {
			defer p.lock.Unlock()
			var t, e = p.wrapped.Provision()
			if e != nil {
				return t, e
			}
			t = &cacheToken{t, nil, &sync.RWMutex{}}
			p.cache = t
			return t, e
		}
		p.lock.Unlock()
		p.lock.RLock()
	}
	var exp, _ = p.cache.Claims().Expiration()
	var start, _ = p.cache.Claims().IssuedAt()
	if nbf, ok := p.cache.Claims().NotBefore(); ok {
		start = nbf
	}
	// buffer 5% of token lifetime
	var cacheLeeway = exp.Sub(start) * time.Duration(1) / time.Duration(20)
	if cacheLeeway < minCacheLeeway {
		cacheLeeway = minCacheLeeway
	}
	if time.Since(exp)+cacheLeeway <= 0 {
		p.lock.RUnlock()
		return p.cache, nil
	}
	p.lock.RUnlock()
	p.lock.Lock()
	defer p.lock.Unlock()
	exp, _ = p.cache.Claims().Expiration()
	if time.Since(exp)+cacheLeeway <= 0 {
		return p.cache, nil
	}
	var t, e = p.wrapped.Provision()
	if e != nil {
		return t, e
	}
	t = &cacheToken{t, nil, &sync.RWMutex{}}
	p.cache = t
	return t, e
}
