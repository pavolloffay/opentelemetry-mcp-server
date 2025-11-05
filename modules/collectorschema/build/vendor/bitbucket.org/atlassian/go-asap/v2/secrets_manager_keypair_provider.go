package asap

import (
	"encoding/json"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
)

const (
	minCacheTTL     = 1 * time.Second // set this low to test a cache refresh after a keypair rotation
	maxCacheTTL     = 2 * time.Hour
	defaultCacheTTL = 1 * time.Hour
)

var (
	IAMRoleArnRegex = regexp.MustCompile(`^arn:(aws|aws-us-gov):iam::([0-9]{12}):role\/([^\/]+)$`)
)

type SecretsManagerAPI interface {
	GetSecretValue(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
}

type SecretsManagerKeypairProvider struct {
	client          SecretsManagerAPI
	lock            sync.RWMutex
	privateKeyARN   string
	cacheTTL        time.Duration
	latestKeyID     string
	privateKeys     map[string]interface{}
	lastUpdatedTime time.Time
}

func NewSecretsManagerKeypairProvider(privateKeyARN, region, role, cacheTTL string) (*SecretsManagerKeypairProvider, error) {
	err := validateRegionAndRole(region, role)
	if err != nil {
		return nil, errors.Wrapf(err, "Invalid input; region: %s; role: %s", region, role)
	}

	cacheTTLDuration, err := parseCacheTTL(cacheTTL)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to validate the cacheTTL")
	}

	secretsManagerClient := buildSecretsManagerClient(region, role)
	provider := &SecretsManagerKeypairProvider{
		client:          secretsManagerClient,
		privateKeyARN:   privateKeyARN,
		cacheTTL:        cacheTTLDuration,
		latestKeyID:     "",
		privateKeys:     map[string]interface{}{},
		lastUpdatedTime: time.Time{},
	}

	return provider, nil
}

func (p *SecretsManagerKeypairProvider) GetKeyID() (string, error) {
	p.lock.RLock()
	latestKeyID := p.latestKeyID
	privateKeys := p.privateKeys
	lastUpdatedTime := p.lastUpdatedTime
	p.lock.RUnlock()

	if latestKeyID == "" {
		return p.refreshCache()
	}

	p.lock.RLock()
	_, keyIDExists := privateKeys[latestKeyID]
	p.lock.RUnlock()

	isTimeToRefresh := shouldRefreshCache(lastUpdatedTime, p.cacheTTL)

	if keyIDExists && !isTimeToRefresh {
		return latestKeyID, nil
	}

	return p.refreshCache()
}

func (p *SecretsManagerKeypairProvider) Fetch(keyID string) (interface{}, error) {
	p.lock.RLock()
	privateKey, exists := p.privateKeys[keyID]
	p.lock.RUnlock()

	if !exists {
		return nil, errors.Errorf("Failed to get the private key from the map; keyID: %s", keyID)
	}

	return privateKey, nil
}

func (p *SecretsManagerKeypairProvider) refreshCache() (string, error) {
	secretValue, err := p.getSecretValue()
	if err != nil {
		return "", errors.Wrapf(err, "Failed to get the secret value from Secrets Manager; privateKeyARN: %s", p.privateKeyARN)
	}

	keyID, exists := secretValue["ASAP_KEY_ID"]
	if !exists {
		return "", errors.Errorf("Failed to get the keyID from the secret value; privateKeyARN: %s", p.privateKeyARN)
	}

	privateKey, exists := secretValue["ASAP_PRIVATE_KEY"]
	if !exists {
		return "", errors.Errorf("Failed to get the private key from the secret value; privateKeyARN: %s", p.privateKeyARN)
	}
	privateKeyPem, err := NewPrivateKey([]byte(privateKey))
	if err != nil {
		return "", errors.Wrap(err, "Failed to convert the private key into PEM format")
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	p.latestKeyID = keyID
	p.privateKeys[p.latestKeyID] = privateKeyPem
	p.lastUpdatedTime = time.Now()

	return p.latestKeyID, nil
}

func (p *SecretsManagerKeypairProvider) getSecretValue() (map[string]string, error) {
	getSecretValueInput := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(p.privateKeyARN),
	}

	result, err := p.client.GetSecretValue(getSecretValueInput)
	if err != nil {
		return nil, handleClientError(err, "Failed to get the secret value", "privateKeyARN: "+p.privateKeyARN)
	}

	if result.SecretString == nil {
		return nil, errors.Errorf("Secret string is nil; privateKeyARN: %s", p.privateKeyARN)
	}

	var secretValue map[string]string
	if err := json.Unmarshal([]byte(*result.SecretString), &secretValue); err != nil {
		return nil, errors.Wrapf(err, "Failed to parse the secret string; privateKeyARN: %s", p.privateKeyARN)
	}

	return secretValue, nil
}

func validateRegionAndRole(region string, role string) error {
	if region == "" {
		return errors.New("The region is empty")
	}

	if role == "" {
		return nil
	}

	if !IAMRoleArnRegex.MatchString(role) {
		return errors.Errorf("The role is not a valid AWS IAM ARN; role: %s", role)
	}

	return nil
}

func parseCacheTTL(cacheTTL string) (time.Duration, error) {
	if cacheTTL == "" {
		return defaultCacheTTL, nil
	}

	cacheTTLDuration, err := time.ParseDuration(cacheTTL)
	if err != nil {
		return 0, errors.Wrapf(err, "Failed to parse the duration; cacheTTL: %s", cacheTTL)
	}

	if cacheTTLDuration < minCacheTTL || cacheTTLDuration > maxCacheTTL {
		return 0, errors.Errorf("Invalid cacheTTL; it must be between 1 second and 2 hours (inclusive); cacheTTL: %s", cacheTTLDuration)
	}

	return cacheTTLDuration, nil
}

func buildSecretsManagerClient(region string, role string) SecretsManagerAPI {
	useFIPSEndpoint := endpoints.FIPSEndpointStateDisabled
	if os.Getenv("AWS_USE_FIPS_ENDPOINT") == "true" {
		useFIPSEndpoint = endpoints.FIPSEndpointStateEnabled
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigDisable,
		Config: aws.Config{
			UseFIPSEndpoint: useFIPSEndpoint,
			Region:          aws.String(region),
		},
	}))

	conf := &aws.Config{}
	if role != "" {
		conf.Credentials = stscreds.NewCredentials(sess, role)
	}

	client := secretsmanager.New(sess, conf)

	return client
}

func handleClientError(err error, msg string, input interface{}) error {
	awsErrCode := ""
	errMessage := err.Error()

	if aerr, ok := err.(awserr.Error); ok {
		awsErrCode = aerr.Code()
		errMessage = aerr.Message()
	}

	return errors.Wrapf(err, "%s; awsErrorCode: %s; errorMessage: %s; input: %v", msg, awsErrCode, errMessage, input)
}

func shouldRefreshCache(lastUpdatedTime time.Time, cacheTTL time.Duration) bool {
	currentTime := time.Now()
	cacheRefreshTime := lastUpdatedTime.Add(cacheTTL)
	return currentTime.After(cacheRefreshTime)
}
