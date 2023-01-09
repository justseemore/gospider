package requests

import (
	"bufio"
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

//go:linkname optsHook nhooyr.io/websocket.(*CompressionMode).opts
func optsHook(val *websocket.CompressionMode) *compressionOptions

//go:linkname setHeaderHook nhooyr.io/websocket.(*compressionOptions).setHeader
func setHeaderHook(val *compressionOptions, h http.Header)

type wsOption struct {
	opts  *websocket.DialOptions
	copts *compressionOptions
}

func setWsHeaders(headers http.Header, option *RequestOption) error {
	option.WsOption.HTTPHeader = headers
	secWebSocketKey, err := secWebSocketKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate Sec-WebSocket-Key: %w", err)
	}
	var copts *compressionOptions
	if option.WsOption.CompressionMode != websocket.CompressionDisabled {
		copts = optsHook(&option.WsOption.CompressionMode)
	}
	option.WsOption.HTTPHeader.Set("Connection", "Upgrade")
	option.WsOption.HTTPHeader.Set("Upgrade", "websocket")
	option.WsOption.HTTPHeader.Set("Sec-WebSocket-Version", "13")
	option.WsOption.HTTPHeader.Set("Sec-WebSocket-Key", secWebSocketKey)
	if len(option.WsOption.Subprotocols) > 0 {
		option.WsOption.HTTPHeader.Set("Sec-WebSocket-Protocol", strings.Join(option.WsOption.Subprotocols, ","))
	}
	if copts != nil {
		setHeaderHook(copts, option.WsOption.HTTPHeader)
	}
	option.wsOption = &wsOption{
		opts:  &option.WsOption,
		copts: copts,
	}
	return nil
}

func newWsConn(resp *http.Response, option *RequestOption) (*websocket.Conn, error) {
	var err error
	if option.wsOption.copts, err = verifyServerExtensions(option.wsOption.copts, resp.Header); err != nil {
		return nil, err
	}
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return nil, fmt.Errorf("response body is not a io.ReadWriteCloser")
	}
	return newConn(connConfig{
		subprotocol:    resp.Header.Get("Sec-WebSocket-Protocol"),
		rwc:            rwc,
		client:         true,
		copts:          option.wsOption.copts,
		flateThreshold: option.wsOption.opts.CompressionThreshold,
		br:             getBufioReader(rwc),
		bw:             getBufioWriter(rwc),
	}), nil
}
