# proxy-connect-dialer-go

proxy-connect-dialer-go is a pure go library with no additional imports that provides a custom dialer which uses the HTTP `CONNECT` method to connect through a proxy.

## Usage

The Dialer may be used directly:

```go
proxyURL, err := url.Parse("https://proxy.internal:8080")
if err != nil {
    return err
}

dialer, err := NewConnectDialer(proxyURL, &net.Dialer{})
if err != nil {
    return err
}

// conn is a ready to use connection to the destination server through the specified proxy.
conn, err := dialer.Dial(network, address)
if err != nil {
    return err
}
```

It can also be used in place of any Dialer, or ContextDialer:

```go
proxyURL, err := url.Parse("https://proxy.internal:8080")
if err != nil {
    return err
}

dialer, err := NewConnectDialer(proxyURL, &net.Dialer{})
if err != nil {
    return err
}

// httpClient uses the dialer's DialContext method to connect through the proxy.
httpClient := &http.Client{
    Transport: &http.Transport{
        DialContext: dialer.DialContext,
    },
}
```

Or it can be used with [golang.org/x/net/proxy](https://pkg.go.dev/golang.org/x/net/proxy):

```go
import "golang.org/x/net/proxy"

// Register a dialer with no options
proxy.RegisterDialer("http", NewConnectDialer)

// Register a dialer with known options
f := func(u *url.URL, d Dialer) (Dialer, error) {
    return NewConnectDialer(u, d, WithTLS(&tls.Config{
        InsecureSkipVerify: true,
    }))
}
proxy.RegisterDialer("https", f)

// Determine the correct dialer to use with proxy.FromURL
proxyURL, err := url.Parse("https://proxy.internal:8080")
if err != nil {
    return err
}
dialer, err := proxy.FromURL(proxyURL, &net.Dialer{})
if err != nil {
    return err
}
```
