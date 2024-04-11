package requests

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/justseemore/gospider/http2"
	"github.com/justseemore/gospider/ja3"
)

type ClientOption struct {
	GetProxy              func(ctx context.Context, url *url.URL) (string, error) //根据url 返回代理，支持https,http,socks5 代理协议
	Proxy                 string                                                  //设置代理,支持https,http,socks5 代理协议
	TLSHandshakeTimeout   time.Duration                                           //tls 超时时间,default:15
	ResponseHeaderTimeout time.Duration                                           //第一个response headers 接收超时时间,default:30
	DisCookie             bool                                                    //关闭cookies管理
	DisCompression        bool                                                    //关闭请求头中的压缩功能
	LocalAddr             string                                                  //本地网卡出口ip
	IdleConnTimeout       time.Duration                                           //空闲连接在连接池中的超时时间,default:90
	KeepAlive             time.Duration                                           //keepalive保活检测定时,default:30
	DnsCacheTime          time.Duration                                           //dns解析缓存时间60*30
	AddrType              AddrType                                                //优先使用的addr 类型
	GetAddrType           func(string) AddrType
	Dns                   string        //dns
	Ja3                   bool          //开启ja3
	Ja3Spec               ja3.Ja3Spec   //指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	H2Ja3                 bool          //开启h2指纹
	H2Ja3Spec             ja3.H2Ja3Spec //h2指纹

	RedirectNum int   //重定向次数,小于0为禁用,0:不限制
	DisDecode   bool  //关闭自动编码
	DisRead     bool  //关闭默认读取请求体
	DisUnZip    bool  //变比自动解压
	TryNum      int64 //重试次数

	OptionCallBack func(context.Context, *RequestOption) error //请求参数回调,用于对请求参数进行修改。返回error,中断重试请求,返回nil继续
	ResultCallBack func(context.Context, *Response) error      //结果回调,用于对结果进行校验。返回nil，直接返回,返回err的话，如果有errCallBack 走errCallBack，没有继续try
	ErrCallBack    func(context.Context, error) error          //错误回调,返回error,中断重试请求,返回nil继续

	Timeout time.Duration //请求超时时间
	Headers any           //请求头
	Bar     bool          //是否开启bar
}
type Client struct {
	http2Upg    *http2.Upg
	redirectNum int   //重定向次数
	disDecode   bool  //关闭自动编码
	disRead     bool  //关闭默认读取请求体
	disUnZip    bool  //变比自动解压
	tryNum      int64 //重试次数

	optionCallBack func(context.Context, *RequestOption) error //请求参数回调,用于对请求参数进行修改。返回error,中断重试请求,返回nil继续
	resultCallBack func(context.Context, *Response) error      //结果回调,用于对结果进行校验。返回nil，直接返回,返回err的话，如果有errCallBack 走errCallBack，没有继续try
	errCallBack    func(context.Context, error) error          //错误回调,返回error,中断重试请求,返回nil继续

	timeout time.Duration //请求超时时间
	headers any           //请求头
	bar     bool          //是否开启bar
	dialer  *DialClient   //dialer

	disCookie bool
	client    *http.Client

	ctx context.Context
	cnl context.CancelFunc
}

// 新建一个请求客户端,发送请求必须创建哈
func NewClient(preCtx context.Context, options ...ClientOption) (*Client, error) {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	ctx, cnl := context.WithCancel(preCtx)
	var option ClientOption
	//初始化参数
	if len(options) > 0 {
		option = options[0]
	}
	if option.IdleConnTimeout == 0 {
		option.IdleConnTimeout = time.Second * 90
	}
	if option.KeepAlive == 0 {
		option.KeepAlive = time.Second * 30
	}
	if option.TLSHandshakeTimeout == 0 {
		option.TLSHandshakeTimeout = time.Second * 15
	}
	if option.ResponseHeaderTimeout == 0 {
		option.ResponseHeaderTimeout = time.Second * 30
	}
	if option.DnsCacheTime == 0 {
		option.DnsCacheTime = time.Second * 60 * 30
	}
	if option.Ja3Spec.IsSet() {
		option.Ja3 = true
	}
	dialClient, err := NewDail(ctx, DialOption{
		Ja3:                 option.Ja3,
		Ja3Spec:             option.Ja3Spec,
		TLSHandshakeTimeout: option.TLSHandshakeTimeout,
		DnsCacheTime:        option.DnsCacheTime,
		GetProxy:            option.GetProxy,
		Proxy:               option.Proxy,
		KeepAlive:           option.KeepAlive,
		LocalAddr:           option.LocalAddr,
		AddrType:            option.AddrType,
		GetAddrType:         option.GetAddrType,
		Dns:                 option.Dns,
	})
	if err != nil {
		cnl()
		return nil, err
	}
	var client http.Client
	//创建cookiesjar
	var jar *cookiejar.Jar
	if !option.DisCookie {
		jar = newJar()
	}
	transport := &http.Transport{
		MaxIdleConns:        655350,
		MaxConnsPerHost:     655350,
		MaxIdleConnsPerHost: 655350,
		ProxyConnectHeader: http.Header{
			"User-Agent": []string{UserAgent},
		},
		TLSHandshakeTimeout:   option.TLSHandshakeTimeout,
		ResponseHeaderTimeout: option.ResponseHeaderTimeout,
		DisableCompression:    option.DisCompression,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout:       option.IdleConnTimeout, //空闲连接在连接池中的超时时间
		DialContext:           dialClient.requestHttpDialContext,
		DialTLSContext:        dialClient.requestHttpDialTlsContext,
		ForceAttemptHTTP2:     true,
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url, ctxData.host = r.URL, r.Host
			if ctxData.host == "" {
				ctxData.host = ctxData.url.Host
			}
			if ctxData.requestCallBack != nil {
				req, err := cloneRequest(r, ctxData.disBody)
				if err != nil {
					return nil, err
				}
				if err = ctxData.requestCallBack(r.Context(), req); err != nil {
					return nil, err
				}
			}
			return nil, nil
		},
	}
	var http2Upg *http2.Upg
	if option.H2Ja3 || option.H2Ja3Spec.IsSet() {
		http2Upg = http2.NewUpg(transport, http2.UpgOption{H2Ja3Spec: option.H2Ja3Spec, DialTLSContext: dialClient.requestHttp2DialTlsContext})
		transport.TLSNextProto = map[string]func(authority string, c *tls.Conn) http.RoundTripper{
			"h2": func(authority string, c *tls.Conn) http.RoundTripper {
				return http2Upg.UpgradeFn(authority, c)
			},
		}
	}
	client.Transport = transport
	client.Jar = jar
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		ctxData := req.Context().Value(keyPrincipalID).(*reqCtxData)
		if ctxData.responseCallBack != nil {
			resp, err := cloneResponse(req.Response, false)
			if err != nil {
				return err
			}
			if err = ctxData.responseCallBack(req.Context(), resp); err != nil {
				return err
			}
		}
		if ctxData.redirectNum == 0 || ctxData.redirectNum >= len(via) {
			return nil
		}
		return http.ErrUseLastResponse
	}
	result := &Client{
		ctx:            ctx,
		cnl:            cnl,
		dialer:         dialClient,
		client:         &client,
		http2Upg:       http2Upg,
		disCookie:      option.DisCookie,
		redirectNum:    option.RedirectNum,
		disDecode:      option.DisDecode,
		disRead:        option.DisRead,
		disUnZip:       option.DisUnZip,
		tryNum:         option.TryNum,
		optionCallBack: option.OptionCallBack,
		resultCallBack: option.ResultCallBack,
		errCallBack:    option.ErrCallBack,
		timeout:        option.Timeout,
		headers:        option.Headers,
		bar:            option.Bar,
	}
	return result, nil
}

func (obj *Client) SetProxy(proxy string) error {
	return obj.dialer.SetProxy(proxy)
}
func (obj *Client) SetGetProxy(getProxy func(ctx context.Context, url *url.URL) (string, error)) {
	obj.dialer.SetGetProxy(getProxy)
}

// 关闭客户端
func (obj *Client) Close() {
	obj.CloseIdleConnections()
	obj.cnl()
}

// 关闭客户端中的空闲连接
func (obj *Client) CloseIdleConnections() {
	obj.client.CloseIdleConnections()
	if obj.http2Upg != nil {
		obj.http2Upg.CloseIdleConnections()
	}
}

// 返回url 的cookies,也可以设置url 的cookies
func (obj *Client) Cookies(href string, cookies ...any) (Cookies, error) {
	return cookie(obj.client.Jar, href, cookies...)
}

type Jar struct {
	jar *cookiejar.Jar
}

func newJar() *cookiejar.Jar {
	jar, _ := cookiejar.New(nil)
	return jar
}

func NewJar() *Jar {
	return &Jar{
		jar: newJar(),
	}
}
func (obj *Jar) Cookies(href string, cookies ...any) (Cookies, error) {
	return cookie(obj.jar, href, cookies...)
}
func (obj *Jar) ClearCookies() {
	*obj.jar = *newJar()
}

func cookie(jar http.CookieJar, href string, cookies ...any) (Cookies, error) {
	if jar == nil {
		return nil, nil
	}
	u, err := url.Parse(href)
	if err != nil {
		return nil, err
	}
	for _, cookie := range cookies {
		cooks, err := ReadCookies(cookie)
		if err != nil {
			return nil, err
		}
		jar.SetCookies(u, cooks)
	}
	return jar.Cookies(u), nil
}

// 清除cookies
func (obj *Client) ClearCookies() {
	if obj.client.Jar != nil {
		*obj.client.Jar.(*cookiejar.Jar) = *newJar()
	}
}
func (obj *Client) getClient(option RequestOption) *http.Client {
	if option.Jar != nil {
		return &http.Client{
			Transport:     obj.client.Transport,
			CheckRedirect: obj.client.CheckRedirect,
			Jar:           option.Jar.jar,
		}
	}
	if !option.DisCookie || obj.client.Jar == nil {
		return obj.client
	}
	return &http.Client{
		Transport:     obj.client.Transport,
		CheckRedirect: obj.client.CheckRedirect,
	}
}
