package request

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/scalyr/dataset-go/pkg/config"
)

func (ap *AuthParams) setToken(token string) {
	ap.Token = token
}

type AuthParams struct {
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
}

type TokenSetter interface {
	setToken(token string)
}

// ApiRequest represents a generic DataSet REST API request, with all its properties
type ApiRequest struct {
	httpMethod    string
	userAgent     string
	payload       []byte
	request       interface{}
	uri           string
	apiKey        string
	supportedKeys []string
	err           error
}

func NewApiRequest(requestType string, uri string) *ApiRequest {
	return &ApiRequest{httpMethod: requestType, uri: uri}
}

func (r *ApiRequest) WithWriteLog(tokens config.DataSetTokens) *ApiRequest {
	if r.apiKey != "" {
		return r
	}

	if tokens.WriteLog != "" {
		r.apiKey = tokens.WriteLog
	} else {
		r.supportedKeys = append(r.supportedKeys, "WriteLog")
	}
	return r
}

func (r *ApiRequest) WithReadLog(tokens config.DataSetTokens) *ApiRequest {
	if r.apiKey != "" {
		return r
	}

	if tokens.ReadLog != "" {
		r.apiKey = tokens.ReadLog
	} else {
		r.supportedKeys = append(r.supportedKeys, "ReadLog")
	}
	return r
}

func (r *ApiRequest) WithReadConfig(tokens config.DataSetTokens) *ApiRequest {
	if r.apiKey != "" {
		return r
	}

	if tokens.ReadConfig != "" {
		r.apiKey = tokens.ReadConfig
	} else {
		r.supportedKeys = append(r.supportedKeys, "ReadConfig")
	}
	return r
}

func (r *ApiRequest) WithWriteConfig(tokens config.DataSetTokens) *ApiRequest {
	if r.apiKey != "" {
		return r
	}

	if tokens.WriteConfig != "" {
		r.apiKey = tokens.WriteConfig
	} else {
		r.supportedKeys = append(r.supportedKeys, "WriteConfig")
	}
	return r
}

func (r *ApiRequest) jsonRequest(request TokenSetter) *ApiRequest {
	payload, err := json.Marshal(request)
	r.request = request
	if err != nil {
		r.err = err
		return r
	}
	r.payload = payload
	return r
}

func (r *ApiRequest) WithPayload(payload []byte) *ApiRequest {
	r.payload = payload
	return r
}

func (r *ApiRequest) WithUserAgent(userAgent string) *ApiRequest {
	r.userAgent = userAgent
	return r
}

func (r *ApiRequest) emptyRequest() *ApiRequest {
	return r.jsonRequest(TokenSetter(&AuthParams{}))
}

func (r *ApiRequest) HttpRequest() (*http.Request, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.payload == nil || len(r.payload) == 0 {
		r.emptyRequest()
	}

	if r.apiKey == "" && len(r.supportedKeys) > 0 {
		return nil, fmt.Errorf("no API Key Found - Supported Tokens for %v are %v", r.uri, r.supportedKeys)
	} else if r.request != nil {
		r.request.(TokenSetter).setToken(r.apiKey)
	}

	var err error
	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(r.payload); err != nil {
		r.err = fmt.Errorf("cannot compress payload: %w", err)
		return nil, r.err
	}
	if err = g.Close(); err != nil {
		r.err = fmt.Errorf("cannot finish compression: %w", err)
		return nil, r.err
	}

	req, err := http.NewRequest(r.httpMethod, r.uri, &buf)
	if err != nil {
		r.err = fmt.Errorf("failed to create NewApiRequest: %w", err)
		return nil, r.err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Encoding", "gzip")
	req.Header.Add("User-Agent", r.userAgent)

	return req, nil
}
