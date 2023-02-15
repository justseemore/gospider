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

type dialClient struct {
	ctx        context.Context
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

func newDail(ctx context.Context, session_option ClientOption) (*dialClient, error) {
	var err error
	dialCli := &dialClient{
		dnsIpData:  make(map[string]msgClient),
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
	obj.lock.Lock()
	obj.dnsIpData[addr] = msgData
	obj.lock.Unlock()
}
func (obj *dialClient) getIpData(addr string) (msgClient, bool) {
	obj.lock.RLock()
	msgData, ok := obj.dnsIpData[addr]
	obj.lock.RUnlock()
	return msgData, ok
}

func (obj *dialClient) addrToIp(addr string) string {
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

func (obj *dialClient) loadHost(host string) (string, bool) {
	msgdata, ok := obj.getIpData(host)
	if ok && time.Now().Unix()-msgdata.time < obj.dnsTimeout {
		return msgdata.host, true
	}
	return host, false
}

type proxyDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

func verifySocks5(proxyData *url.URL, addr string, conn net.Conn) (err error) {
	if _, err = conn.Write([]byte{5, 2, 0, 2}); err != nil {
		return
	}
	readCon := make([]byte, 4)
	if _, err = io.ReadFull(conn, readCon[:2]); err != nil {
		return
	}
	switch readCon[1] {
	case 2:
		if proxyData.User == nil {
			err = errors.New("需要验证")
			return
		}
		pwd, pwdOk := proxyData.User.Password()
		if !pwdOk {
			err = errors.New("密码格式不对")
			return
		}
		usr := proxyData.User.Username()

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
	if host, port, err = tools.SplitHostPort(addr); err != nil {
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
func GetSocks5ProxyConn(ctx context.Context, dialer proxyDialer, proxyData *url.URL, addr string) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()

	if conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxyData.Hostname(), proxyData.Port())); err != nil {
		return
	}
	didVerify := make(chan struct{})
	go func() {
		defer close(didVerify)
		err = verifySocks5(proxyData, addr, conn)
	}()
	select {
	case <-ctx.Done():
		return conn, ctx.Err()
	case <-didVerify:
		return
	}
}
func GetHttpProxyConn(ctx context.Context, dialer *net.Dialer, proxyData *url.URL) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	conn, err = getHttpConn(ctx, dialer, proxyData)
	if proxyData.User != nil {
		if password, ok := proxyData.User.Password(); ok {
			return &httpConn{
				rawConn:            conn,
				proxyAuthorization: tools.Base64Encode(proxyData.User.Username() + ":" + password),
			}, err
		}
	}
	return
}
func GetHttpsProxyConn(ctx context.Context, dialer *net.Dialer, proxyData *url.URL, addr string, host string) (conn net.Conn, err error) {
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = getHttpConn(ctx, dialer, proxyData); err != nil {
		return
	}
	err = Http2HttpsConn(ctx, proxyData, addr, host, conn)
	return
}
func getHttpConn(ctx context.Context, dialer *net.Dialer, proxyData *url.URL) (net.Conn, error) {
	return dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxyData.Hostname(), proxyData.Port()))
}
func Http2HttpsConn(ctx context.Context, proxyData *url.URL, addr string, host string, conn net.Conn) (err error) {
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
func cloneUrl(u *url.URL) *url.URL {
	r := *u
	return &r
}
func (obj *dialClient) dialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	if reqData.url == nil {
		return nil, tools.WrapError(ErrFatal, "not found reqData.url")
	}
	var nowProxy *url.URL
	if reqData.disProxy { //关闭代理直接返回
		return obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
	} else if reqData.proxy != nil { //单独代理设置优先级最高
		nowProxy = cloneUrl(reqData.proxy)
		if reqData.isCallback { //走官方代理
			if reqData.proxyUser != nil {
				nowProxy.User = reqData.proxyUser
				return GetHttpProxyConn(ctx, obj.dialer, nowProxy)
			}
			return obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
		}
	} else if obj.getProxy != nil { //走自实现代理
		if proxyUrl, err := obj.getProxy(ctx, reqData.url); err != nil {
			return nil, err
		} else if nowProxy, err = verifyProxy(proxyUrl); err != nil {
			return nil, err
		}
	} else if obj.proxy != nil { //走自实现代理
		nowProxy = cloneUrl(obj.proxy)
	}
	if nowProxy != nil { //走自实现代理
		switch nowProxy.Scheme {
		case "socks5":
			return GetSocks5ProxyConn(ctx, obj.dialer, nowProxy, obj.addrToIp(addr))
		case "http":
			switch reqData.url.Scheme {
			case "http":
				return GetHttpProxyConn(ctx, obj.dialer, nowProxy)
			case "https":
				return GetHttpsProxyConn(ctx, obj.dialer, nowProxy, obj.addrToIp(addr), addr)
			default:
				return nil, tools.WrapError(ErrFatal, "target url scheme error")
			}
		}
	}
	return obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
}

// type TestRead struct {
// 	con net.Conn
// }

// func (obj *TestRead) Read(b []byte) (int, error) {
// 	i, err := obj.con.Read(b)
// 	log.Print("read: ", tools.BytesToString(b))
// 	return i, err
// }
// func (obj *TestRead) Write(b []byte) (int, error) {
// 	log.Print("writet: ", tools.BytesToString(b))
// 	i, err := obj.con.Write(b)
// 	return i, err
// }

// func (obj *TestRead) Close() error {
// 	return obj.con.Close()
// }
// func (obj *TestRead) LocalAddr() net.Addr {
// 	return obj.con.LocalAddr()
// }
// func (obj *TestRead) RemoteAddr() net.Addr {
// 	return obj.con.RemoteAddr()
// }
// func (obj *TestRead) SetDeadline(t time.Time) error {
// 	return obj.con.SetDeadline(t)
// }
// func (obj *TestRead) SetReadDeadline(t time.Time) error {
// 	return obj.con.SetReadDeadline(t)
// }
// func (obj *TestRead) SetWriteDeadline(t time.Time) error {
// 	return obj.con.SetWriteDeadline(t)
// }

func (obj *dialClient) dialTlsContext(ctx context.Context, network string, addr string) (tlsConn net.Conn, err error) {
	var conn net.Conn
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()
	if conn, err = obj.dialContext(ctx, network, addr); err != nil {
		return nil, err
	}
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]
	if reqData.ja3 {
		return ja3.Ja3DialContext(ctx, conn, reqData.ja3Id, reqData.h2, serverName)
	}
	// tt := TestRead{
	// 	con: tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName, NextProtos: []string{"h2", "http/1.1"}}),
	// }
	// return &tt, err
	return tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName, NextProtos: []string{"h2", "http/1.1"}}), err
}
func (obj *dialClient) dialTlsContext2(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
	return obj.dialTlsContext(ctx, network, addr)
}
