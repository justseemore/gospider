package requests

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"golang.org/x/net/http2"
	"golang.org/x/net/publicsuffix"
)

type ClientOption struct {
	GetProxy              func(ctx context.Context, url *url.URL) (string, error)
	Proxy                 string
	TLSHandshakeTimeout   int64  //tls 超时时间,default:15
	ResponseHeaderTimeout int64  //第一个response headers 接收超时时间,default:30
	DisCookie             bool   //关闭cookies管理
	DisAlive              bool   //关闭长连接
	DisCompression        bool   //关闭请求头中的压缩功能
	LocalAddr             string //本地网卡出口ip
	IdleConnTimeout       int64  //空闲连接在连接池中的超时时间,default:30
	KeepAlive             int64  //keepalive保活检测定时,default:15
	DnsCacheTime          int64  //dns解析缓存时间60*30
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
	Ja3           bool                                      //开启ja3

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
func checkRedirect(req *http.Request, via []*http.Request) error {
	ctxData := req.Context().Value(keyPrincipalID).(*reqCtxData)
	if ctxData.redirectNum == 0 || ctxData.redirectNum >= len(via) {
		ctxData.url = req.URL
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
