package requests

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"gitee.com/baixudong/gospider/http2"
	"gitee.com/baixudong/gospider/ja3"
)

type ClientOption struct {
	GetProxy              func(ctx context.Context, url *url.URL) (string, error) //根据url 返回代理，支持https,http,socks5 代理协议
	Proxy                 string                                                  //设置代理,支持https,http,socks5 代理协议
	TLSHandshakeTimeout   int64                                                   //tls 超时时间,default:15
	ResponseHeaderTimeout int64                                                   //第一个response headers 接收超时时间,default:30
	DisCookie             bool                                                    //关闭cookies管理
	DisAlive              bool                                                    //关闭长连接
	DisCompression        bool                                                    //关闭请求头中的压缩功能
	LocalAddr             string                                                  //本地网卡出口ip
	IdleConnTimeout       int64                                                   //空闲连接在连接池中的超时时间,default:30
	KeepAlive             int64                                                   //keepalive保活检测定时,default:15
	DnsCacheTime          int64                                                   //dns解析缓存时间60*30
	DisDnsCache           bool                                                    //是否关闭dns 缓存,影响dns 解析
	AddrType              AddrType                                                //优先使用的addr 类型
	GetAddrType           func(string) AddrType
	Dns                   string              //dns
	Ja3                   bool                //开启ja3
	Ja3Spec               ja3.ClientHelloSpec //指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	H2Ja3                 bool                //开启h2指纹
	H2Ja3Spec             ja3.H2Ja3Spec       //h2指纹

	RedirectNum   int                                         //重定向次数
	DisDecode     bool                                        //关闭自动编码
	DisRead       bool                                        //关闭默认读取请求体
	DisUnZip      bool                                        //变比自动解压
	TryNum        int64                                       //重试次数
	BeforCallBack func(context.Context, *RequestOption) error //请求前回调的方法
	AfterCallBack func(context.Context, *Response) error      //请求后回调的方法
	ErrCallBack   func(context.Context, error) bool           //请求error回调
	Timeout       int64                                       //请求超时时间
	Headers       any                                         //请求头
	Bar           bool                                        //是否开启bar
}
type Client struct {
	http2Upg      *http2.Upg
	redirectNum   int                                         //重定向次数
	disDecode     bool                                        //关闭自动编码
	disRead       bool                                        //关闭默认读取请求体
	disUnZip      bool                                        //变比自动解压
	tryNum        int64                                       //重试次数
	beforCallBack func(context.Context, *RequestOption) error //请求前回调的方法
	afterCallBack func(context.Context, *Response) error      //请求后回调的方法
	errCallBack   func(context.Context, error) bool           //请求error回调
	timeout       int64                                       //请求超时时间
	headers       any                                         //请求头
	bar           bool                                        //是否开启bar

	ja3           bool                //开启ja3
	ja3Spec       ja3.ClientHelloSpec //指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	disCookie     bool
	disAlive      bool
	client        *http.Client
	baseTransport *http.Transport
	ctx           context.Context
	cnl           context.CancelFunc
}

// 新建一个请求客户端,发送请求必须创建哈
func NewClient(preCtx context.Context, client_optinos ...ClientOption) (*Client, error) {
	var isG bool
	if preCtx == nil {
		preCtx = context.TODO()
	} else {
		isG = true
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
		session_option.KeepAlive = 15
	}
	if session_option.TLSHandshakeTimeout == 0 {
		session_option.TLSHandshakeTimeout = 15
	}
	if session_option.ResponseHeaderTimeout == 0 {
		session_option.ResponseHeaderTimeout = 30
	}
	if session_option.DnsCacheTime == 0 {
		session_option.DnsCacheTime = 60 * 30
	}
	dialClient, err := NewDail(DialOption{
		TLSHandshakeTimeout: session_option.TLSHandshakeTimeout,
		DnsCacheTime:        session_option.DnsCacheTime,
		GetProxy:            session_option.GetProxy,
		Proxy:               session_option.Proxy,
		KeepAlive:           session_option.KeepAlive,
		LocalAddr:           session_option.LocalAddr,
		AddrType:            session_option.AddrType,
		GetAddrType:         session_option.GetAddrType,
		DisDnsCache:         session_option.DisDnsCache,
		Dns:                 session_option.Dns,
	})
	if err != nil {
		cnl()
		return nil, err
	}
	var client http.Client
	//创建cookiesjar
	var jar *cookiejar.Jar
	if !session_option.DisCookie {
		if jar, err = cookiejar.New(nil); err != nil {
			cnl()
			return nil, err
		}
	}
	baseTransport := http.Transport{
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
		DialContext:           dialClient.requestHttpDialContext,
		DialTLSContext:        dialClient.requestHttpDialTlsContext,
		ForceAttemptHTTP2:     true,
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url = r.URL
			if ctxData.disProxy || ctxData.ja3 { //关闭代理或ja3, 走自实现代理
				return nil, nil
			}
			if ctxData.proxy != nil {
				ctxData.isCallback = true //官方代理实现
			}
			return ctxData.proxy, nil
		},
	}
	var http2Upg *http2.Upg
	if session_option.H2Ja3 || session_option.H2Ja3Spec.IsSet() {
		http2Upg = http2.NewUpg(http2.UpgOption{H2Ja3Spec: session_option.H2Ja3Spec, DialTLSContext: dialClient.requestHttp2DialTlsContext})
		baseTransport.TLSNextProto = map[string]func(authority string, c *tls.Conn) http.RoundTripper{
			"h2": http2Upg.UpgradeFn,
		}
	}
	client.Transport = baseTransport.Clone()
	client.Jar = jar
	client.CheckRedirect = checkRedirect
	result := &Client{
		ctx:           ctx,
		cnl:           cnl,
		client:        &client,
		http2Upg:      http2Upg,
		baseTransport: &baseTransport,
		disAlive:      session_option.DisAlive,
		disCookie:     session_option.DisCookie,
		ja3:           session_option.Ja3,
		ja3Spec:       session_option.Ja3Spec,

		redirectNum:   session_option.RedirectNum,
		disDecode:     session_option.DisDecode,
		disRead:       session_option.DisRead,
		disUnZip:      session_option.DisUnZip,
		tryNum:        session_option.TryNum,
		beforCallBack: session_option.BeforCallBack,
		afterCallBack: session_option.AfterCallBack,
		errCallBack:   session_option.ErrCallBack,
		timeout:       session_option.Timeout,
		headers:       session_option.Headers,
		bar:           session_option.Bar,
	}
	if isG {
		go func() {
			<-result.ctx.Done()
			result.Close()
		}()
	}
	return result, nil
}
func checkRedirect(req *http.Request, via []*http.Request) error {
	ctxData := req.Context().Value(keyPrincipalID).(*reqCtxData)
	if ctxData.redirectNum == 0 || ctxData.redirectNum >= len(via) {
		ctxData.url = req.URL
		ctxData.host = req.Host
		return nil
	}
	return http.ErrUseLastResponse
}

func (obj *Client) clone(request_option RequestOption) *http.Client {
	cli := &http.Client{
		CheckRedirect: obj.client.CheckRedirect,
	}
	if !request_option.DisCookie && obj.client.Jar != nil {
		cli.Jar = obj.client.Jar
	}
	if !request_option.DisAlive {
		cli.Transport = obj.client.Transport
	} else {
		cli.Transport = obj.baseTransport.Clone()
	}
	return cli
}

// 克隆请求客户端,返回一个由相同option 构造的客户端，但是不会克隆:连接池,ookies
func (obj *Client) Clone() *Client {
	result := *obj
	result.client.Transport = result.baseTransport.Clone()
	return &result
}

// 重置客户端至初始状态
func (obj *Client) Reset() error {
	if obj.client.Jar != nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return err
		}
		obj.client.Jar = jar
	}
	obj.CloseIdleConnections()
	obj.client.Transport = obj.baseTransport.Clone()
	return nil
}

// 关闭客户端
func (obj *Client) Close() {
	obj.CloseIdleConnections()
	obj.cnl()
}
func (obj *Client) Closed() bool {
	select {
	case <-obj.ctx.Done():
		return true
	default:
		return false
	}
}

// 关闭客户端中的空闲连接
func (obj *Client) CloseIdleConnections() {
	obj.client.CloseIdleConnections()
	obj.baseTransport.CloseIdleConnections()
	if obj.http2Upg != nil {
		obj.http2Upg.CloseIdleConnections()
	}
}

// 返回url 的cookies,也可以设置url 的cookies
func (obj *Client) Cookies(href string, cookies ...*http.Cookie) Cookies {
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

// 清除cookies
func (obj *Client) ClearCookies() error {
	var jar *cookiejar.Jar
	var err error
	jar, err = cookiejar.New(nil)
	if err != nil {
		return err
	}
	obj.client.Jar = jar
	obj.client.Jar = jar
	return nil
}
func (obj *Client) getClient(request_option RequestOption) *http.Client {
	if request_option.DisAlive || request_option.DisCookie {
		temp_client := obj.clone(request_option)
		return temp_client
	} else {
		return obj.client
	}
}
