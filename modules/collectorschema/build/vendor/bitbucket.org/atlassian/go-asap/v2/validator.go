package asap

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/SermoDigital/jose/crypto"
	"github.com/SermoDigital/jose/jws"
	"github.com/SermoDigital/jose/jwt"
)

const (
	minLeeway     = time.Second
	defaultLeeway = time.Second
	maxLeeway     = 30 * time.Second
)

// Validator is a component used to validate incoming ASAP tokens.
// Example implementations would include one that ensure all
// required claims are present for ASAP and one that verifies
// individual claims required by a service.
type Validator interface {
	Validate(token Token) error
}

type validatorFunc func(Token) error

func (v validatorFunc) Validate(t Token) error {
	return v(t)
}

// ValidatorChain is a collection of Validator implementations that should
// be run against a given token.
type validatorChain struct {
	validators []Validator
}

// NewValidatorChain composes all given validators into one.
func NewValidatorChain(vs ...Validator) Validator {
	return &validatorChain{vs}
}

// Validate iterates through the contained Validator implementations and
// executes them in the order provided. It exits on the first error encountered.
func (v *validatorChain) Validate(t Token) error {
	for _, validator := range v.validators {
		var e = validator.Validate(t)
		if e != nil {
			return e
		}
	}
	return nil
}

type requiredClaimsValidator struct {
	claimNames []string
}

// NewRequiredClaimsValidator takes a set of names and returns a Validator
// that fails if any of the given claims are not present.
func NewRequiredClaimsValidator(names ...string) Validator {
	return &requiredClaimsValidator{names}
}

func (v *requiredClaimsValidator) Validate(t Token) error {
	for _, name := range v.claimNames {
		if !t.Claims().Has(name) {
			return fmt.Errorf("Missing claim %s", name)
		}
	}
	return nil
}

type allowedStringsValidator struct {
	claimName      string
	allowedStrings map[string]struct{} // (0 bytes value) set like structure for faster lookups
}

// NewAllowedStringsValidator takes a claim name and set of allowed string
// values. If a value is given for the claim that is not in the list then the
// Validator will return an error.
func NewAllowedStringsValidator(name string, values ...string) Validator {
	var allowedStrings = make(map[string]struct{}, len(values))
	for _, v := range values {
		allowedStrings[v] = struct{}{}
	}
	return &allowedStringsValidator{name, allowedStrings}
}

func (v *allowedStringsValidator) Validate(t Token) error {
	var claimValue, ok = t.Claims().Get(v.claimName).(string)
	if !ok {
		return fmt.Errorf("Claim %s did not contain a string value", v.claimName)
	}
	if _, ok := v.allowedStrings[claimValue]; ok {
		return nil
	}
	return fmt.Errorf("Claim %s:%s did not match an approved value", v.claimName, claimValue)
}

type allowedAudienceValidator struct {
	allowedStrings []string
}

// NewAllowedAudienceValidator takes a set of allowed audience values. If a
// at least one of the given audience values matches on of the approved then
// the validator will return no error.
func NewAllowedAudienceValidator(values ...string) Validator {
	return &allowedAudienceValidator{values}
}

func (v *allowedAudienceValidator) Validate(t Token) error {
	var audienceValues, _ = t.Claims().Audience()
	for _, allowed := range v.allowedStrings {
		for _, given := range audienceValues {
			if given == allowed {
				return nil
			}
		}
	}
	return fmt.Errorf("No given audience values %s matched an approved value %s", audienceValues, v.allowedStrings)
}

var kidRegex = regexp.MustCompile(`^[\w.\-\+/]*$`)

func kidValidator(t Token) error {
	var kid, ok = t.(jws.JWS).Protected().Get(ClaimKeyID).(string)
	if !ok {
		return fmt.Errorf("missing or invalid kid")
	}
	var issuer, _ = t.Claims().Issuer()
	if !strings.HasPrefix(kid, issuer+"/") {
		return fmt.Errorf("the KeyID %s does not start with the issuer name %s", kid, issuer)
	}

	for _, s := range strings.Split(kid, "/") {
		if s == "." || s == ".." {
			return fmt.Errorf("the KeyID %s contains invalid segments (. or ..)", kid)
		}
	}

	if !kidRegex.MatchString(kid) {
		return fmt.Errorf("the KeyID %s does not match the approved regexp", kid)
	}

	return nil
}

// KidValidator enforces the ASAP formatting rules for the kid header.
var KidValidator = validatorFunc(kidValidator)

func retrieveAndValidateAlgorithm(t Token) (crypto.SigningMethod, error) {
	given, ok := t.(jws.JWS).Protected().Get(ClaimAlgorithm).(string)
	if !ok {
		return nil, fmt.Errorf("Missing algorithm")
	}
	signingMethod, ok := signingMethodMap[given]
	if !ok {
		return nil, fmt.Errorf("Unsupported algorithm: %s", given)
	}
	return signingMethod, nil
}

func algorithmValidator(t Token) error {
	_, err := retrieveAndValidateAlgorithm(t)
	return err
}

// AlgorithmValidator enforces the ASAP rules around allowed crypto algorithms.
var AlgorithmValidator = validatorFunc(algorithmValidator)

func expirationValidator(t Token) error {
	var issuedAt, ok = t.Claims().IssuedAt()
	if !ok {
		return fmt.Errorf("Missing or invalid issued at time")
	}
	var expiration, _ = t.Claims().Expiration()

	if issuedAt.Add(time.Hour).Before(expiration) {
		return fmt.Errorf("IssuedAt time %v is more than an hour before Expiration time %v", issuedAt, expiration)
	}

	return nil
}

// ExpirationValidator enforces the ASAP rules around token lifetime. Specifically,
// it rejects all tokens that are provisioned for greater than one hour.
var ExpirationValidator = validatorFunc(expirationValidator)

// DefaultValidator applies the basic ASAP validation for a JWT by validating
// the kid and ensuring that all required ASAP claims are present. This should
// be combined with your own validation rules using NewValidatorChain.
var DefaultValidator = NewValidatorChain(
	KidValidator,
	AlgorithmValidator,
	ExpirationValidator,
	NewRequiredClaimsValidator(ClaimIssuer, ClaimExpiration, ClaimIssuedAt, ClaimAudience, ClaimTokenID),
)

type signatureValidator struct {
	fetcher KeyFetcher
	leeway  time.Duration
}

// SignatureValidatorOption is a functional option type for setting custom values
// for fields on SignatureValidators
type SignatureValidatorOption func(*signatureValidator)

// WithLeeway is a SignatureValidatorOption that takes in a time.Duration between 1s and 30s
// which can be used to set the leeway for NBF and EXP to account for server clock skew
func WithLeeway(l time.Duration) SignatureValidatorOption {
	return func(sv *signatureValidator) {
		if l < minLeeway {
			l = minLeeway
		} else if l > maxLeeway {
			l = maxLeeway
		}
		sv.leeway = l
	}
}

// NewSignatureValidator enforces that tokens are signed by the key they claim
// to be.
func NewSignatureValidator(fetcher KeyFetcher, options ...SignatureValidatorOption) Validator {
	validator := &signatureValidator{fetcher, defaultLeeway}
	for _, opt := range options {
		opt(validator)
	}
	return validator
}

func (v *signatureValidator) Validate(t Token) error {
	kid, ok := t.(jws.JWS).Protected().Get(ClaimKeyID).(string)
	if !ok {
		return fmt.Errorf("Missing or invalid key id")
	}

	signingMethod, err := retrieveAndValidateAlgorithm(t)
	if err != nil {
		return err
	}
	var k, e = v.fetcher.Fetch(kid)
	if e != nil {
		return e
	}
	return t.Validate(k, signingMethod, &jwt.Validator{EXP: v.leeway, NBF: v.leeway})
}
