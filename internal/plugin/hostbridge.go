// Package plugin implements the CLIProxyAPI method dispatcher for the
// clinepass-quota-cliproxyapi plugin: registration, reconfiguration, and the
// Management Center quota page/endpoint.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// RawCaller performs one host callback round-trip: method name plus JSON
// request bytes in, JSON envelope bytes out.
type RawCaller func(method string, payload []byte) ([]byte, error)

// HostBridge routes the single outbound call this plugin needs
// ("host.http.do") through the injected raw caller. Callers must never
// place API keys or response bodies in log messages.
type HostBridge struct {
	call RawCaller
}

// NewHostBridge wraps the injected raw host caller. main.go wires it to the
// C host API after init; tests inject fakes.
func NewHostBridge(call RawCaller) *HostBridge {
	return &HostBridge{call: call}
}

// hostHTTPReq is the wire shape accepted by host.http.do.
type hostHTTPReq struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
}

// decodeEnvelope parses a host callback response envelope.
func decodeEnvelope(raw []byte) (pluginabi.Envelope, error) {
	var env pluginabi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return pluginabi.Envelope{}, fmt.Errorf("undecodable host response")
	}
	return env, nil
}

// Do issues an upstream HTTP request through the host callback
// "host.http.do"; transport-level failures surface as errors whose text is
// safe for logs (no keys, no response bodies). ctx is accepted for API
// symmetry with pluginapi.HostHTTPClient; the underlying C call is already
// synchronous on the calling (host) goroutine and the host enforces its own
// configured request timeout on the actual upstream fetch.
func (b *HostBridge) Do(_ context.Context, req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	if b == nil || b.call == nil {
		return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed: bridge unavailable")
	}
	payload, err := json.Marshal(hostHTTPReq{Method: req.Method, URL: req.URL, Headers: req.Headers, Body: req.Body})
	if err != nil {
		return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed: invalid request")
	}
	raw, err := b.call(pluginabi.MethodHostHTTPDo, payload)
	if err != nil {
		return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed")
	}
	env, err := decodeEnvelope(raw)
	if err != nil {
		return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed: %w", err)
	}
	if !env.OK {
		return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed")
	}
	var resp pluginapi.HTTPResponse
	if len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, &resp); err != nil {
			return pluginapi.HTTPResponse{}, fmt.Errorf("host http do failed: undecodable response body")
		}
	}
	return resp, nil
}
