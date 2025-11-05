package asap

import (
	"fmt"
	"net/http"
)

type transportDecorator struct {
	wrapped     http.RoundTripper
	provisioner Provisioner
	privatekey  interface{}
}

// RoundTrip annotates the outgoing request and calls the wrapped Client.
func (c *transportDecorator) RoundTrip(r *http.Request) (*http.Response, error) {
	var token, err = c.provisioner.Provision()
	if err != nil {
		return nil, err
	}
	var headerValue, e = token.Serialize(c.privatekey)
	if e != nil {
		return nil, e
	}
	var bearer = fmt.Sprintf("Bearer %s", string(headerValue))
	r.Header.Set("Authorization", bearer)
	return c.wrapped.RoundTrip(r)
}

// NewTransportDecorator wraps a transport in order to include the asap token
// header in outgoing requests.
func NewTransportDecorator(provisioner Provisioner, pk interface{}) func(http.RoundTripper) http.RoundTripper {
	return func(c http.RoundTripper) http.RoundTripper {
		return &transportDecorator{c, provisioner, pk}
	}
}
