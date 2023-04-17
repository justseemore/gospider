package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/kinds"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"
	"gitee.com/baixudong/gospider/websocket"
	"golang.org/x/net/http2"
)

//go:embed gospider.crt
var CrtFile []byte

//go:embed gospider.key
var KeyFile []byte

func getCert(serverName string) (tlsCert tls.Certificate, err error) {
	crt, err := tools.LoadCertData(CrtFile)
	if err != nil {
		return tlsCert, err
	}
	key, err := tools.LoadCertKeyData(KeyFile)
	if err != nil {
		return tlsCert, err
	}
	cert, err := tools.GetCertWithCN(crt, key, serverName)
	if err != nil {
		return tlsCert, err
	}
	return tools.GetTlsCert(cert, key)
}
func getCert2(preCert *x509.Certificate) (tlsCert tls.Certificate, err error) {
	crt, err := tools.LoadCertData(CrtFile)
	if err != nil {
		return tlsCert, err
	}
	key, err := tools.LoadCertKeyData(KeyFile)
	if err != nil {
		return tlsCert, err
	}
	cert, err := tools.GetCertWithCert(crt, key, preCert)
	if err != nil {
		return tlsCert, err
	}
	return tools.GetTlsCert(cert, key)
}

type ClientOption struct {
	Ja3                 bool                                                    //是否开启ja3
	Ja3Spec             ja3.ClientHelloSpec                                     //指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	ProxyJa3            bool                                                    //连接代理时是否开启ja3
	ProxyJa3Spec        ja3.ClientHelloSpec                                     //连接代理时指定ja3Spec,//指定ja3Spec,使用ja3.CreateSpecWithStr 或者ja3.CreateSpecWithId 生成
	DisDnsCache         bool                                                    //是否关闭dns 缓存
	Usr                 string                                                  //用户名
	Pwd                 string                                                  //密码
	IpWhite             []net.IP                                                //白名单 192.168.1.1,192.168.1.2
	Port                int                                                     //代理端口
	Host                string                                                  //代理host
	CrtFile             []byte                                                  //公钥,根证书
	KeyFile             []byte                                                  //私钥
	Capture             bool                                                    //抓包开关
	TLSHandshakeTimeout int64                                                   //tls 握手超时时间
	DnsCacheTime        int64                                                   //dns 缓存时间
	GetProxy            func(ctx context.Context, url *url.URL) (string, error) //代理ip http://116.62.55.139:8888
	Proxy               string                                                  //代理ip http://192.168.1.50:8888
	KeepAlive           int64                                                   //保活时间
	LocalAddr           string                                                  //本地网卡出口
	ServerName          string                                                  //https 域名或ip
	Vpn                 bool                                                    //是否是vpn
	Dns                 string                                                  //dns
	AddrType            requests.AddrType                                       //host优先解析的类型
	GetAddrType         func(string) requests.AddrType                          //控制host优先解析的类型
}

type Client struct {
	Debug               bool  //是否打印debug
	Err                 error //错误
	DisVerify           bool  //关闭验证
	ResponseCallBack    func(*http.Request, *http.Response)
	WsCallBack          func(websocket.MessageType, []byte, string)
	ReadRequestCallBack func(*http.Request)

	capture bool

	http2Server    *http2.Server
	http2Transport *http2.Transport
	cert           tls.Certificate
	dialer         *requests.DialClient //连接的Dialer
	listener       net.Listener         //Listener 服务
	basic          string
	usr            string
	pwd            string
	vpn            bool
	ipWhite        *kinds.Set[string]
	ja3            bool
	ja3Spec        ja3.ClientHelloSpec
	ctx            context.Context
	cnl            context.CancelFunc
	host           string
	port           string
}

func NewClient(pre_ctx context.Context, options ...ClientOption) (*Client, error) {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if pre_ctx == nil {
		pre_ctx = context.TODO()
	}
	ctx, cnl := context.WithCancel(pre_ctx)
	server := Client{}
	server.ctx = ctx
	server.cnl = cnl
	if option.Vpn {
		server.vpn = option.Vpn
	}
	if option.Usr != "" && option.Pwd != "" {
		server.basic = "Basic " + tools.Base64Encode(option.Usr+":"+option.Pwd)
		server.usr = option.Usr
		server.pwd = option.Pwd
	}
	if option.Ja3 {
		server.ja3 = true
	} else if option.Ja3Spec.IsSet() {
		server.ja3 = true
	}
	server.ja3Spec = option.Ja3Spec
	//白名单
	server.ipWhite = kinds.NewSet[string]()
	for _, ip_white := range option.IpWhite {
		server.ipWhite.Add(ip_white.String())
	}
	var err error
	//dialer
	if server.dialer, err = requests.NewDail(requests.DialOption{
		TLSHandshakeTimeout: option.TLSHandshakeTimeout,
		DnsCacheTime:        option.DnsCacheTime,
		GetProxy:            option.GetProxy,
		Proxy:               option.Proxy,
		KeepAlive:           option.KeepAlive,
		LocalAddr:           option.LocalAddr,
		Ja3:                 option.ProxyJa3,
		Ja3Spec:             option.ProxyJa3Spec,
		DisDnsCache:         option.DisDnsCache,
		Dns:                 option.Dns,
		GetAddrType:         option.GetAddrType,
		AddrType:            option.AddrType,
	}); err != nil {
		return nil, err
	}
	//证书
	if option.CrtFile == nil || option.KeyFile == nil {
		if server.cert, err = getCert(option.ServerName); err != nil {
			return nil, err
		}
	} else {
		if server.cert, err = tls.X509KeyPair(option.CrtFile, option.KeyFile); err != nil {
			return nil, err
		}
	}
	//构造listen
	if server.listener, err = net.Listen("tcp", net.JoinHostPort(option.Host, strconv.Itoa(option.Port))); err != nil {
		return nil, err
	}
	if server.host, server.port, err = net.SplitHostPort(server.listener.Addr().String()); err != nil {
		return nil, err
	}
	server.capture = option.Capture
	server.http2Server = new(http2.Server)
	server.http2Transport = new(http2.Transport)
	return &server, nil
}

// 代理监听的端口
func (obj *Client) Addr() string {
	return net.JoinHostPort(obj.host, obj.port)
}

func (obj *Client) Run() error {
	defer obj.Close()
	pool := thread.NewClient(obj.ctx, 65535)
	pool.Debug = obj.Debug
	for {
		select {
		case <-obj.ctx.Done():
			obj.Err = obj.ctx.Err()
			return obj.Err
		default:
			client, err := obj.listener.Accept() //接受数据
			if err != nil {
				obj.Err = err
				return err
			}
			if _, err = pool.Write(&thread.Task{
				Func: obj.mainHandle,
				Args: []any{client},
			}); err != nil {
				obj.Err = err
				return obj.Err
			}
		}
	}
}
func (obj *Client) Close() {
	obj.listener.Close()
	obj.cnl()
}
func (obj *Client) Done() <-chan struct{} {
	return obj.ctx.Done()
}

func (obj *Client) whiteVerify(client net.Conn) bool {
	if obj.DisVerify {
		return true
	}
	host, _, err := net.SplitHostPort(client.RemoteAddr().String())
	if err != nil || !obj.ipWhite.Has(host) {
		return false
	}
	return true
}

// 返回:请求所有内容,第一行的内容被" "分割的数组,第一行的内容,error
func (obj *Client) verifyPwd(client net.Conn, clientReq *http.Request) error {
	if obj.basic != "" && clientReq.Header.Get("Proxy-Authorization") != obj.basic && !obj.whiteVerify(client) { //验证密码是否正确
		client.Write([]byte(fmt.Sprintf("%s 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic\r\n\r\n", clientReq.Proto)))
		return errors.New("auth verify fail")
	}
	return nil
}
func (obj *Client) getHttpProxyConn(ctx context.Context, ipUrl *url.URL) (net.Conn, error) {
	return obj.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipUrl.Hostname(), ipUrl.Port()))
}

func (obj *Client) mainHandle(ctx context.Context, client net.Conn) (err error) {
	if obj.Debug {
		defer func() {
			if err != nil {
				log.Print("proxy debugger:\n", err)
			}
		}()
	}
	if client == nil {
		return errors.New("client is nil")
	}
	defer client.Close()
	if obj.basic == "" && !obj.whiteVerify(client) {
		return errors.New("auth verify false")
	}
	clientReader := bufio.NewReader(client)
	firstCons, err := clientReader.Peek(1)
	if err != nil {
		return err
	}
	if obj.vpn {
		if firstCons[0] == 22 {
			return obj.httpsHandle(ctx, newProxyCon(ctx, client, clientReader, ProxyOption{}, true))
		}
		return errors.New("vpn error")
	}
	switch firstCons[0] {
	case 5: //socks5 代理
		return obj.sockes5Handle(ctx, newProxyCon(ctx, client, clientReader, ProxyOption{}, true))
	case 22: //https 代理
		return obj.httpsHandle(ctx, newProxyCon(ctx, client, clientReader, ProxyOption{}, true))
	default: //http 代理
		return obj.httpHandle(ctx, newProxyCon(ctx, client, clientReader, ProxyOption{}, true))
	}
}
