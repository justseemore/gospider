package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
	_ "unsafe"

	"gitee.com/baixudong/gospider/websocket"
	utls "github.com/refraction-networking/utls"
)

//go:linkname readRequest net/http.readRequest
func readRequest(b *bufio.Reader) (*http.Request, error)

type ProxyOption struct {
	init     bool
	http2    bool
	host     string
	schema   string
	method   string
	port     string
	isWs     bool
	tls      bool
	wsOption websocket.Option
	ctx      context.Context
	cnl      context.CancelFunc
}
type ProxyConn struct {
	client bool
	conn   net.Conn
	reader *bufio.Reader
	option *ProxyOption
}

func newProxyCon(preCtx context.Context, conn net.Conn, reader *bufio.Reader, option ProxyOption, client bool) *ProxyConn {
	option.ctx, option.cnl = context.WithCancel(preCtx)

	return &ProxyConn{conn: conn, reader: reader, option: &option, client: client}
}

type connectionStater interface {
	ConnectionState() tls.ConnectionState
}
type connectionStater2 interface {
	ConnectionState() utls.ConnectionState
}

func (obj *ProxyConn) ConnectionState() tls.ConnectionState {
	tlsConn, ok := obj.conn.(connectionStater)
	if ok {
		return tlsConn.ConnectionState()
	} else {
		tlsConn2, ok := obj.conn.(connectionStater2)
		connstate := tlsConn2.ConnectionState()
		if ok {
			return tls.ConnectionState{
				Version:                     connstate.Version,
				HandshakeComplete:           connstate.HandshakeComplete,
				DidResume:                   connstate.DidResume,
				CipherSuite:                 connstate.CipherSuite,
				NegotiatedProtocol:          connstate.NegotiatedProtocol,
				NegotiatedProtocolIsMutual:  connstate.NegotiatedProtocolIsMutual,
				ServerName:                  connstate.ServerName,
				PeerCertificates:            connstate.PeerCertificates,
				VerifiedChains:              connstate.VerifiedChains,
				SignedCertificateTimestamps: connstate.SignedCertificateTimestamps,
				OCSPResponse:                connstate.OCSPResponse,
				TLSUnique:                   connstate.TLSUnique,
			}
		}
	}
	return tls.ConnectionState{}
}
func (obj *ProxyConn) Read(b []byte) (int, error) {
	n, err := obj.reader.Read(b)
	if err != nil {
		obj.Close()
	}
	return n, err
}
func (obj *ProxyConn) Write(b []byte) (int, error) {
	n, err := obj.conn.Write(b)
	if err != nil {
		obj.Close()
	}
	return n, err
}
func (obj *ProxyConn) Close() error {
	defer obj.option.cnl()
	return obj.conn.Close()
}
func (obj *ProxyConn) LocalAddr() net.Addr {
	return obj.conn.LocalAddr()
}
func (obj *ProxyConn) RemoteAddr() net.Addr {
	return obj.conn.RemoteAddr()
}
func (obj *ProxyConn) SetDeadline(t time.Time) error {
	return obj.conn.SetDeadline(t)
}
func (obj *ProxyConn) SetReadDeadline(t time.Time) error {
	return obj.conn.SetReadDeadline(t)
}
func (obj *ProxyConn) SetWriteDeadline(t time.Time) error {
	return obj.conn.SetWriteDeadline(t)
}
func (obj *ProxyConn) readResponse(req *http.Request) (*http.Response, error) {
	response, err := http.ReadResponse(obj.reader, req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == 101 && response.Header.Get("Upgrade") == "websocket" {
		obj.option.isWs = true
		obj.option.wsOption = websocket.GetHeaderOption(response.Header, false)
	}
	return response, err
}
func (obj *ProxyConn) readRequest(readRequestCallBack func(*http.Request)) (*http.Request, error) {
	clientReq, err := readRequest(obj.reader)
	if err != nil {
		return clientReq, err
	}
	if readRequestCallBack != nil {
		readRequestCallBack(clientReq)
	}
	obj.option.init = true
	if clientReq.Header.Get("Upgrade") == "websocket" {
		obj.option.isWs = true
		obj.option.wsOption = websocket.GetHeaderOption(clientReq.Header, true)
	}

	hostName := clientReq.URL.Hostname()
	obj.option.method = clientReq.Method
	if obj.option.host == "" {
		if headHost := clientReq.Header.Get("Host"); headHost != "" {
			obj.option.host = headHost
		} else if clientReq.Host != "" {
			obj.option.host = clientReq.Host
		} else if hostName != "" {
			obj.option.host = hostName
		}
	}
	if hostName == "" {
		if clientReq.Host != "" {
			clientReq.URL.Host = clientReq.Host
		} else {
			clientReq.URL.Host = obj.option.host
		}
	}

	if hostName := clientReq.URL.Hostname(); hostName == "" {
		clientReq.URL.Host = clientReq.Host
	} else if clientReq.Host == "" {
		clientReq.Host = hostName
	}
	if obj.option.schema == "" {
		if clientReq.URL.Scheme == "" {
			if clientReq.Method == http.MethodConnect {
				obj.option.schema = "https"
			} else {
				obj.option.schema = "http"
			}
			clientReq.URL.Scheme = obj.option.schema
		} else {
			obj.option.schema = clientReq.URL.Scheme
		}
	} else if clientReq.URL.Scheme == "" {
		clientReq.URL.Scheme = obj.option.schema
	}
	if obj.option.port == "" {
		if clientReq.URL.Port() == "" {
			if obj.option.schema == "https" {
				obj.option.port = "443"
			} else {
				obj.option.port = "80"
			}
			clientReq.URL.Host = clientReq.URL.Hostname() + ":" + obj.option.port
		} else {
			obj.option.port = clientReq.URL.Port()
		}
	} else if clientReq.URL.Port() == "" {
		clientReq.URL.Host = clientReq.URL.Hostname() + ":" + obj.option.port
	}
	return clientReq, err
}
