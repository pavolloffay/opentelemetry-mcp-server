package dialer

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

type Dialer interface {
	Dial(network, addr string) (c net.Conn, err error)
}

type ContextDialer interface {
	Dialer
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

var (
	ErrUnsupportedScheme  = errors.New("requested scheme is unsupported")
	ErrUnsupportedNetwork = errors.New("requested network is unsupported")
)

type proxyConnectDialer struct {
	url     *url.URL
	forward Dialer
	options *proxyConnectDialerOptions
}

func (p *proxyConnectDialer) Dial(network, addr string) (net.Conn, error) {
	return p.DialContext(context.Background(), network, addr)
}

func (p *proxyConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, ErrUnsupportedNetwork
	}

	var conn net.Conn
	var err error
	if contextForward, ok := p.forward.(ContextDialer); ok {
		conn, err = contextForward.DialContext(ctx, network, p.url.Host)
	} else {
		conn, err = p.forward.Dial(network, p.url.Host)
	}
	if err != nil {
		return nil, err
	}

	if p.url.Scheme == "https" {
		conn = tls.Client(conn, p.options.tls)
	}

	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: p.options.proxyConnect.Clone(),
		Close:  false,
	}
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req = req.WithContext(ctx)

	// Add a (basic) proxy-auth header if username and password are non-empty.
	if p.options.username != "" && p.options.password != "" {
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(p.options.username+":"+p.options.password))
		req.Header.Add("Proxy-Authorization", basicAuth)
	}

	resp, err := p.roundTrip(conn, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("dialer recieved non-200 response for CONNECT request: %d", resp.StatusCode)
	}
	return conn, nil
}

func (p *proxyConnectDialer) roundTrip(conn net.Conn, req *http.Request) (*http.Response, error) {
	if err := req.WriteProxy(conn); err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(conn), req)
}

// proxyConnectDialerOptions are optional options that can be specified when creating a ProxyConnectDialer.
type proxyConnectDialerOptions struct {
	tls          *tls.Config
	proxyConnect http.Header
	username     string
	password     string
}

type Option func(o *proxyConnectDialerOptions) error

// WithTLS specifies proxy-specific TLS config.
// At a minimum ServerName or InsecureSkipVerify must be set.
func WithTLS(cfg *tls.Config) Option {
	return func(o *proxyConnectDialerOptions) error {
		o.tls = cfg
		return nil
	}
}

// WithProxyConnect specifies the optional set of http.Header values for the CONNECT request.
func WithProxyConnectHeaders(headers http.Header) Option {
	return func(o *proxyConnectDialerOptions) error {
		o.proxyConnect = headers
		return nil
	}
}

// WithProxyAuthorization sets the Proxy-Autorization header to a basic value constructed out of the specified username and password values.
// Both values must be non-empty strings in the Option.
// If specified directly in options, and as part of the url, the url value is used.
// If the url value exists, but either the username, or password is empty, no header is sent.
func WithProxyAuthorization(username, password string) Option {
	return func(o *proxyConnectDialerOptions) error {
		if username == "" {
			return fmt.Errorf("provided username is empty")
		}
		if password == "" {
			return fmt.Errorf("provided password is empty")
		}
		o.username = username
		o.password = password
		return nil
	}
}

// NewProxyConnectDialer creates an HTTP proxy dialer that uses a CONNECT request for the initial request to the proxy.
// The url's scheme must be set to http, or https.
// The passed Dialer is used as a forwarding dialer to make network requests.
// If the forwarding Dialer is context-aware, the DialContext method will be used.
// The dialer may be configured with optional arguments.
func NewProxyConnectDialer(u *url.URL, forward Dialer, opts ...Option) (ContextDialer, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedScheme, u.Scheme)
	}
	options := &proxyConnectDialerOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, fmt.Errorf("invalid option: %w", err)
		}
	}

	// Set Username and password to those associated with the url if present
	if u.User != nil {
		options.username = u.User.Username()
		options.password, _ = u.User.Password()
	}

	// Ensure that both username and password are set
	if options.username == "" || options.password == "" {
		options.username = ""
		options.password = ""
	}

	// If it's an https connection, and there's no tls.Config we need to determine the ServerName
	if u.Scheme == "https" && options.tls == nil {
		serverName, _, err := net.SplitHostPort(u.Host)
		if err != nil && err.Error() == "missing port in address" {
			serverName = u.Host
		}
		if serverName == "" {
			return nil, fmt.Errorf("unable to create tls.Config: could not detect ServerName from url: %w", err)
		}
		options.tls = &tls.Config{
			ServerName: serverName,
		}
	}

	return &proxyConnectDialer{
		url:     u,
		forward: forward,
		options: options,
	}, nil
}
