package asap

import "github.com/SermoDigital/jose/crypto"

const (
	// ClaimAlgorithm is the JWT specific encryption algorithm claim.
	ClaimAlgorithm = "alg"
	// ClaimKeyID is the JWT specified key identifier claim.
	ClaimKeyID = "kid"
	// ClaimIssuer is the JWT specified token issuer claim.
	ClaimIssuer = "iss"
	// ClaimExpiration is the JWT specified token expiration claim.
	ClaimExpiration = "exp"
	// ClaimIssuedAt is the JWT specified issued at claim.
	ClaimIssuedAt = "iat"
	// ClaimAudience is the JWT specified audience claim.
	ClaimAudience = "aud"
	// ClaimTokenID is the JWT specified JWT ID claim.
	ClaimTokenID = "jti"
	// ClaimSubject is the JWT specified subject claim.
	ClaimSubject = "sub"
	// ClaimNotBefore is the JWT specified not before claim.
	ClaimNotBefore = "nbf"
)

const (
	methodES256 = "ES256"
	methodES384 = "ES384"
	methodES512 = "ES512"

	methodRS256 = "RS256"
	methodRS384 = "RS384"
	methodRS512 = "RS512"
)

var signingMethodMap = map[string]crypto.SigningMethod{
	// ECDSA
	methodES256: crypto.SigningMethodES256,
	methodES384: crypto.SigningMethodES384,
	methodES512: crypto.SigningMethodES512,

	// RSA
	methodRS256: crypto.SigningMethodRS256,
	methodRS384: crypto.SigningMethodRS384,
	methodRS512: crypto.SigningMethodRS512,
}
