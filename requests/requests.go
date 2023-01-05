package requests

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/bar"
	"gitee.com/baixudong/gospider/bs4"

	"gitee.com/baixudong/gospider/tools"

	utls "github.com/refraction-networking/utls"
	"github.com/tidwall/gjson"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
	"golang.org/x/net/publicsuffix"
	"nhooyr.io/websocket"
)

var UserAgent = `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36`

// 请求操作========================================================================= start
var defaultHeaders = http.Header{
	"Accept-encoding": []string{"gzip, deflate, br"},
	"Accept":          []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
	"Accept-Language": []string{"zh-CN,zh;q=0.9"},
	"User-Agent":      []string{UserAgent},
}

type myInt int

const (
	keyPrincipalID myInt = iota
)

var (
	errFatal = errors.New("致命错误")
)

type reqCtxData struct {
	proxyUser   *url.Userinfo
	proxy       *url.URL
	url         *url.URL
	redirectNum int
	disProxy    bool
}
type File struct {
	Key     string //字段的key
	Name    string //文件名
	Content []byte //文件的内容
	Type    string //文件类型
}

type httpConn struct {
	rawConn            net.Conn
	proxyAuthorization string
}

func (obj *httpConn) Write(b []byte) (n int, err error) {
	if obj.proxyAuthorization == "" {
		return obj.rawConn.Write(b)
	}
	b = bytes.Replace(b, []byte("\r\n"), tools.StringToBytes(fmt.Sprintf("\r\nProxy-Authorization: Basic %s\r\n", obj.proxyAuthorization)), 1)
	obj.proxyAuthorization = ""
	return obj.rawConn.Write(b)
}
func (obj *httpConn) Read(b []byte) (n int, err error) {
	return obj.rawConn.Read(b)
}
func (obj *httpConn) Close() error {
	return obj.rawConn.Close()
}
func (obj *httpConn) LocalAddr() net.Addr {
	return obj.rawConn.LocalAddr()
}
func (obj *httpConn) RemoteAddr() net.Addr {
	return obj.rawConn.RemoteAddr()
}
func (obj *httpConn) SetDeadline(t time.Time) error {
	return obj.rawConn.SetDeadline(t)
}
func (obj *httpConn) SetReadDeadline(t time.Time) error {
	return obj.rawConn.SetReadDeadline(t)
}
func (obj *httpConn) SetWriteDeadline(t time.Time) error {
	return obj.rawConn.SetWriteDeadline(t)
}

type RequestOption struct {
	Method        string
	Url           string //基础url
	Host          string //host
	Proxy         string //代理,http,socks5
	Timeout       int64  //请求超时时间
	Headers       any    //请求头
	Cookies       any    // cookies
	Files         []File //文件
	Params        any    //url params,url 参数key,val
	Form          any    //multipart/form-data,适用于文件上传
	Data          any    //application/x-www-form-urlencoded,适用于key,val
	Body          *bytes.Reader
	Json          any                                       //application/json
	Text          any                                       //text/xml
	TempData      any                                       //临时变量
	Bytes         []byte                                    //二进制内容
	DisCookie     bool                                      //关闭cookies管理
	DisDecode     bool                                      //关闭自动解码
	Bar           bool                                      //是否开启bar
	DisProxy      bool                                      //是否关闭代理
	TryNum        int64                                     //重试次数
	CurTryNum     int64                                     //当前尝试次数
	BeforCallBack func(*RequestOption)                      //请求之前回调
	AfterCallBack func(*RequestOption, *Response) *Response //请求之后回调
	RedirectNum   int                                       //重定向次数
	DisAlive      bool                                      //关闭长连接
	DisRead       bool                                      //关闭默认读取请求体
	DisUnZip      bool                                      //变比自动解压
	Err           error                                     //请求过程中的error
	Http2         bool                                      //开启http2 transport
	converUrl     string
	contentType   string
}
type Client struct {
	RedirectNum   int                                       //重定向次数
	DisDecode     bool                                      //关闭自动编码
	DisRead       bool                                      //关闭默认读取请求体
	DisUnZip      bool                                      //变比自动解压
	TryNum        int64                                     //重试次数
	BeforCallBack func(*RequestOption)                      //请求前回调的方法
	AfterCallBack func(*RequestOption, *Response) *Response //请求后回调的方法
	Timeout       int64                                     //请求超时时间
	Http2         bool                                      //开启http2 transport

	Headers map[string]string //请求头
	Bar     bool              //是否开启bar

	disCookie bool //关闭cookies管理
	disAlive  bool //关闭长连接

	client        *http.Client
	baseTransport *http.Transport

	client2        *http.Client
	baseTransport2 *http2.Transport

	ctx context.Context
	cnl context.CancelFunc
}
type ClientOption struct {
	GetProxy              func(ctx context.Context, url *url.URL) (string, error)
	Proxy                 string
	TLSHandshakeTimeout   int64  //tls 超时时间,default:8
	ResponseHeaderTimeout int64  //第一个response headers 接收超时时间,default:8
	DisCookie             bool   //关闭cookies管理
	DisAlive              bool   //关闭长连接
	DisCompression        bool   //关闭请求头中的压缩功能
	DisHttp2              bool   //关闭http2
	LocalAddr             string //本地网卡出口ip
	IdleConnTimeout       int64  //空闲连接在连接池中的超时时间,default:20
	KeepAlive             int64  //keepalive保活检测定时,default:10
	DnsCacheTime          int64  //dns解析缓存时间60*30
	Ja3                   bool   //是否启用ja3
}

type Response struct {
	Response  *http.Response
	WebSocket *websocket.Conn
	cnl       context.CancelFunc
	content   []byte
	encoding  string
	disDecode bool
	disUnzip  bool
}
type dialClient struct {
	ctx        context.Context
	getProxy   func(ctx context.Context, url *url.URL) (string, error)
	proxy      *url.URL
	dialer     *net.Dialer
	dnsIpData  sync.Map
	dnsTimeout int64
	ja3        bool
}
type msgClient struct {
	time int64
	addr string
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
	dial, err := proxy.FromURL(proxyData, obj.dialer)
	if err != nil {
		return nil, err
	}
	return dial.Dial("tcp", obj.addrToIp(addr))
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

func Http2httpsConn(ctx context.Context, proxyData *url.URL, addr string, conn net.Conn) error {
	var err error
	hdr := make(http.Header)
	hdr.Add("User-Agent", UserAgent)
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
			Host:   addr,
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
		if !obj.ja3 {
			rawConn, err := obj.dialer.DialContext(ctx, network, obj.addrToIp(addr))
			if err != nil {
				return rawConn, err
			}
			if reqData.proxyUser != nil && reqData.url.Scheme == "http" {
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
			return obj.getSocksProxyConn(ctx, reqData.proxy, addr)
		case "http":
			switch reqData.url.Scheme {
			case "http":
				return obj.getHttpProxyConn(ctx, reqData.proxy)
			case "https":
				conn, err := obj.getHttpConn(ctx, reqData.proxy)
				if err != nil {
					return conn, err
				}
				if err = Http2httpsConn(ctx, reqData.proxy, addr, conn); err != nil {
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
		return conn, err
	}
	if obj.ja3 {
		log.Print("ja3")
		tlsConn := utls.UClient(conn, &utls.Config{InsecureSkipVerify: true}, utls.HelloChrome_Auto)
		return tlsConn, tlsConn.HandshakeContext(ctx)
	}
	return tls.Client(conn, &tls.Config{InsecureSkipVerify: true}), err
}
func (obj *dialClient) dialTlsContext2(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
	return obj.dialTlsContext(ctx, network, addr)
}
func newBody(val any, valType string, dataMap map[string][]string) (*bytes.Reader, error) {
	switch value := val.(type) {
	case gjson.Result:
		if !value.IsObject() {
			return nil, errors.New("body-type错误")
		}
		switch valType {
		case "json", "text":
			return bytes.NewReader(tools.StringToBytes(value.Raw)), nil
		case "data", "params":
			tempVal := url.Values{}
			for kk, vv := range value.Map() {
				if vv.IsArray() {
					for _, v := range vv.Array() {
						tempVal.Add(kk, v.String())
					}
				} else {
					tempVal.Add(kk, vv.String())
				}
			}
			return bytes.NewReader(tools.StringToBytes(tempVal.Encode())), nil
		case "form":
			for kk, vv := range value.Map() {
				kkvv := []string{}
				if vv.IsArray() {
					for _, v := range vv.Array() {
						kkvv = append(kkvv, v.String())
					}
				} else {
					kkvv = append(kkvv, vv.String())
				}
				dataMap[kk] = kkvv
			}
			return nil, nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case string:
		switch valType {
		case "json", "text", "data":
			return bytes.NewReader(tools.StringToBytes(value)), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case []byte:
		switch valType {
		case "json", "text", "data":
			return bytes.NewReader(value), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case io.Reader:
		tempCon, err := io.ReadAll(value)
		return bytes.NewReader(tempCon), err
	default:
		return newBody(tools.Any2json(value), valType, dataMap)
	}
}
func (obj *RequestOption) newHeaders() error {
	if obj.Headers == nil {
		obj.Headers = defaultHeaders.Clone()
		return nil
	}
	switch headers := obj.Headers.(type) {
	case http.Header:
		return nil
	case gjson.Result:
		if !headers.IsObject() {
			return errors.New("new headers error")
		}
		head := http.Header{}
		for kk, vv := range headers.Map() {
			if vv.IsArray() {
				for _, v := range vv.Array() {
					head.Add(kk, v.String())
				}
			} else {
				head.Add(kk, vv.String())
			}
		}
		obj.Headers = head
		return nil
	default:
		obj.Headers = tools.Any2json(headers)
		return obj.newHeaders()
	}
}
func (obj *RequestOption) newCookies() error {
	if obj.Cookies == nil {
		return nil
	}
	switch cookies := obj.Cookies.(type) {
	case []*http.Cookie:
		return nil
	case gjson.Result:
		if !cookies.IsObject() {
			return errors.New("new cookies error")
		}
		cook := []*http.Cookie{}
		for kk, vv := range cookies.Map() {
			if vv.IsArray() {
				for _, v := range vv.Array() {
					cook = append(cook, &http.Cookie{
						Name:  kk,
						Value: v.String(),
					})
				}
			} else {
				cook = append(cook, &http.Cookie{
					Name:  kk,
					Value: vv.String(),
				})
			}
		}
		obj.Cookies = cook
		return nil
	default:
		obj.Cookies = tools.Any2json(cookies)
		return obj.newCookies()
	}
}
func (obj *RequestOption) optionInit() error {
	obj.converUrl = obj.Url
	var err error
	if obj.Bytes != nil {
		obj.Body = bytes.NewReader(obj.Bytes)
	}
	if obj.Form != nil {
		tempBody := bytes.NewBuffer(nil)
		dataMap := map[string][]string{}
		obj.Body, err = newBody(obj.Form, "form", dataMap)
		if err != nil {
			return err
		}
		writer := multipart.NewWriter(tempBody)
		for key, vals := range dataMap {
			for _, val := range vals {
				err := writer.WriteField(key, val)
				if err != nil {
					return err
				}
			}
		}
		escapeQuotes := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
		for _, file := range obj.Files {
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
					escapeQuotes.Replace(file.Key), escapeQuotes.Replace(file.Name)))
			if file.Type == "" {
				h.Set("Content-Type", "application/octet-stream")
			} else {
				h.Set("Content-Type", file.Type)
			}
			wp, err := writer.CreatePart(h)
			if err != nil {
				return err
			}
			_, err = wp.Write(file.Content)
			if err != nil {
				return err
			}
		}
		err = writer.Close()
		if err != nil {
			return err
		}
		obj.contentType = writer.FormDataContentType()
		temCon, err := io.ReadAll(tempBody)
		if err != nil {
			return err
		}
		obj.Body = bytes.NewReader(temCon)
	}
	if obj.Data != nil {
		obj.Body, err = newBody(obj.Data, "data", nil)
		if err != nil {
			return err
		}
		obj.contentType = "application/x-www-form-urlencoded"
	}
	if obj.Json != nil {
		obj.Body, err = newBody(obj.Json, "json", nil)
		if err != nil {
			return err
		}
		obj.contentType = "application/json"
	}
	if obj.Text != nil {
		obj.Body, err = newBody(obj.Text, "text", nil)
		if err != nil {
			return err
		}
		obj.contentType = "text/plain"
	}
	if obj.Params != nil {

		tempParam, err := newBody(obj.Params, "params", nil)
		if err != nil {
			return err
		}
		con, err := io.ReadAll(tempParam)
		if err != nil {
			return err
		}
		pu, err := url.Parse(obj.Url)
		if err != nil {
			return err
		}
		if pu.Query() == nil {
			obj.converUrl = obj.Url + "?" + tools.BytesToString(con)
		} else {
			obj.converUrl = obj.Url + "&" + tools.BytesToString(con)
		}
	}
	if err = obj.newHeaders(); err != nil {
		return err
	}
	return obj.newCookies()
}

func newDail(ctx context.Context, session_option ClientOption) (*dialClient, error) {
	var err error
	dialCli := &dialClient{
		dialer:     &net.Dialer{Timeout: time.Second * 8},
		ctx:        ctx,
		ja3:        session_option.Ja3,
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
func newHttp2Transport(ctx context.Context, session_option ClientOption, dialCli *dialClient) *http2.Transport {
	return &http2.Transport{
		DisableCompression: session_option.DisCompression,
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DialTLSContext:     dialCli.dialTlsContext2,
		AllowHTTP:          true,
		ReadIdleTimeout:    time.Duration(session_option.IdleConnTimeout) * time.Second, //空闲连接在连接池中的超时时间
		PingTimeout:        time.Second * time.Duration(session_option.TLSHandshakeTimeout),
		WriteByteTimeout:   time.Second * time.Duration(session_option.ResponseHeaderTimeout),
	}
}
func newHttpTransport(ctx context.Context, session_option ClientOption, dialCli *dialClient) http.Transport {
	return http.Transport{
		MaxIdleConns:        655350,
		MaxConnsPerHost:     655350,
		MaxIdleConnsPerHost: 655350,
		ProxyConnectHeader: http.Header{
			"User-Agent": []string{UserAgent},
		},
		TLSHandshakeTimeout:   time.Second * time.Duration(session_option.TLSHandshakeTimeout),
		ResponseHeaderTimeout: time.Second * time.Duration(session_option.ResponseHeaderTimeout),
		DisableKeepAlives:     session_option.DisAlive,
		DisableCompression:    session_option.DisCompression,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout:       time.Duration(session_option.IdleConnTimeout) * time.Second, //空闲连接在连接池中的超时时间

		ForceAttemptHTTP2: !session_option.DisHttp2,

		DialContext:    dialCli.dialContext,
		DialTLSContext: dialCli.dialTlsContext,
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url = r.URL
			if session_option.Ja3 {
				return nil, nil
			}
			if ctxData.proxy != nil && ctxData.proxy.User != nil {
				ctxData.proxyUser, ctxData.proxy.User = ctxData.proxy.User, nil
			}
			return ctxData.proxy, nil
		},
	}
}

func cloneTransport(t *http2.Transport) *http2.Transport {
	return &http2.Transport{
		DisableCompression: t.DisableCompression,
		TLSClientConfig:    t.TLSClientConfig,
		DialTLSContext:     t.DialTLSContext,
		AllowHTTP:          t.AllowHTTP,
		ReadIdleTimeout:    t.ReadIdleTimeout, //空闲连接在连接池中的超时时间
		PingTimeout:        t.PingTimeout,
		WriteByteTimeout:   t.WriteByteTimeout,
	}
}
func checkRedirect(req *http.Request, via []*http.Request) error {
	ctxData := req.Context().Value(keyPrincipalID).(*reqCtxData)
	if ctxData.redirectNum == 0 || ctxData.redirectNum >= len(via) {
		ctxData.url = req.URL
		return nil
	}
	return http.ErrUseLastResponse
}
func NewClient(preCtx context.Context, client_optinos ...ClientOption) (*Client, error) {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	ctx, cnl := context.WithCancel(preCtx)
	var session_option ClientOption
	//初始化参数
	if len(client_optinos) > 0 {
		session_option = client_optinos[0]
	}
	if session_option.IdleConnTimeout == 0 {
		session_option.IdleConnTimeout = 30
	}
	if session_option.KeepAlive == 0 {
		session_option.KeepAlive = 10
	}
	if session_option.TLSHandshakeTimeout == 0 {
		session_option.TLSHandshakeTimeout = 10
	}
	if session_option.ResponseHeaderTimeout == 0 {
		session_option.ResponseHeaderTimeout = 30
	}
	if session_option.DnsCacheTime == 0 {
		session_option.DnsCacheTime = 60 * 30
	}
	dialClient, err := newDail(ctx, session_option)
	if err != nil {
		cnl()
		return nil, err
	}
	var client http.Client
	var client2 http.Client
	//创建cookiesjar
	var jar *cookiejar.Jar
	if !session_option.DisCookie {
		if jar, err = cookiejar.New(nil); err != nil {
			cnl()
			return nil, err
		}
	}
	baseTransport := newHttpTransport(ctx, session_option, dialClient)
	baseTransport2 := newHttp2Transport(ctx, session_option, dialClient)

	client.Transport = baseTransport.Clone()
	client2.Transport = cloneTransport(baseTransport2)

	client.Jar = jar
	client2.Jar = jar

	client.CheckRedirect = checkRedirect
	client2.CheckRedirect = checkRedirect

	return &Client{ctx: ctx, cnl: cnl, client: &client, baseTransport: &baseTransport, client2: &client2, baseTransport2: baseTransport2, disAlive: session_option.DisAlive, disCookie: session_option.DisCookie}, nil
}

func (obj *Client) newRequestOption(option *RequestOption) {
	if option.TryNum == 0 {
		option.TryNum = obj.TryNum
	}
	if option.BeforCallBack == nil {
		option.BeforCallBack = obj.BeforCallBack
	}
	if option.AfterCallBack == nil {
		option.AfterCallBack = obj.AfterCallBack
	}
	if option.Headers == nil {
		if obj.Headers == nil {
			option.Headers = defaultHeaders.Clone()
		} else {
			option.Headers = obj.Headers
		}
	}
	if !option.Bar {
		option.Bar = obj.Bar
	}
	if option.RedirectNum == 0 {
		option.RedirectNum = obj.RedirectNum
	}
	if option.Timeout == 0 {
		option.Timeout = obj.Timeout
	}
	if !option.DisAlive {
		option.DisAlive = obj.disAlive
	}
	if !option.DisCookie {
		option.DisCookie = obj.disCookie
	}
	if !option.DisDecode {
		option.DisDecode = obj.DisDecode
	}
	if !option.DisRead {
		option.DisRead = obj.DisRead
	}
	if !option.DisUnZip {
		option.DisUnZip = obj.DisUnZip
	}
	if !option.Http2 {
		option.Http2 = obj.Http2
	}
}

func (obj *Client) Request(preCtx context.Context, method string, href string, options ...RequestOption) (*Response, error) {
	if obj == nil {
		return nil, errors.New("初始化client失败")
	}
	if preCtx == nil {
		preCtx = obj.ctx
	}
	var option RequestOption
	if len(options) > 0 {
		option = options[0]
	}
	var err error
	option.Method = method
	option.Url = href
	if option.Body != nil {
		option.TryNum = 0
	}
	obj.newRequestOption(&option)
	if option.BeforCallBack == nil {
		if err = option.optionInit(); err != nil {
			return nil, err
		}
	}
	//开始请求
	var resp *Response
	for ; option.CurTryNum <= option.TryNum; option.CurTryNum++ {
		select {
		case <-preCtx.Done():
			return nil, preCtx.Err()
		default:
			if option.BeforCallBack != nil {
				option.BeforCallBack(&option)
			}
			if option.BeforCallBack != nil || option.AfterCallBack != nil {
				obj.newRequestOption(&option)
				if err = option.optionInit(); err != nil {
					return nil, err
				}
			}
			if option.Body != nil {
				option.Body.Seek(0, 0)
			}
			resp, option.Err = obj.tempRequest(preCtx, option)
			if option.Err != nil && errors.Is(option.Err, errFatal) {
				return resp, option.Err
			}
			if option.AfterCallBack != nil {
				callBackRespon := option.AfterCallBack(&option, resp)
				if callBackRespon != nil && option.Err == nil {
					return callBackRespon, option.Err
				}
			} else if option.Err == nil {
				return resp, option.Err
			}
		}
	}
	if option.Err != nil {
		return resp, option.Err
	}
	return resp, errors.New("max try num")
}

func (obj *Client) newResponse(r *http.Response, request_option RequestOption) (*Response, error) {
	response := &Response{Response: r}
	if request_option.DisRead { //是否预读
		return response, nil
	}
	if request_option.DisUnZip || r.Uncompressed { //是否解压
		response.disUnzip = true
	}
	response.disDecode = request_option.DisDecode      //是否解码
	return response, response.read(request_option.Bar) //读取内容
}
func (obj *Client) getClient(request_option RequestOption) *http.Client {
	if request_option.DisAlive || request_option.DisCookie {
		temp_client := obj.clone(request_option)
		return temp_client
	} else {
		if request_option.Http2 {
			return obj.client2
		}
		return obj.client
	}
}
func verifyProxy(proxyUrl string) (*url.URL, error) {
	proxy, err := url.Parse(proxyUrl)
	if err != nil {
		return nil, err
	}
	switch proxy.Scheme {
	case "http", "socks5":
		return proxy, nil
	default:
		return nil, tools.WrapError(errFatal, "不支持的代理协议")
	}
}

func (obj *Client) tempRequest(preCtx context.Context, request_option RequestOption) (*Response, error) {
	method := strings.ToUpper(request_option.Method)
	href := request_option.converUrl
	var reqs *http.Request
	var err error
	var isWs bool
	//构造ctxData
	ctxData := new(reqCtxData)
	ctxData.disProxy = request_option.DisProxy
	if request_option.Proxy != "" { //代理相关构造
		tempProxy, err := verifyProxy(request_option.Proxy)
		if err != nil {
			return nil, tools.WrapError(errFatal, err)
		}
		ctxData.proxy = tempProxy
	}
	if request_option.RedirectNum != 0 { //重定向次数
		ctxData.redirectNum = request_option.RedirectNum
	}
	//构造ctx,cnl
	var cancel context.CancelFunc
	reqCtx := context.WithValue(preCtx, keyPrincipalID, ctxData)
	if request_option.Timeout > 0 { //超时
		reqCtx, cancel = context.WithTimeout(reqCtx, time.Duration(request_option.Timeout)*time.Second)
	} else {
		reqCtx, cancel = context.WithCancel(reqCtx)
	}

	//创建request
	if request_option.Body != nil {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, request_option.Body)
	} else {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, nil)
	}
	if err != nil {
		cancel()
		return nil, tools.WrapError(errFatal, err)
	}
	ctxData.url = reqs.URL
	//判断ws
	if reqs.URL.Scheme == "ws" || reqs.URL.Scheme == "wss" {
		isWs = true
	}
	//添加headers
	var headOk bool
	if reqs.Header, headOk = request_option.Headers.(http.Header); !headOk {
		cancel()
		return nil, tools.WrapError(errFatal, "headers 转换错误")
	}

	if !isWs && reqs.Header.Get("Content-type") == "" && request_option.contentType != "" {
		reqs.Header.Add("Content-Type", request_option.contentType)
	}
	//host构造
	if request_option.Host != "" {
		reqs.Host = request_option.Host
	} else if reqs.Header.Get("Host") != "" {
		reqs.Host = reqs.Header.Get("Host")
	}

	//添加cookies
	if request_option.Cookies != nil {
		cooks, cookOk := request_option.Cookies.([]*http.Cookie)
		if !cookOk {
			cancel()
			return nil, tools.WrapError(errFatal, "cookies 转换错误")
		}
		for _, vv := range cooks {
			reqs.AddCookie(vv)
		}
	}
	if !request_option.Http2 {
		reqs.Close = request_option.DisAlive
	}
	//开始发送请求
	var r *http.Response
	var r2 *websocket.Conn
	var response *Response
	var err2 error
	if isWs {
		r2, r, err = websocket.Dial(reqCtx, href, &websocket.DialOptions{
			HTTPClient: obj.getClient(request_option),
			HTTPHeader: reqs.Header,
		})
	} else {
		r, err = obj.getClient(request_option).Do(reqs)
	}
	if r != nil {
		if isWs && r.StatusCode == 101 {
			request_option.DisRead = true
		}
		r.Close = request_option.DisAlive
		if response, err2 = obj.newResponse(r, request_option); err2 != nil { //创建 response
			cancel()
			return response, err2
		}
		response.WebSocket = r2
		if request_option.DisRead {
			response.cnl = cancel
		} else {
			cancel()
		}
		if err != nil {
			cancel()
		}
	} else {
		cancel()
	}
	return response, err
}

func (obj *Client) clone(request_option RequestOption) *http.Client {
	cli := &http.Client{
		CheckRedirect: obj.client.CheckRedirect,
	}
	if !request_option.DisCookie && obj.client.Jar != nil {
		cli.Jar = obj.client.Jar
	}
	if request_option.Http2 {
		if !request_option.DisAlive {
			cli.Transport = obj.client2.Transport
		} else {
			cli.Transport = cloneTransport(obj.baseTransport2)
		}
	} else {
		if !request_option.DisAlive {
			cli.Transport = obj.client.Transport
		} else {
			cli.Transport = obj.baseTransport.Clone()
		}
	}
	return cli
}
func (obj *Client) Reset() error {
	if obj.client.Jar != nil {
		jar, err := cookiejar.New(&cookiejar.Options{
			PublicSuffixList: publicsuffix.List,
		})
		if err != nil {
			return err
		}
		obj.client.Jar = jar
	}
	obj.CloseIdleConnections()
	obj.client.Transport = obj.baseTransport.Clone()
	return nil
}
func (obj *Client) Close() {
	obj.CloseIdleConnections()
	obj.cnl()
}
func (obj *Client) CloseIdleConnections() {
	obj.client.CloseIdleConnections()
}
func (obj *Client) Cookies(href string, cookies ...*http.Cookie) []*http.Cookie {
	if obj.client.Jar == nil {
		return nil
	}
	u, err := url.Parse(href)
	if err != nil {
		return nil
	}
	obj.client.Jar.SetCookies(u, cookies)
	return obj.client.Jar.Cookies(u)
}
func (obj *Client) ClearCookies() error {
	var jar *cookiejar.Jar
	var err error
	jar, err = cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return err
	}
	obj.client.Jar = jar
	obj.client.Jar = jar
	return nil
}
func (obj *Response) Location() (*url.URL, error) {
	return obj.Response.Location()
}
func (obj *Response) Cookies() []*http.Cookie {
	if obj.Response == nil {
		return []*http.Cookie{}
	}
	return obj.Response.Cookies()
}
func (obj *Response) StatusCode() int {
	if obj.Response == nil {
		return 0
	}
	return obj.Response.StatusCode
}
func (obj *Response) Url() *url.URL {
	if obj.Response == nil {
		return nil
	}
	return obj.Response.Request.URL
}
func (obj *Response) Headers() http.Header {
	return obj.Response.Header
}

func (obj *Response) Text(val ...string) string {
	if len(val) > 0 {
		obj.content = tools.StringToBytes(val[0])
	}
	return tools.BytesToString(obj.content)
}
func (obj *Response) Decode(encoding string) {
	if obj.encoding != encoding {
		obj.encoding = encoding
		obj.content = tools.Decode(obj.content, encoding)
	}
}
func (obj *Response) Map(path ...string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal(obj.content, &data); err != nil {
		return nil
	}
	return data
}
func (obj *Response) Json(path ...string) gjson.Result {
	return tools.Any2json(obj.content, path...)
}
func (obj *Response) Content(val ...[]byte) []byte {
	if len(val) > 0 {
		obj.content = val[0]
	}
	return obj.content
}
func (obj *Response) Html() *bs4.Client {
	return bs4.NewClient(obj.Text(), obj.Url().String())
}
func (obj *Response) ContentType() string {
	return obj.Response.Header.Get("Content-Type")
}
func (obj *Response) ContentEncoding() string {
	return obj.Response.Header.Get("Content-Encoding")
}
func (obj *Response) ContentLength() int64 {
	return obj.Response.ContentLength
}

type barBody struct {
	body *bytes.Buffer
	bar  *bar.Client
}

func (obj *barBody) Write(con []byte) (int, error) {
	l, err := obj.body.Write(con)
	obj.bar.Print(int64(l))
	return l, err
}
func (obj *Response) barRead() (*bytes.Buffer, error) {
	barData := &barBody{
		bar:  bar.NewClient(obj.Response.ContentLength),
		body: bytes.NewBuffer(nil),
	}
	_, err := io.Copy(barData, obj.Response.Body)
	if err != nil {
		return nil, err
	}
	return barData.body, nil
}
func (obj *Response) verifyBytes() bool {
	return strings.Contains(obj.Headers().Get("Accept-Ranges"), "bytes")
}

func (obj *Response) read(bar bool) error { //读取body,对body 解压，解码操作
	defer obj.Close()
	var bBody *bytes.Buffer
	var err error
	if bar && obj.ContentLength() > 0 { //是否打印进度条,读取内容
		bBody, err = obj.barRead()
	} else {
		bBody = bytes.NewBuffer(nil)
		_, err = io.Copy(bBody, obj.Response.Body)
	}
	if err != nil {
		return errors.New("io.Copy error: " + err.Error())
	}
	if !obj.disUnzip {
		if bBody, err = tools.ZipDecode(bBody, obj.ContentEncoding()); err != nil {
			return errors.New("gzip NewReader error: " + err.Error())
		}
	}
	obj.content = bBody.Bytes()
	if !obj.disDecode && !obj.verifyBytes() {
		obj.content, obj.encoding = tools.Charset(obj.content, obj.ContentType())
	}
	return nil
}
func (obj *Response) Close() error {
	if obj.cnl != nil {
		defer obj.cnl()
	}
	io.Copy(io.Discard, obj.Response.Body)
	err := obj.Response.Body.Close()
	return err
}
