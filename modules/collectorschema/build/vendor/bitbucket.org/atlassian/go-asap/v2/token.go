package asap

import (
	"fmt"

	"github.com/SermoDigital/jose/jws"
	"github.com/SermoDigital/jose/jwt"
)

// Token is an interface that abstracts an underlying JWT implementation.
// It can be decomposed to a byte slice for transmission and provides
// access to the JWT claims.
type Token interface {
	jwt.JWT
}

// CacheableKeyer is a trait for cacheable entities
type CacheableKeyer interface {
	CacheKey() string
}

// cacheableToken extends CacheableKeyer to implement CacheableKeyer
type cacheableToken struct {
	Token
	jws.JWS  // To support conversion, used in validator to access protected headers
	cacheKey string
}

func (t *cacheableToken) CacheKey() string {
	return t.cacheKey
}

// ParseToken converts a string header value, minus the "Bearer " bit, into
// a Token.
func ParseToken(raw string) (Token, error) {
	t, err := jws.ParseJWT([]byte(raw))
	if err != nil {
		return nil, err
	}

	j, ok := t.(jws.JWS)
	if !ok {
		return nil, fmt.Errorf("invalid token: %v", t)
	}

	return &cacheableToken{t, j, raw}, nil
}
