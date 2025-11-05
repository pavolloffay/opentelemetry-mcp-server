package asap

// AutorotatingKeypairProvider gets an autorotating keypair from a source.
type AutorotatingKeypairProvider interface {
	GetKeyID() (string, error)
	Fetch(keyID string) (interface{}, error)
}
