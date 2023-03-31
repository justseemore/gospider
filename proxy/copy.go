package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	_ "unsafe"

	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/tools"
	"gitee.com/baixudong/gospider/websocket"
	"golang.org/x/net/http2"
)

func (obj *Client) wsSend(ctx context.Context, wsClient *websocket.Conn, wsServer *websocket.Conn) (err error) {
	defer wsServer.Close("close")
	defer wsClient.Close("close")
	var msgType websocket.MessageType
	var msgData []byte
	for {
		if msgType, msgData, err = wsClient.Recv(ctx); err != nil {
			return
		}
		if obj.WsCallBack != nil {
			obj.WsCallBack(msgType, msgData, "send")
		}
		if err = wsServer.Send(ctx, msgType, msgData); err != nil {
			return
		}
	}
}
func (obj *Client) wsRecv(ctx context.Context, wsClient *websocket.Conn, wsServer *websocket.Conn) (err error) {
	defer wsServer.Close("close")
	defer wsClient.Close("close")
	var msgType websocket.MessageType
	var msgData []byte
	for {
		if msgType, msgData, err = wsServer.Recv(ctx); err != nil {
			return
		}
		if obj.WsCallBack != nil {
			obj.WsCallBack(msgType, msgData, "recv")
		}
		if err = wsClient.Send(ctx, msgType, msgData); err != nil {
			return
		}
	}
}

type Flusher interface {
	FlushError() error
}

func (obj *Client) http21Copy(preCtx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	var lock sync.Mutex
	ctx, cnl := context.WithCancel(preCtx)
	defer cnl()
	var startSize atomic.Int64
	var endSize atomic.Int64
	go obj.http2Server.ServeConn(client, &http2.ServeConnOpts{
		Context: ctx,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startSize.Add(1)
			r.URL.Scheme = "https"
			r.URL.Host = net.JoinHostPort(client.option.host, client.option.port)
			r.Proto = "HTTP/1.1"
			r.ProtoMajor = 1
			r.ProtoMinor = 1
			lock.Lock()
			if err = r.Write(server); err != nil {
				server.Close()
				client.Close()
				lock.Unlock()
				return
			}
			resp, err := server.readResponse(r)
			if err != nil {
				server.Close()
				client.Close()
				lock.Unlock()
				return
			}
			lock.Unlock()
			if obj.RequestCallBack != nil {
				obj.RequestCallBack(r, resp)
			}
			for kk, vvs := range resp.Header {
				for _, vv := range vvs {
					w.Header().Add(kk, vv)
				}
			}
			w.WriteHeader(resp.StatusCode)
			if _, err = io.Copy(w, resp.Body); err != nil {
				server.Close()
				client.Close()
				return
			}
			if flush, ok := w.(Flusher); ok {
				flush.FlushError()
			}
			select {
			case <-server.option.ctx.Done():
				select {
				case <-client.option.ctx.Done():
				case <-ctx.Done():
				default:
					go func() {
						select {
						case <-client.option.ctx.Done():
						case <-ctx.Done():
						case <-r.Context().Done():
							client.Close()
						}
					}()
				}
			default:
				endSize.Add(1)
			}
		}),
	})
	select {
	case <-client.option.ctx.Done():
		return
	case <-server.option.ctx.Done():
		if startSize.Load() == endSize.Load() {
			return
		}
		select {
		case <-client.option.ctx.Done():
			return
		case <-ctx.Done():
			return
		}
	case <-ctx.Done():
		return
	}
}
func (obj *Client) http22Copy(preCtx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	serverConn, err := obj.http2Transport.NewClientConn(server)
	if err != nil {
		return err
	}
	defer serverConn.Close()
	ctx, cnl := context.WithCancel(preCtx)
	defer cnl()
	var startSize atomic.Int64
	var endSize atomic.Int64

	go obj.http2Server.ServeConn(client, &http2.ServeConnOpts{
		Context: ctx,
		Handler: http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				startSize.Add(1)
				r.URL.Scheme = "https"
				r.URL.Host = net.JoinHostPort(tools.GetServerName(client.option.host), client.option.port)
				resp, err := serverConn.RoundTrip(r)
				if err != nil {
					server.Close()
					client.Close()
					return
				}
				if obj.RequestCallBack != nil {
					obj.RequestCallBack(r, resp)
				}
				for kk, vvs := range resp.Header {
					for _, vv := range vvs {
						w.Header().Add(kk, vv)
					}
				}
				w.WriteHeader(resp.StatusCode)
				_, err = io.Copy(w, resp.Body)
				if err != nil {
					server.Close()
					client.Close()
					return
				}
				if flush, ok := w.(Flusher); ok {
					flush.FlushError()
				}
				select {
				case <-server.option.ctx.Done():
					select {
					case <-client.option.ctx.Done():
					case <-ctx.Done():
					default:
						go func() {
							select {
							case <-client.option.ctx.Done():
							case <-ctx.Done():
							case <-r.Context().Done():
								client.Close()
							}
						}()
					}
				default:
					endSize.Add(1)
				}
			},
		),
	},
	)
	select {
	case <-client.option.ctx.Done():
		return
	case <-server.option.ctx.Done():
		if startSize.Load() == endSize.Load() {
			return
		}
		select {
		case <-client.option.ctx.Done():
			return
		case <-ctx.Done():
			return
		}
	case <-ctx.Done():
		return
	}
}
func (obj *Client) http12Copy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	serverConn, err := obj.http2Transport.NewClientConn(server)
	if err != nil {
		return err
	}
	defer serverConn.Close()
	var req *http.Request
	var resp *http.Response
	var startSize atomic.Int64
	var endSize atomic.Int64
	go func() {
		defer client.Close()
		defer server.Close()
		for {
			if req, err = client.readRequest(); err != nil {
				return
			}
			startSize.Add(1)
			req.Proto = "HTTP/2.0"
			req.ProtoMajor = 2
			req.ProtoMinor = 0
			if resp, err = serverConn.RoundTrip(req); err != nil {
				return
			}
			if obj.RequestCallBack != nil {
				obj.RequestCallBack(req, resp)
			}
			resp.Proto = "HTTP/1.1"
			resp.ProtoMajor = 1
			resp.ProtoMinor = 1
			if err = resp.Write(client); err != nil {
				return
			}
			select {
			case <-server.option.ctx.Done():
				return
			default:
				endSize.Add(1)
			}
		}
	}()
	select {
	case <-client.option.ctx.Done():
		return
	case <-server.option.ctx.Done():
		if startSize.Load() == endSize.Load() {
			return
		}
		<-client.option.ctx.Done()
		return
	}
}
func (obj *Client) http11Copy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	var req *http.Request
	var rsp *http.Response
	var startSize atomic.Int64
	var endSize atomic.Int64
	donCha := make(chan struct{})
	go func() {
		defer close(donCha)
		for !server.option.isWs {
			if req, err = client.readRequest(); err != nil {
				server.Close()
				client.Close()
				return
			}
			startSize.Add(1)
			if err = req.Write(server); err != nil {
				server.Close()
				client.Close()
				return
			}
			if rsp, err = server.readResponse(req); err != nil {
				server.Close()
				client.Close()
				return
			}
			if obj.RequestCallBack != nil {
				obj.RequestCallBack(req, rsp)
			}
			if err = rsp.Write(client); err != nil {
				server.Close()
				client.Close()
				return
			}
			select {
			case <-server.option.ctx.Done():
				err = errors.New("server closed")
				client.Close()
				return
			default:
				endSize.Add(1)
			}
		}
	}()

	select {
	case <-donCha:
		return
	case <-client.option.ctx.Done():
		server.Close()
		if err == nil {
			err = errors.New("client closed")
		}
		return
	case <-server.option.ctx.Done():
		if startSize.Load() == endSize.Load() {
			client.Close()
			if err == nil {
				err = errors.New("server closed")
			}
			return
		}
		<-client.option.ctx.Done()
		if err == nil {
			err = errors.New("server closed")
		}
		return
	}
}
func (obj *Client) copyMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	if client.option.schema == "http" {
		return obj.copyHttpMain(ctx, client, server)
	} else if client.option.schema == "https" {
		if obj.RequestCallBack != nil || obj.WsCallBack != nil || obj.ja3 || obj.capture {
			return obj.copyHttpsMain(ctx, client, server)
		}
		return obj.copyHttpMain(ctx, client, server)
	} else {
		return errors.New("schema error")
	}
}
func (obj *Client) copyHttpMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer server.Close()
	defer client.Close()
	if client.option.http2 && !server.option.http2 { //http12 逻辑
		return obj.http21Copy(ctx, client, server)
	}
	if !client.option.http2 && server.option.http2 { //http12 逻辑
		return obj.http12Copy(ctx, client, server)
	}
	if client.option.http2 && server.option.http2 {
		if obj.RequestCallBack == nil {
			go func() {
				defer client.Close()
				defer server.Close()
				io.Copy(client, server)
			}()
			_, err = io.Copy(server, client)
			return err
		} else {
			return obj.http22Copy(ctx, client, server)
		}
	}
	if obj.RequestCallBack == nil && obj.WsCallBack == nil { //没有回调直接返回
		go func() {
			defer client.Close()
			defer server.Close()
			io.Copy(client, server)
		}()
		_, err = io.Copy(server, client)
		return err
	}
	if err = obj.http11Copy(ctx, client, server); err != nil { //http 开始回调
		return err
	}
	if obj.WsCallBack == nil { //没有ws 回调直接返回
		go func() {
			defer client.Close()
			defer server.Close()
			io.Copy(client, server)
		}()
		_, err = io.Copy(server, client)
		return err
	}
	//ws 开始回调
	wsClient := websocket.NewConn(client, false, client.option.wsOption)
	wsServer := websocket.NewConn(server, true, server.option.wsOption)
	defer wsServer.Close("close")
	defer wsClient.Close("close")
	go obj.wsRecv(ctx, wsClient, wsServer)
	return obj.wsSend(ctx, wsClient, wsServer)
}
func (obj *Client) copyHttpsMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	tlsServer, http2, certs, err := obj.tlsServer(ctx, server, client.option.host, client.option.isWs || server.option.isWs)
	if err != nil {
		return err
	}
	server.option.http2 = http2
	var cert tls.Certificate
	if len(certs) > 0 {
		cert, err = getCert2(certs[0])
	} else {
		cert, err = getCert(tools.GetServerName(client.option.host))
	}
	if err != nil {
		return err
	}
	tlsClient, http2, err := obj.tlsClient(ctx, client, client.option.isWs || server.option.isWs, cert)
	if err != nil {
		return err
	}
	client.option.http2 = http2
	clientProxy := NewProxyCon(ctx, tlsClient, bufio.NewReader(tlsClient), *client.option, true)
	serverProxy := NewProxyCon(ctx, tlsServer, bufio.NewReader(tlsServer), *server.option, false)
	return obj.copyHttpMain(ctx, clientProxy, serverProxy)
}
func (obj *Client) tlsClient(ctx context.Context, conn net.Conn, ws bool, cert tls.Certificate) (tlsConn *tls.Conn, http2 bool, err error) {
	var nextProtos []string
	if ws {
		nextProtos = []string{"http/1.1"}
	} else {
		nextProtos = []string{"h2", "http/1.1"}
	}
	tlsConn = tls.Server(conn, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
		NextProtos:         nextProtos,
	})
	if err = tlsConn.HandshakeContext(ctx); err != nil {
		return nil, false, err
	}
	return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", err
}
func (obj *Client) tlsServer(ctx context.Context, conn net.Conn, addr string, ws bool) (net.Conn, bool, []*x509.Certificate, error) {
	if obj.ja3 {
		if tlsConn, err := ja3.NewClient(ctx, conn, obj.ja3Spec, ws, addr); err != nil {
			return tlsConn, false, nil, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", tlsConn.ConnectionState().PeerCertificates, err
		}
	} else {
		var nextProtos []string
		if ws {
			nextProtos = []string{"http/1.1"}
		} else {
			nextProtos = []string{"h2", "http/1.1"}
		}
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(addr), NextProtos: nextProtos})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return tlsConn, false, nil, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", tlsConn.ConnectionState().PeerCertificates, err
		}
	}
}
