package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

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
func (obj *Client) http21Copy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	var lock sync.Mutex

	go obj.http2Server.ServeConn(client, &http2.ServeConnOpts{
		Context: ctx,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			}
		}),
	})
	select {
	case <-client.option.ctx.Done():
		return client.option.ctx.Err()
	case <-server.option.ctx.Done():
		return server.option.ctx.Err()
	}
}
func (obj *Client) http22Copy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	serverConn, err := obj.http2Transport.NewClientConn(server)
	if err != nil {
		return err
	}
	go obj.http2Server.ServeConn(client, &http2.ServeConnOpts{
		Context: ctx,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = "https"
			r.URL.Host = net.JoinHostPort(client.option.host, client.option.port)
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
			if _, err = io.Copy(w, resp.Body); err != nil {
				server.Close()
				client.Close()
			}
		}),
	})
	select {
	case <-client.option.ctx.Done():
		return client.option.ctx.Err()
	case <-server.option.ctx.Done():
		return server.option.ctx.Err()
	}
}
func (obj *Client) http12Copy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	serverConn, err := obj.http2Transport.NewClientConn(server)
	if err != nil {
		return err
	}
	var req *http.Request
	var resp *http.Response
	for {
		if req, err = client.readRequest(); err != nil {
			return err
		}
		req.Proto = "HTTP/2.0"
		req.ProtoMajor = 2
		req.ProtoMinor = 0
		if resp, err = serverConn.RoundTrip(req); err != nil {
			return err
		}
		if obj.RequestCallBack != nil {
			obj.RequestCallBack(req, resp)
		}
		resp.Proto = "HTTP/1.1"
		resp.ProtoMajor = 1
		resp.ProtoMinor = 1
		if err = resp.Write(client); err != nil {
			return err
		}
	}
}
func (obj *Client) httpCopy(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	var req *http.Request
	var rsp *http.Response
	for !server.option.isWs {
		if req, err = client.readRequest(); err != nil {
			return err
		}
		if err = req.Write(server); err != nil {
			return err
		}

		if rsp, err = server.readResponse(req); err != nil {
			return err
		}
		if obj.RequestCallBack != nil {
			obj.RequestCallBack(req, rsp)
		}
		if err = rsp.Write(client); err != nil {
			return err
		}
	}
	return nil
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
			go io.Copy(client, server)
			_, err = io.Copy(server, client)
			return err
		} else {
			return obj.http22Copy(ctx, client, server)
		}
	}
	if obj.RequestCallBack == nil && obj.WsCallBack == nil { //没有回调直接返回
		go io.Copy(client, server)
		_, err = io.Copy(server, client)
		return err
	}
	if err = obj.httpCopy(ctx, client, server); err != nil { //http 开始回调
		return err
	}
	if obj.WsCallBack == nil { //没有ws 回调直接返回
		go io.Copy(client, server)
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
	clientProxy := NewProxyCon(ctx, tlsClient, bufio.NewReader(tlsClient), *client.option)
	serverProxy := NewProxyCon(ctx, tlsServer, bufio.NewReader(tlsServer), *server.option)
	return obj.copyHttpMain(ctx, clientProxy, serverProxy)
}
