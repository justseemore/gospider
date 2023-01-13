package websocket

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	_ "unsafe"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
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

//go:linkname secWebSocketAccept nhooyr.io/websocket.secWebSocketAccept
func secWebSocketAccept(string) string

//go:linkname selectSubprotocol nhooyr.io/websocket.selectSubprotocol
func selectSubprotocol(r *http.Request, subprotocols []string) string

//go:linkname acceptCompression nhooyr.io/websocket.acceptCompression
func acceptCompression(r *http.Request, w http.ResponseWriter, mode websocket.CompressionMode) (*compressionOptions, error)

//go:linkname verifyServerExtensions nhooyr.io/websocket.verifyServerExtensions
func verifyServerExtensions(copts *compressionOptions, h http.Header) (*compressionOptions, error)

type Conn struct {
	br   *bufio.Reader
	bw   *bufio.Writer
	rwc  io.ReadWriteCloser
	conn *websocket.Conn
}
type ClientOption struct {
	Subprotocols         []string        // Subprotocols lists the WebSocket subprotocols to negotiate with the server.
	CompressionMode      CompressionMode // CompressionMode controls the compression mode.
	CompressionThreshold int             // CompressionThreshold controls the minimum size of a message before compression is applied ,Defaults to 512 bytes for CompressionNoContextTakeover and 128 bytes for CompressionContextTakeover.
	CompressionOptions   *compressionOptions
}
type MessageType = websocket.MessageType
type AcceptOptions = websocket.AcceptOptions
type CompressionMode = websocket.CompressionMode

const (

	// MessageText is for UTF-8 encoded text messages like JSON.
	MessageText websocket.MessageType = websocket.MessageText
	// MessageBinary is for binary messages like protobufs.
	MessageBinary websocket.MessageType = websocket.MessageBinary

	CompressionContextTakeover   CompressionMode = websocket.CompressionContextTakeover
	CompressionDisabled          CompressionMode = websocket.CompressionDisabled
	CompressionNoContextTakeover CompressionMode = websocket.CompressionNoContextTakeover
)

func optsHook(m CompressionMode) *compressionOptions {
	return &compressionOptions{
		clientNoContextTakeover: m == CompressionNoContextTakeover,
		serverNoContextTakeover: m == CompressionNoContextTakeover,
	}
}

//go:linkname setHeaderHook nhooyr.io/websocket.(*compressionOptions).setHeader
func setHeaderHook(val *compressionOptions, h http.Header)

func SetClientHeaders(headers http.Header, option *ClientOption) error {
	secWebSocketKey, err := secWebSocketKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate Sec-WebSocket-Key: %w", err)
	}
	if option.CompressionMode != CompressionDisabled {
		option.CompressionOptions = optsHook(option.CompressionMode)
	}
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Version", "13")
	headers.Set("Sec-WebSocket-Key", secWebSocketKey)
	if len(option.Subprotocols) > 0 {
		headers.Set("Sec-WebSocket-Protocol", strings.Join(option.Subprotocols, ","))
	}
	if option.CompressionOptions != nil {
		setHeaderHook(option.CompressionOptions, headers)
	}
	return nil
}
func NewClientConn(resp *http.Response, option *ClientOption) (*Conn, error) {
	var err error
	if option.CompressionOptions, err = verifyServerExtensions(option.CompressionOptions, resp.Header); err != nil {
		return nil, err
	}
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return nil, fmt.Errorf("response body is not a io.ReadWriteCloser")
	}
	br := getBufioReader(rwc)
	bw := getBufioWriter(rwc)
	return &Conn{
		rwc: rwc,
		br:  br,
		bw:  bw,
		conn: newConn(connConfig{
			subprotocol:    resp.Header.Get("Sec-WebSocket-Protocol"),
			rwc:            rwc,
			client:         true,
			copts:          option.CompressionOptions,
			flateThreshold: option.CompressionThreshold,
			br:             br,
			bw:             bw,
		}),
	}, nil
}

func NewServerConn(w http.ResponseWriter, r *http.Request, opts *AcceptOptions) (_ *Conn, err error) {
	if opts == nil {
		opts = &AcceptOptions{}
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("http.ResponseWriter does not implement http.Hijacker")
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return nil, err
	}
	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	key := r.Header.Get("Sec-WebSocket-Key")
	w.Header().Set("Sec-WebSocket-Accept", secWebSocketAccept(key))

	subproto := selectSubprotocol(r, opts.Subprotocols)
	if subproto != "" {
		w.Header().Set("Sec-WebSocket-Protocol", subproto)
	}

	copts, err := acceptCompression(r, w, opts.CompressionMode)
	if err != nil {
		return nil, err
	}

	w.WriteHeader(http.StatusSwitchingProtocols)
	// See https://github.com/nhooyr/websocket/issues/166
	if ginWriter, ok := w.(interface {
		WriteHeaderNow()
	}); ok {
		ginWriter.WriteHeaderNow()
	}
	netConn, brw, err := hj.Hijack()
	if err != nil {
		err = fmt.Errorf("failed to hijack connection: %w", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil, err
	}
	// https://github.com/golang/go/issues/32314
	b, _ := brw.Reader.Peek(brw.Reader.Buffered())
	brw.Reader.Reset(io.MultiReader(bytes.NewReader(b), netConn))

	return &Conn{
		rwc: netConn,
		br:  brw.Reader,
		bw:  brw.Writer,
		conn: newConn(connConfig{
			subprotocol:    w.Header().Get("Sec-WebSocket-Protocol"),
			rwc:            netConn,
			client:         false,
			copts:          copts,
			flateThreshold: opts.CompressionThreshold,

			br: brw.Reader,
			bw: brw.Writer,
		}),
	}, nil
}

func (obj *Conn) SetReadLimit(n int64) {
	obj.conn.SetReadLimit(n)
}

func (obj *Conn) Conn() *websocket.Conn {
	return obj.conn
}

func (obj *Conn) Rwc() io.ReadWriteCloser {
	return obj.rwc
}
func (obj *Conn) ReadJson(ctx context.Context, v interface{}) error {
	return wsjson.Read(ctx, obj.conn, v)
}
func (obj *Conn) WriteJson(ctx context.Context, v interface{}) error {
	return wsjson.Write(ctx, obj.conn, v)
}
func (obj *Conn) Read(p []byte) (n int, err error) {
	return obj.br.Read(p)
}
func (obj *Conn) Write(p []byte) (n int, err error) {
	return obj.bw.Write(p)
}

func (obj *Conn) ReadMsg(ctx context.Context) (MessageType, []byte, error) {
	return obj.conn.Read(ctx)
}
func (obj *Conn) WriteMsg(ctx context.Context, typ MessageType, p []byte) error {
	return obj.conn.Write(ctx, typ, p)
}
func (obj *Conn) Close(reason string) error {
	defer obj.rwc.Close()
	return obj.conn.Close(websocket.StatusInternalError, reason)
}
