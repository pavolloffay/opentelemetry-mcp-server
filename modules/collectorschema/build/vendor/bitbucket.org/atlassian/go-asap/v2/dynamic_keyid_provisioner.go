package asap

import (
	"time"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/google/uuid"
)

type DynamicKeyIDProvisioner struct {
	jitProvider   func() string
	ttl           time.Duration
	issuer        string
	audience      []string
	signingMethod crypto.SigningMethod
	provider      AutorotatingKeypairProvider
}

func NewDynamicKeyIDProvisioner(ttl time.Duration, issuer string, audience []string, signingMethod crypto.SigningMethod, provider AutorotatingKeypairProvider) Provisioner {
	return &DynamicKeyIDProvisioner{func() string { return uuid.New().String() }, ttl, issuer, audience, signingMethod, provider}
}

func (p *DynamicKeyIDProvisioner) Provision() (Token, error) {
	var claims = jws.Claims{}
	claims.SetIssuer(p.issuer)
	claims.SetJWTID(p.jitProvider())
	claims.SetIssuedAt(time.Now())
	claims.SetExpiration(time.Now().Add(p.ttl))
	claims.SetAudience(p.audience...)
	var t = jws.NewJWT(claims, p.signingMethod)
	keyID, err := p.provider.GetKeyID()
	if err != nil {
		return nil, err
	}
	t.(jws.JWS).Protected().Set(ClaimKeyID, keyID)
	return t, nil
}
