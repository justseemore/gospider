package requests

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	_ "unsafe"

	"nhooyr.io/websocket"
)

type compressionOptions struct {
	clientNoContextTakeover bool
	serverNoContextTakeover bool
}

type connConfig struct {
	subprotocol    string
	rwc            io.ReadWriteCloser
	client         bool
	copts          *compressionOptions
	flateThreshold int

	br *bufio.Reader
	bw *bufio.Writer
}

//go:linkname newConn nhooyr.io/websocket.newConn
func newConn(cfg connConfig) *websocket.Conn

//go:linkname getBufioReader nhooyr.io/websocket.getBufioReader
func getBufioReader(r io.Reader) *bufio.Reader

//go:linkname getBufioWriter nhooyr.io/websocket.getBufioWriter
func getBufioWriter(w io.Writer) *bufio.Writer

//go:linkname secWebSocketKey nhooyr.io/websocket.secWebSocketKey
func secWebSocketKey(rr io.Reader) (string, error)

//go:linkname verifyServerExtensions nhooyr.io/websocket.verifyServerExtensions
func verifyServerExtensions(copts *compressionOptions, h http.Header) (*compressionOptions, error)

func optsHook(m websocket.CompressionMode) *compressionOptions {
	return &compressionOptions{
		clientNoContextTakeover: m == websocket.CompressionNoContextTakeover,
		serverNoContextTakeover: m == websocket.CompressionNoContextTakeover,
	}
}

//go:linkname setHeaderHook nhooyr.io/websocket.(*compressionOptions).setHeader
func setHeaderHook(val *compressionOptions, h http.Header)

type WsOption struct {
	Subprotocols         []string                  // Subprotocols lists the WebSocket subprotocols to negotiate with the server.
	CompressionMode      websocket.CompressionMode // CompressionMode controls the compression mode.
	CompressionThreshold int                       // CompressionThreshold controls the minimum size of a message before compression is applied ,Defaults to 512 bytes for CompressionNoContextTakeover and 128 bytes for CompressionContextTakeover.
}

func setWsHeaders(headers http.Header, option *RequestOption) error {
	secWebSocketKey, err := secWebSocketKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate Sec-WebSocket-Key: %w", err)
	}
	var copts *compressionOptions
	if option.WsOption.CompressionMode != websocket.CompressionDisabled {
		copts = optsHook(option.WsOption.CompressionMode)
	}
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Version", "13")
	headers.Set("Sec-WebSocket-Key", secWebSocketKey)
	if len(option.WsOption.Subprotocols) > 0 {
		headers.Set("Sec-WebSocket-Protocol", strings.Join(option.WsOption.Subprotocols, ","))
	}
	if copts != nil {
		setHeaderHook(copts, headers)
	}
	option.compressionOptions = copts
	return nil
}

func newWsConn(resp *http.Response, option *RequestOption) (*websocket.Conn, io.ReadWriteCloser, error) {
	var err error
	if option.compressionOptions, err = verifyServerExtensions(option.compressionOptions, resp.Header); err != nil {
		return nil, nil, err
	}
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return nil, nil, fmt.Errorf("response body is not a io.ReadWriteCloser")
	}
	return newConn(connConfig{
		subprotocol:    resp.Header.Get("Sec-WebSocket-Protocol"),
		rwc:            rwc,
		client:         true,
		copts:          option.compressionOptions,
		flateThreshold: option.WsOption.CompressionThreshold,
		br:             getBufioReader(rwc),
		bw:             getBufioWriter(rwc),
	}), rwc, nil
}

func (obj *Response) WsRead(ctx context.Context) (websocket.MessageType, []byte, error) {
	for {
		msgType, msgCon, msgErr := obj.webSocketConn.Read(ctx)
		if !errors.Is(msgErr, io.EOF) {
			return msgType, msgCon, msgErr
		}
	}
}
func (obj *Response) WsWrite(ctx context.Context, typ websocket.MessageType, p []byte) error {
	return obj.webSocketConn.Write(ctx, typ, p)
}
