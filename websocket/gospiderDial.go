package websocket

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GospiderOption struct {
	opts  *DialOptions
	copts *compressionOptions
}

func GospiderNewOption(headers http.Header) (*GospiderOption, error) {
	opts := &DialOptions{HTTPHeader: headers}
	secWebSocketKey, err := secWebSocketKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Sec-WebSocket-Key: %w", err)
	}
	var copts *compressionOptions
	if opts.CompressionMode != CompressionDisabled {
		copts = opts.CompressionMode.opts()
	}
	opts.HTTPHeader.Set("Connection", "Upgrade")
	opts.HTTPHeader.Set("Upgrade", "websocket")
	opts.HTTPHeader.Set("Sec-WebSocket-Version", "13")
	opts.HTTPHeader.Set("Sec-WebSocket-Key", secWebSocketKey)
	if len(opts.Subprotocols) > 0 {
		opts.HTTPHeader.Set("Sec-WebSocket-Protocol", strings.Join(opts.Subprotocols, ","))
	}
	if copts != nil {
		copts.setHeader(opts.HTTPHeader)
	}
	return &GospiderOption{
		opts:  opts,
		copts: copts,
	}, nil
}

func GospiderNewConn(resp *http.Response, option *GospiderOption) (*Conn, error) {
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return nil, fmt.Errorf("response body is not a io.ReadWriteCloser")
	}
	return newConn(connConfig{
		subprotocol:    resp.Header.Get("Sec-WebSocket-Protocol"),
		rwc:            rwc,
		client:         true,
		copts:          option.copts,
		flateThreshold: option.opts.CompressionThreshold,
		br:             getBufioReader(rwc),
		bw:             getBufioWriter(rwc),
	}), nil
}
