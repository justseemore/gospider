package requests

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/tools"
)

type DialClient struct {
	getProxy   func(ctx context.Context, url *url.URL) (string, error)
	proxy      *url.URL
	dialer     *net.Dialer
	dnsIpData  map[string]msgClient
	lock       sync.RWMutex
	dnsTimeout int64
}
type msgClient struct {
	time int64
	host string
}

type DialOption struct {
	TLSHandshakeTimeout int64
	DnsCacheTime        int64
	KeepAlive           int64
	GetProxy            func(ctx context.Context, url *url.URL) (string, error)
	Proxy               string
	LocalAddr           string
}

func NewDail(option DialOption) (*DialClient, error) {
	if option.KeepAlive == 0 {
		option.KeepAlive = 15
	}
	if option.TLSHandshakeTimeout == 0 {
		option.TLSHandshakeTimeout = 15
	}
	if option.DnsCacheTime == 0 {
		option.DnsCacheTime = 60 * 30
	}
	var err error
	dialCli := &DialClient{
		dnsIpData:  make(map[string]msgClient),
		dialer:     &net.Dialer{Timeout: time.Second * time.Duration(option.TLSHandshakeTimeout)},
		dnsTimeout: option.DnsCacheTime,
		getProxy:   option.GetProxy,
	}
	if option.Proxy != "" {
		if dialCli.proxy, err = verifyProxy(option.Proxy); err != nil {
			return dialCli, err
		}
	}
	if option.LocalAddr != "" {
		if !strings.Contains(option.LocalAddr, ":") {
			option.LocalAddr += ":0"
		}
		if dialCli.dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", option.LocalAddr); err != nil {
			return dialCli, err
		}
	}
	if option.KeepAlive != 0 {
		dialCli.dialer.KeepAlive = time.Duration(option.KeepAlive) * time.Second //keepalive保活检测定时
	}
	return dialCli, err
}

func (obj *DialClient) GetProxy(ctx context.Context, url *url.URL) (*url.URL, error) {
	if obj.proxy != nil {
		return obj.proxy, nil
	}
	if obj.getProxy != nil {
		href, err := obj.getProxy(ctx, url)
		if err != nil {
			return nil, err
		}
		return url.Parse(href)
	}
	return nil, nil
}

func (obj *DialClient) setIpData(addr string, msgData msgClient) {
	obj.lock.Lock()
	obj.dnsIpData[addr] = msgData
	obj.lock.Unlock()
}
func (obj *DialClient) getIpData(addr string) (msgClient, bool) {
	obj.lock.RLock()
	msgData, ok := obj.dnsIpData[addr]
	obj.lock.RUnlock()
	return msgData, ok
}
func (obj *DialClient) loadHost(host string) (string, bool) {
	msgdata, ok := obj.getIpData(host)
	if ok && time.Now().Unix()-msgdata.time < obj.dnsTimeout {
		return msgdata.host, true
	}
	return host, false
}
func (obj *DialClient) AddrToIp(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	_, ipInt := tools.ParseIp(host)
	if ipInt == 4 || ipInt == 6 {
		return addr
	}
	host, ok := obj.loadHost(host)
	if !ok {
		names, err := net.LookupIP(host)
		if err != nil || len(names) == 0 {
			return addr
		}
		host = names[0].String()
		obj.setIpData(addr, msgClient{time: time.Now().Unix(), host: host})
	}
	return net.JoinHostPort(host, port)
}

func (obj *DialClient) clientVerifySocks5(proxyUrl *url.URL, addr string, conn net.Conn) (err error) {
	if _, err = conn.Write([]byte{5, 2, 0, 2}); err != nil {
		return
	}
	readCon := make([]byte, 4)
	if _, err = io.ReadFull(conn, readCon[:2]); err != nil {
		return
	}
	switch readCon[1] {
	case 2:
		if proxyUrl.User == nil {
			err = errors.New("需要验证")
			return
		}
		pwd, pwdOk := proxyUrl.User.Password()
		if !pwdOk {
			err = errors.New("密码格式不对")
			return
		}
		usr := proxyUrl.User.Username()

		if usr == "" {
			err = errors.New("用户名格式不对")
			return
		}
		if _, err = conn.Write(append(
			append(
				[]byte{1, byte(len(usr))},
				tools.StringToBytes(usr)...,
			),
			append(
				[]byte{byte(len(pwd))},
				tools.StringToBytes(pwd)...,
			)...,
		)); err != nil {
			return
		}
		if _, err = io.ReadFull(conn, readCon[:2]); err != nil {
			return
		}
		switch readCon[1] {
		case 0:
		default:
			err = errors.New("验证失败")
			return
		}
	case 0:
	default:
		err = errors.New("不支持的验证方式")
		return
	}
	var host string
	var port int
	if host, port, err = tools.SplitHostPort(obj.AddrToIp(addr)); err != nil {
		return
	}
	writeCon := []byte{5, 1, 0}
	ip, ipInt := tools.ParseIp(host)
	switch ipInt {
	case 4:
		writeCon = append(writeCon, 1)
		writeCon = append(writeCon, ip...)
	case 6:
		writeCon = append(writeCon, 4)
		writeCon = append(writeCon, ip...)
	case 0:
		if len(host) > 255 {
			err = errors.New("FQDN too long")
			return
		}
		writeCon = append(writeCon, 3)
		writeCon = append(writeCon, byte(len(host)))
		writeCon = append(writeCon, host...)
	}
	writeCon = append(writeCon, byte(port>>8), byte(port))
	if _, err = conn.Write(writeCon); err != nil {
		return
	}
	if _, err = io.ReadFull(conn, readCon); err != nil {
		return
	}
	if readCon[0] != 5 {
		err = errors.New("版本不对")
		return
	}
	if readCon[1] != 0 {
		err = errors.New("连接失败")
		return
	}
	if readCon[3] != 1 {
		err = errors.New("连接类型不一致")
		return
	}

	switch readCon[3] {
	case 1: //ipv4地址
		if _, err = io.ReadFull(conn, readCon); err != nil {
			return
		}
	case 3: //域名
		if _, err = io.ReadFull(conn, readCon[:1]); err != nil { //域名的长度
			return
		}
		if _, err = io.ReadFull(conn, make([]byte, readCon[0])); err != nil {
			return
		}
	case 4: //IPv6地址
		if _, err = io.ReadFull(conn, make([]byte, 16)); err != nil {
			return
		}
	default:
		err = errors.New("invalid atyp")
		return
	}
	_, err = io.ReadFull(conn, readCon[:2])
	return
}
func cloneUrl(u *url.URL) *url.URL {
	r := *u
	return &r
}
func (obj *DialClient) DialContext(ctx context.Context, netword string, addr string) (net.Conn, error) {
	return obj.dialer.DialContext(ctx, netword, obj.AddrToIp(addr))
}
func (obj *DialClient) DialTlsProxyContext(ctx context.Context, netword string, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	conn, err := obj.DialContext(ctx, netword, addr)
	return tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetHostName(host), NextProtos: []string{"h2", "http/1.1"}}), err
}

func (obj *DialClient) newPwdConn(conn net.Conn, proxyUrl *url.URL) net.Conn {
	if password, ok := proxyUrl.User.Password(); ok {
		return &pwdConn{
			rawConn:            conn,
			proxyAuthorization: tools.Base64Encode(proxyUrl.User.Username() + ":" + password),
		}
	}
	return conn
}
func (obj *DialClient) Http2HttpProxy(ctx context.Context, network string, proxyUrl *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	conn, err = obj.DialContext(ctx, network, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port()))
	if proxyUrl.User != nil {
		return obj.newPwdConn(conn, proxyUrl), err
	}
	return
}
func (obj *DialClient) Http2HttpsProxy(ctx context.Context, network string, addr string, proxyUrl *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	conn, err = obj.DialTlsProxyContext(ctx, network, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port()))
	if proxyUrl.User != nil {
		return obj.newPwdConn(conn, proxyUrl), err
	}
	return
}
func (obj *DialClient) Http2Socks5Proxy(ctx context.Context, network string, addr string, proxyUrl *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = obj.DialContext(ctx, network, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port())); err != nil {
		return
	}
	didVerify := make(chan struct{})
	go func() {
		defer close(didVerify)
		err = obj.clientVerifySocks5(proxyUrl, addr, conn)
	}()
	select {
	case <-ctx.Done():
		return conn, ctx.Err()
	case <-didVerify:
		return
	}
}

func (obj *DialClient) clientVerifyHttps(ctx context.Context, proxyUrl *url.URL, addr string, host string, conn net.Conn) (err error) {
	hdr := make(http.Header)
	hdr.Set("User-Agent", UserAgent)
	if proxyUrl.User != nil {
		if password, ok := proxyUrl.User.Password(); ok {
			hdr.Set("Proxy-Authorization", "Basic "+tools.Base64Encode(proxyUrl.User.Username()+":"+password))
		}
	}
	didReadResponse := make(chan struct{}) // closed after CONNECT write+read is done or fails
	var resp *http.Response
	go func() {
		defer close(didReadResponse)
		connectReq := &http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Opaque: obj.AddrToIp(addr)},
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
		return
	}
	if resp.StatusCode != 200 {
		_, text, ok := strings.Cut(resp.Status, " ")
		if !ok {
			return errors.New("unknown status code")
		}
		return errors.New(text)
	}
	return
}

func (obj *DialClient) Https2HttpProxy(ctx context.Context, network string, addr string, host string, proxyUrl *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = obj.DialContext(ctx, network, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port())); err != nil {
		return
	}
	if err = obj.clientVerifyHttps(ctx, proxyUrl, addr, host, conn); err != nil {
		return
	}
	return
}

func (obj *DialClient) Https2HttpsProxy(ctx context.Context, network string, addr string, host string, proxyUrl *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = obj.DialTlsProxyContext(ctx, network, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port())); err != nil {
		return
	}
	if err = obj.clientVerifyHttps(ctx, proxyUrl, addr, host, conn); err != nil {
		return
	}
	return
}

func (obj *DialClient) Https2Socks5Proxy(ctx context.Context, network string, addr string, proxyUrl *url.URL) (conn net.Conn, err error) {
	return obj.Http2Socks5Proxy(ctx, network, addr, proxyUrl)
}

func (obj *DialClient) DialContextForProxy(ctx context.Context, netword string, scheme string, addr string, host string, proxyUrl *url.URL) (net.Conn, error) {
	if proxyUrl == nil {
		return obj.DialContext(ctx, netword, addr)
	}
	switch scheme {
	case "http":
		switch proxyUrl.Scheme {
		case "http":
			return obj.Http2HttpProxy(ctx, netword, proxyUrl)
		case "https":
			return obj.Http2HttpsProxy(ctx, netword, addr, proxyUrl)
		case "socks5":
			return obj.Http2Socks5Proxy(ctx, netword, addr, proxyUrl)
		default:
			return nil, errors.New("proxyUrl Scheme error")
		}
	case "https":
		switch proxyUrl.Scheme {
		case "http":
			return obj.Https2HttpProxy(ctx, netword, addr, host, proxyUrl)
		case "https":
			return obj.Https2HttpsProxy(ctx, netword, addr, host, proxyUrl)
		case "socks5":
			return obj.Https2Socks5Proxy(ctx, netword, addr, proxyUrl)
		default:
			return nil, errors.New("proxyUrl Scheme error")
		}
	default:
		return nil, errors.New("url Scheme error")
	}
}

func (obj *DialClient) requestHttpDialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	if reqData.url == nil {
		return nil, tools.WrapError(ErrFatal, "not found reqData.url")
	}
	var nowProxy *url.URL
	if reqData.disProxy { //关闭代理直接返回
		return obj.DialContext(ctx, network, addr)
	} else if reqData.proxy != nil { //单独代理设置优先级最高
		nowProxy = cloneUrl(reqData.proxy)
		if reqData.isCallback { //走官方代理
			if reqData.proxyUser != nil { //需要隐藏用户密码
				nowProxy.User = reqData.proxyUser
				if nowProxy.Scheme == "http" && reqData.url.Scheme == "http" { //这种情况添加用户密码
					return obj.Http2HttpProxy(ctx, network, nowProxy)
				}
			}
			return obj.DialContext(ctx, network, addr)
		}
	} else if tempProxy, err := obj.GetProxy(ctx, reqData.url); err != nil {
		return nil, err
	} else {
		nowProxy = cloneUrl(tempProxy)
	}
	if nowProxy != nil { //走自实现代理
		host := reqData.host
		if host == "" {
			var err error
			if host, _, err = net.SplitHostPort(addr); err != nil {
				return nil, err
			}
		}
		return obj.DialContextForProxy(ctx, network, reqData.url.Scheme, addr, host, nowProxy)
	}
	return obj.DialContext(ctx, network, addr)
}
func (obj *DialClient) requestHttpDialTlsContext(ctx context.Context, network string, addr string) (tlsConn net.Conn, err error) {
	var conn net.Conn
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = obj.requestHttpDialContext(ctx, network, addr); err != nil {
		return nil, err
	}
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	host := reqData.host
	if host == "" {
		addr = host
	}
	if reqData.ja3 {
		if utlsConn, err := ja3.Client(ctx, conn, reqData.ja3Id, host); err != nil {
			return utlsConn, err
		} else if reqData.h2 != (utlsConn.ConnectionState().NegotiatedProtocol == "h2") {
			return utlsConn, tools.WrapError(ErrFatal, "请强制设置http2")
		} else {
			tlsConn = utlsConn
		}
	} else {
		tlsConn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetHostName(host), NextProtos: []string{"h2", "http/1.1"}})
	}
	if reqData.isCallback && reqData.proxyUser != nil && reqData.proxy.Scheme == "https" && reqData.url.Scheme == "http" { //官方代理,有账号密码，代理为https,url 为http ，添加账号
		nowProxy := cloneUrl(reqData.proxy)
		nowProxy.User = reqData.proxyUser
		return obj.newPwdConn(tlsConn, nowProxy), err
	}
	return tlsConn, err
}
func (obj *DialClient) requestHttp2DialTlsContext(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) { //验证tls 是否可以直接用
	if cfg.ServerName != "" {
		ctx.Value(keyPrincipalID).(*reqCtxData).host = cfg.ServerName
	}
	return obj.requestHttpDialTlsContext(ctx, network, addr)
}
