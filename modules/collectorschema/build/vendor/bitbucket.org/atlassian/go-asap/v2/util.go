package asap

import (
	"github.com/SermoDigital/jose/jws"
	"github.com/pkg/errors"
)

func GetKeyIDFromToken(token Token) (string, error) {
	jsonWebSignature, ok := token.(jws.JWS)
	if !ok {
		return "", errors.New("Token is not a JSON web signature")
	}

	header := jsonWebSignature.Protected()
	if header == nil {
		return "", errors.New("Protected header is nil")
	}

	rawKeyID := header.Get(ClaimKeyID)
	if rawKeyID == nil {
		return "", errors.New("Missing the kid header")
	}

	keyID, ok := rawKeyID.(string)
	if !ok {
		return "", errors.New("kid header value is not a string")
	}

	return keyID, nil
}
