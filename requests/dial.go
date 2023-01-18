package requests

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/tools"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

type dialClient struct {
	ctx        context.Context
	getProxy   func(ctx context.Context, url *url.URL) (string, error)
	proxy      *url.URL
	dialer     *net.Dialer
	dnsIpData  sync.Map
	dnsTimeout int64
}
type msgClient struct {
	time int64
	addr string
}
type proxyDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
	Dial(network, addr string) (c net.Conn, err error)
}

func ProxyFromUrl(u *url.URL, forward proxy.Dialer) (proxyDialer, error) {
	dial, err := proxy.FromURL(u, forward)
	if err != nil {
		return nil, err
	}
	dialer, ok := dial.(proxyDialer)
	if !ok {
		return dialer, errors.New("proxyDialer 转换失败")
	}
	return dialer, nil
}
func newDail(ctx context.Context, session_option ClientOption) (*dialClient, error) {
	var err error
	dialCli := &dialClient{
		dialer:     &net.Dialer{Timeout: time.Second * time.Duration(session_option.TLSHandshakeTimeout)},
		ctx:        ctx,
		dnsTimeout: session_option.DnsCacheTime,
		getProxy:   session_option.GetProxy,
	}
	if session_option.Proxy != "" {
		if dialCli.proxy, err = verifyProxy(session_option.Proxy); err != nil {
			return dialCli, err
		}
	}
	if session_option.LocalAddr != "" {
		if !strings.Contains(session_option.LocalAddr, ":") {
			session_option.LocalAddr += ":0"
		}
		if dialCli.dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", session_option.LocalAddr); err != nil {
			return dialCli, err
		}
	}
	if session_option.KeepAlive != 0 {
		dialCli.dialer.KeepAlive = time.Duration(session_option.KeepAlive) * time.Second //keepalive保活检测定时
	}
	return dialCli, err
}
func (obj *dialClient) setIpData(addr string, msgData msgClient) {
	obj.dnsIpData.Store(addr, msgData)
}
func (obj *dialClient) addrToIp(addr string) string {
	msgdataAny, ok := obj.dnsIpData.Load(addr)
	if ok {
		msgdata := msgdataAny.(msgClient)
		if time.Now().Unix()-msgdata.time < obj.dnsTimeout {
			return msgdata.addr
		}
	}
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		names, err := net.LookupIP(host)
		if err == nil && len(names) != 0 {
			addr = fmt.Sprintf("[%s]:%s", names[0].String(), port)
			obj.setIpData(addr, msgClient{
				time: time.Now().Unix(),
				addr: addr,
			})
		}
	}
	return addr
}

func (obj *dialClient) getSocksProxyConn(ctx context.Context, proxyData *url.URL, addr string) (net.Conn, error) {
	dial, err := ProxyFromUrl(proxyData, obj.dialer)
	if err != nil {
		return nil, err
	}
	return dial.DialContext(ctx, "tcp", addr)
}
func (obj *dialClient) getHttpProxyConn(ctx context.Context, proxyData *url.URL) (net.Conn, error) {
	rawConn, err := obj.getHttpConn(ctx, proxyData)
	if proxyData.User != nil {
		if password, ok := proxyData.User.Password(); ok {
			return &httpConn{
				rawConn:            rawConn,
				proxyAuthorization: tools.Base64Encode(proxyData.User.Username() + ":" + password),
			}, err
		}
	}
	return rawConn, err
}
func (obj *dialClient) getHttpConn(ctx context.Context, proxyData *url.URL) (net.Conn, error) {
	return obj.dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxyData.Hostname(), proxyData.Port()))
}
func Http2httpsConn(ctx context.Context, proxyData *url.URL, addr string, host string, conn net.Conn) error {
	var err error
	hdr := make(http.Header)
	hdr.Set("User-Agent", UserAgent)
	if proxyData.User != nil {
		if password, ok := proxyData.User.Password(); ok {
			hdr.Set("Proxy-Authorization", "Basic "+tools.Base64Encode(proxyData.User.Username()+":"+password))
		}
	}
	didReadResponse := make(chan struct{}) // closed after CONNECT write+read is done or fails
	var resp *http.Response
	go func() {
		defer close(didReadResponse)
		connectReq := &http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Opaque: addr},
			Host:   host,
			Header: hdr,
		}
		if err = connectReq.Write(conn); err != nil {
			return
		}
		resp, err = http.ReadResponse(bufio.NewReader(conn), connectReq)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-didReadResponse:
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		_, text, ok := strings.Cut(resp.Status, " ")
		if !ok {
			return errors.New("unknown status code")
		}
		return errors.New(text)
	}
	return nil
}
func (obj *dialClient) dialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	if reqData.url == nil {
		return nil, tools.WrapError(errFatal, "not found reqData.url")
	}
	if reqData.disProxy {
		return obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
	} else if reqData.proxy != nil {
		if !reqData.ja3 && !reqData.h2 { //ja3 必须https 才能设置，http2 的transport 没有proxy 方法
			rawConn, err := obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
			if err != nil {
				return rawConn, err
			}
			if reqData.proxyUser != nil && reqData.proxy.Scheme == "http" && reqData.url.Scheme == "http" {
				if password, ok := reqData.proxyUser.Password(); ok {
					return &httpConn{
						rawConn:            rawConn,
						proxyAuthorization: tools.Base64Encode(reqData.proxyUser.Username() + ":" + password),
					}, err
				}
			}
			return rawConn, err
		}
	} else if obj.getProxy != nil {
		proxyUrl, err := obj.getProxy(ctx, reqData.url)
		if err != nil {
			return nil, err
		}
		if reqData.proxy, err = verifyProxy(proxyUrl); err != nil {
			return nil, err
		}
	} else if obj.proxy != nil {
		reqData.proxy = obj.proxy
	}
	if reqData.proxy != nil {
		switch reqData.proxy.Scheme {
		case "socks5":
			return obj.getSocksProxyConn(ctx, reqData.proxy, obj.addrToIp(addr))
		case "http":
			switch reqData.url.Scheme {
			case "http":
				return obj.getHttpProxyConn(ctx, reqData.proxy)
			case "https":
				conn, err := obj.getHttpConn(ctx, reqData.proxy)
				if err != nil {
					return conn, err
				}
				if err = Http2httpsConn(ctx, reqData.proxy, obj.addrToIp(addr), addr, conn); err != nil {
					conn.Close()
				}
				return conn, err
			default:
				return nil, tools.WrapError(errFatal, "target url scheme error")
			}
		}
	}
	return obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
}

func (obj *dialClient) dialTlsContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	conn, err := obj.dialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]
	if reqData.ja3 {
		tlsConn := utls.UClient(conn, &utls.Config{InsecureSkipVerify: true, ServerName: serverName}, utls.HelloCustom)
		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if !reqData.h2 {
			for i := 0; i < len(spec.Extensions); i++ {
				if extension, ok := spec.Extensions[i].(*utls.ALPNExtension); ok {
					alns := []string{}
					for _, aln := range extension.AlpnProtocols {
						if aln != "h2" {
							alns = append(alns, aln)
						}
					}
					extension.AlpnProtocols = alns
				}
			}
		}
		if err = tlsConn.ApplyPreset(&spec); err != nil {
			conn.Close()
			return nil, err
		}
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, err
	}
	return tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName}), err
}
func (obj *dialClient) dialTlsContext2(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
	return obj.dialTlsContext(ctx, network, addr)
}
