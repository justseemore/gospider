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

	"github.com/justseemore/gospider/ja3"
	"github.com/justseemore/gospider/tools"
	utls "github.com/refraction-networking/utls"
)

type DialClient struct {
	getProxy     func(ctx context.Context, url *url.URL) (string, error)
	proxy        *url.URL
	dialer       *net.Dialer
	dnsIpData    sync.Map
	proxyLock    sync.RWMutex
	dnsTimeout   time.Duration
	addrType     AddrType //使用ipv4,ipv6 ,或自动选项
	getAddrType  func(string) AddrType
	proxyJa3     bool //是否启用ja3
	proxyJa3Spec ja3.Ja3Spec
	ja3          bool //是否启用ja3
	ja3Spec      ja3.Ja3Spec
	dns          string //dns
	resolver     *net.Resolver
	ctx          context.Context
	utlsConfig   *utls.Config
	tlsConfig    *tls.Config
}
type msgClient struct {
	time time.Time
	host string
}
type AddrType int

const (
	Auto AddrType = 0
	Ipv4 AddrType = 4
	Ipv6 AddrType = 6
)

type DialOption struct {
	TLSHandshakeTimeout time.Duration
	DnsCacheTime        time.Duration
	KeepAlive           time.Duration
	GetProxy            func(ctx context.Context, url *url.URL) (string, error)
	Proxy               string   //代理
	LocalAddr           string   //使用本地网卡
	AddrType            AddrType //优先使用的地址类型,ipv4,ipv6 ,或自动选项
	GetAddrType         func(string) AddrType
	Ja3                 bool        //是否启用ja3
	Ja3Spec             ja3.Ja3Spec //指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	ProxyJa3            bool        //代理是否启用ja3
	ProxyJa3Spec        ja3.Ja3Spec //指定代理ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	Dns                 string      //dns
}

func NewDail(ctx context.Context, option DialOption) (*DialClient, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	if option.KeepAlive == 0 {
		option.KeepAlive = time.Second * 30
	}
	if option.TLSHandshakeTimeout == 0 {
		option.TLSHandshakeTimeout = time.Second * 15
	}
	if option.DnsCacheTime == 0 {
		option.DnsCacheTime = time.Second * 60 * 30
	}
	if option.Ja3Spec.IsSet() {
		option.Ja3 = true
	}
	if option.ProxyJa3Spec.IsSet() {
		option.ProxyJa3 = true
	}
	var err error
	dialCli := &DialClient{
		utlsConfig: &utls.Config{
			InsecureSkipVerify:     true,
			InsecureSkipTimeVerify: true,
			SessionTicketKey:       [32]byte{},
			ClientSessionCache:     utls.NewLRUClientSessionCache(0),
			OmitEmptyPsk:           true,
		},
		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
			SessionTicketKey:   [32]byte{},
			ClientSessionCache: tls.NewLRUClientSessionCache(0),
		},
		ctx: ctx,
		dialer: &net.Dialer{
			Timeout:   option.TLSHandshakeTimeout,
			KeepAlive: option.KeepAlive,
		},
		dnsTimeout:   option.DnsCacheTime,
		getProxy:     option.GetProxy,
		addrType:     option.AddrType,
		getAddrType:  option.GetAddrType,
		proxyJa3:     option.ProxyJa3,
		proxyJa3Spec: option.ProxyJa3Spec,
		ja3:          option.Ja3,
		ja3Spec:      option.Ja3Spec,
		dns:          option.Dns,
	}
	dialCli.resolver = &net.Resolver{
		Dial: dialCli.DnsDialContext,
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
	dialCli.dialer.SetMultipathTCP(true)
	return dialCli, err
}

func (obj *DialClient) GetProxy(ctx context.Context, href *url.URL) (*url.URL, error) {
	obj.proxyLock.RLock()
	defer obj.proxyLock.RUnlock()
	if obj.proxy != nil {
		return obj.proxy, nil
	}
	if obj.getProxy != nil {
		proxy, err := obj.getProxy(ctx, href)
		if proxy == "" || err != nil {
			return nil, err
		}
		return url.Parse(proxy)
	}
	return nil, nil
}
func (obj *DialClient) Proxy() *url.URL {
	obj.proxyLock.RLock()
	defer obj.proxyLock.RUnlock()
	if obj.proxy == nil {
		return nil
	}
	return cloneUrl(obj.proxy)
}
func (obj *DialClient) SetProxy(proxy string) error {
	obj.proxyLock.Lock()
	defer obj.proxyLock.Unlock()
	if proxy == "" {
		obj.proxy = nil
		return nil
	}
	tmpProxy, err := verifyProxy(proxy)
	if err != nil {
		return err
	}
	if obj.proxy == nil {
		obj.proxy = tmpProxy
	} else {
		*obj.proxy = *tmpProxy
	}
	return nil
}
func (obj *DialClient) SetGetProxy(getProxy func(ctx context.Context, url *url.URL) (string, error)) {
	obj.proxyLock.Lock()
	defer obj.proxyLock.Unlock()
	obj.getProxy = getProxy
}
func (obj *DialClient) Dialer() *net.Dialer {
	return obj.dialer
}
func (obj *DialClient) loadHost(host string) (string, bool) {
	msgDataAny, ok := obj.dnsIpData.Load(host)
	if ok {
		msgdata := msgDataAny.(msgClient)
		if time.Now().Sub(msgdata.time) < obj.dnsTimeout {
			return msgdata.host, true
		}
	}
	return host, false
}
func (obj *DialClient) AddrToIp(ctx context.Context, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, tools.WrapError(err, "addrToIp 错误,SplitHostPort")
	}
	_, ipInt := tools.ParseHost(host)
	if ipInt == 4 || ipInt == 6 {
		return addr, nil
	}
	host, ok := obj.loadHost(host)
	if !ok {
		ip, err := obj.lookupIPAddr(ctx, host)
		if err != nil {
			return addr, tools.WrapError(err, "addrToIp 错误,lookupIPAddr")
		}
		host = ip.String()
		obj.dnsIpData.Store(addr, msgClient{time: time.Now(), host: host})
	}
	return net.JoinHostPort(host, port), nil
}

func (obj *DialClient) clientVerifySocks5(ctx context.Context, proxyUrl *url.URL, addr string, conn net.Conn) (err error) {
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
	if host, port, err = tools.SplitHostPort(addr); err != nil {
		return
	}
	writeCon := []byte{5, 1, 0}
	ip, ipInt := tools.ParseHost(host)
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
func (obj *DialClient) DnsDialContext(ctx context.Context, netword string, addr string) (net.Conn, error) {
	if obj.dns != "" {
		if strings.Contains(obj.dns, ":") {
			addr = obj.dns
		} else {
			addr = net.JoinHostPort(obj.dns, "53")
		}
	}
	return obj.dialer.DialContext(ctx, netword, addr)
}
func (obj *DialClient) lookupIPAddr(ctx context.Context, host string) (net.IP, error) {
	var addrType int
	if obj.addrType != 0 {
		addrType = int(obj.addrType)
	} else if obj.getAddrType != nil {
		addrType = int(obj.getAddrType(host))
	}
	ips, err := obj.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ipType := tools.ParseIp(ip); ipType == 4 || ipType == 6 {
			if addrType == 0 || addrType == ipType {
				return ip, nil
			}
		}
	}
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ipType := tools.ParseIp(ip); ipType == 4 || ipType == 6 {
			return ip, nil
		}
	}
	return nil, errors.New("dns 解析host 失败")
}
func (obj *DialClient) DialContext(ctx context.Context, netword string, addr string) (net.Conn, error) {
	revHost, err := obj.AddrToIp(ctx, addr)
	if err != nil {
		return nil, err
	}
	return obj.dialer.DialContext(ctx, netword, revHost)
}
func (obj *DialClient) AddProxyTls(ctx context.Context, conn net.Conn, host string) (net.Conn, error) {
	if obj.proxyJa3 {
		config := obj.utlsConfig.Clone()
		config.ServerName = tools.GetServerName(host)
		if !obj.proxyJa3Spec.IsSet() {
			obj.proxyJa3Spec = ja3.DefaultJa3Spec()
		}
		if !obj.proxyJa3Spec.HasPsk() {
			ja3.AddPsk(&obj.proxyJa3Spec)
		}
		return ja3.NewClient(ctx, conn, obj.proxyJa3Spec, true, config)
	}
	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(host), NextProtos: []string{"http/1.1"}})
	return tlsConn, tlsConn.HandshakeContext(ctx)
}
func (obj *DialClient) AddTls(ctx context.Context, conn net.Conn, host string, disHttp bool) (tlsConn *tls.Conn, err error) {
	if obj.ja3 {
		var utlsConn *utls.UConn
		config := obj.utlsConfig.Clone()
		config.ServerName = tools.GetServerName(host)
		if !obj.ja3Spec.IsSet() {
			obj.ja3Spec = ja3.DefaultJa3Spec()
		}
		if !obj.ja3Spec.HasPsk() {
			ja3.AddPsk(&obj.ja3Spec)
		}
		utlsConn, err = ja3.NewClient(ctx, conn, obj.ja3Spec, disHttp, config)
		if err != nil {
			err = tools.WrapError(err, "dialClient AddTls ja3.NewClient错误")
			return nil, err
		}
		if tlsConn, err = ja3.Utls2Tls(obj.ctx, ctx, utlsConn, host); err != nil {
			err = tools.WrapError(err, "dialClient AddTls Utls2Tls 错误")
		}
		return
	}
	if disHttp {
		tlsConn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(host), NextProtos: []string{"http/1.1"}})
	} else {
		tlsConn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(host), NextProtos: []string{"h2", "http/1.1"}})
	}
	if err = tlsConn.HandshakeContext(ctx); err != nil {
		err = tools.WrapError(err, "dialClient AddTls tls HandshakeContext 错误")
	}
	return tlsConn, err
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
func (obj *DialClient) Socks5Proxy(ctx context.Context, network string, addr string, proxyUrl *url.URL) (conn net.Conn, err error) {
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
		err = obj.clientVerifySocks5(ctx, proxyUrl, addr, conn)
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
		cHost := host
		_, hport, _ := net.SplitHostPort(host)
		if hport == "" {
			_, aport, _ := net.SplitHostPort(addr)
			if aport != "" {
				cHost = net.JoinHostPort(cHost, aport)
			}
		}
		connectReq := &http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Opaque: addr},
			Host:   cHost,
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
func (obj *DialClient) DialContextWithProxy(ctx context.Context, netword string, scheme string, addr string, host string, proxyUrl *url.URL) (net.Conn, error) {
	if proxyUrl == nil {
		return obj.DialContext(ctx, netword, addr)
	}
	if proxyUrl.Port() == "" {
		if proxyUrl.Scheme == "http" {
			proxyUrl.Host = net.JoinHostPort(proxyUrl.Hostname(), "80")
		} else if proxyUrl.Scheme == "https" {
			proxyUrl.Host = net.JoinHostPort(proxyUrl.Hostname(), "443")
		}
	}
	switch proxyUrl.Scheme {
	case "http", "https":
		conn, err := obj.DialContext(ctx, netword, net.JoinHostPort(proxyUrl.Hostname(), proxyUrl.Port()))
		if err != nil {
			return conn, err
		} else if proxyUrl.Scheme == "https" {
			if conn, err = obj.AddTls(ctx, conn, proxyUrl.Host, true); err != nil {
				return conn, err
			}
		}
		return conn, obj.clientVerifyHttps(ctx, proxyUrl, addr, host, conn)
	case "socks5":
		return obj.Socks5Proxy(ctx, netword, addr, proxyUrl)
	default:
		return nil, errors.New("proxyUrl Scheme error")
	}
}
func (obj *DialClient) requestHttpDialContext(ctx context.Context, network string, addr string) (conn net.Conn, err error) {
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	if reqData.url == nil {
		return nil, tools.WrapError(ErrFatal, "not found reqData.url")
	}
	var nowProxy *url.URL
	if reqData.disProxy || reqData.isCallback { //走正常连接
		if conn, err = obj.DialContext(ctx, network, addr); err != nil {
			err = tools.WrapError(err, "requestHttpDialContext DialContext 错误")
		}
		return
	} else if reqData.proxy != nil { //单独代理设置优先级最高
		nowProxy = reqData.proxy
	} else if nowProxy, err = obj.GetProxy(ctx, reqData.url); err != nil {
		err = tools.WrapError(err, "requestHttpDialContext GetProxy 错误")
		return nil, err
	}
	if nowProxy != nil { //走自实现代理
		if conn, err = obj.DialContextWithProxy(ctx, network, reqData.url.Scheme, addr, reqData.host, nowProxy); err != nil {
			err = tools.WrapError(err, "requestHttpDialContext DialContextWithProxy 错误")
		}
		return
	}
	if conn, err = obj.DialContext(ctx, network, addr); err != nil {
		err = tools.WrapError(err, "requestHttpDialContext DialContext2 错误")
	}
	return
}
func (obj *DialClient) requestHttpDialTlsContext(preCtx context.Context, network string, addr string) (conn net.Conn, err error) {
	if conn, err = obj.requestHttpDialContext(preCtx, network, addr); err != nil {
		return conn, err
	}
	ctx, cnl := context.WithTimeout(preCtx, obj.dialer.Timeout)
	defer cnl()
	reqData := ctx.Value(keyPrincipalID).(*reqCtxData)
	conn, err = obj.AddTls(ctx, conn, reqData.host, reqData.ws)
	return
}
func (obj *DialClient) requestHttp2DialTlsContext(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) { //验证tls 是否可以直接用
	if cfg.ServerName != "" {
		ctx.Value(keyPrincipalID).(*reqCtxData).host = cfg.ServerName
	}
	return obj.requestHttpDialTlsContext(ctx, network, addr)
}
