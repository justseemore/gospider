package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "unsafe"

	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/kinds"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"
	"gitee.com/baixudong/gospider/websocket"
	utls "github.com/refraction-networking/utls"
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

//go:linkname readRequest net/http.readRequest
func readRequest(b *bufio.Reader) (*http.Request, error)

type Client struct {
	Debug           bool  //是否打印debug
	Err             error //错误
	DisVerify       bool  //关闭验证
	RequestCallBack func(*http.Request, *http.Response)
	WsCallBack      func(websocket.MessageType, []byte, string)
	capture         bool

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
	server.host = option.Host
	server.port = strconv.Itoa(option.Port)
	if server.listener, err = net.Listen("tcp", net.JoinHostPort(server.host, server.port)); err != nil {
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
			return obj.httpsHandle(ctx, NewProxyCon(ctx, client, clientReader, ProxyOption{}))
		}
		return errors.New("vpn error")
	}
	switch firstCons[0] {
	case 5: //socks5 代理
		return obj.sockes5Handle(ctx, NewProxyCon(ctx, client, clientReader, ProxyOption{}))
	case 22: //https 代理
		return obj.httpsHandle(ctx, NewProxyCon(ctx, client, clientReader, ProxyOption{}))
	default: //http 代理
		return obj.httpHandle(ctx, NewProxyCon(ctx, client, clientReader, ProxyOption{}))
	}
}

type ProxyOption struct {
	init     bool
	http2    bool
	host     string
	schema   string
	port     string
	isWs     bool
	wsOption websocket.Option
	ctx      context.Context
	cnl      context.CancelFunc
}
type ProxyConn struct {
	conn   net.Conn
	reader *bufio.Reader
	option *ProxyOption
}

func NewProxyCon(preCtx context.Context, conn net.Conn, reader *bufio.Reader, option ProxyOption) *ProxyConn {
	if option.ctx == nil || option.cnl == nil {
		option.ctx, option.cnl = context.WithCancel(preCtx)
	}
	return &ProxyConn{conn: conn, reader: reader, option: &option}
}

type connectionStater interface {
	ConnectionState() tls.ConnectionState
}
type connectionStater2 interface {
	ConnectionState() utls.ConnectionState
}

func (obj *ProxyConn) ConnectionState() tls.ConnectionState {
	tlsConn, ok := obj.conn.(connectionStater)
	if ok {
		return tlsConn.ConnectionState()
	} else {
		tlsConn2, ok := obj.conn.(connectionStater2)
		connstate := tlsConn2.ConnectionState()
		if ok {
			return tls.ConnectionState{
				Version:                     connstate.Version,
				HandshakeComplete:           connstate.HandshakeComplete,
				DidResume:                   connstate.DidResume,
				CipherSuite:                 connstate.CipherSuite,
				NegotiatedProtocol:          connstate.NegotiatedProtocol,
				NegotiatedProtocolIsMutual:  connstate.NegotiatedProtocolIsMutual,
				ServerName:                  connstate.ServerName,
				PeerCertificates:            connstate.PeerCertificates,
				VerifiedChains:              connstate.VerifiedChains,
				SignedCertificateTimestamps: connstate.SignedCertificateTimestamps,
				OCSPResponse:                connstate.OCSPResponse,
				TLSUnique:                   connstate.TLSUnique,
			}
		}
	}
	return tls.ConnectionState{}
}
func (obj *ProxyConn) Read(b []byte) (int, error) {
	n, err := obj.reader.Read(b)
	if err != nil {
		obj.Close()
	}
	return n, err
}
func (obj *ProxyConn) Write(b []byte) (int, error) {
	n, err := obj.conn.Write(b)
	if err != nil {
		obj.Close()
	}
	return n, err
}
func (obj *ProxyConn) Close() error {
	defer obj.option.cnl()
	return obj.conn.Close()
}
func (obj *ProxyConn) LocalAddr() net.Addr {
	return obj.conn.LocalAddr()
}
func (obj *ProxyConn) RemoteAddr() net.Addr {
	return obj.conn.RemoteAddr()
}
func (obj *ProxyConn) SetDeadline(t time.Time) error {
	return obj.conn.SetDeadline(t)
}
func (obj *ProxyConn) SetReadDeadline(t time.Time) error {
	return obj.conn.SetReadDeadline(t)
}
func (obj *ProxyConn) SetWriteDeadline(t time.Time) error {
	return obj.conn.SetWriteDeadline(t)
}
func (obj *ProxyConn) readResponse(req *http.Request) (*http.Response, error) {
	response, err := http.ReadResponse(obj.reader, req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == 101 && response.Header.Get("Upgrade") == "websocket" {
		obj.option.isWs = true
		obj.option.wsOption = websocket.GetHeaderOption(response.Header, false)
	}
	return response, err
}
func (obj *ProxyConn) readRequest() (*http.Request, error) {
	clientReq, err := readRequest(obj.reader)
	if err != nil {
		return clientReq, err
	}
	obj.option.init = true
	if clientReq.Header.Get("Upgrade") == "websocket" {
		obj.option.isWs = true
		obj.option.wsOption = websocket.GetHeaderOption(clientReq.Header, true)
	}

	hostName := clientReq.URL.Hostname()
	if obj.option.host == "" {
		if headHost := clientReq.Header.Get("Host"); headHost != "" {
			obj.option.host = headHost
		} else if clientReq.Host != "" {
			obj.option.host = clientReq.Host
		} else if hostName != "" {
			obj.option.host = hostName
		}
	}
	if hostName == "" {
		if clientReq.Host != "" {
			clientReq.URL.Host = clientReq.Host
		} else {
			clientReq.URL.Host = obj.option.host
		}
	}

	if hostName := clientReq.URL.Hostname(); hostName == "" {
		clientReq.URL.Host = clientReq.Host
	} else if clientReq.Host == "" {
		clientReq.Host = hostName
	}
	if obj.option.schema == "" {
		if clientReq.URL.Scheme == "" {
			if clientReq.Method == http.MethodConnect {
				obj.option.schema = "https"
			} else {
				obj.option.schema = "http"
			}
			clientReq.URL.Scheme = obj.option.schema
		} else {
			obj.option.schema = clientReq.URL.Scheme
		}
	} else if clientReq.URL.Scheme == "" {
		clientReq.URL.Scheme = obj.option.schema
	}
	if obj.option.port == "" {
		if clientReq.URL.Port() == "" {
			if obj.option.schema == "https" {
				obj.option.port = "443"
			} else {
				obj.option.port = "80"
			}
			clientReq.URL.Host = clientReq.URL.Hostname() + ":" + obj.option.port
		} else {
			obj.option.port = clientReq.URL.Port()
		}
	} else if clientReq.URL.Port() == "" {
		clientReq.URL.Host = clientReq.URL.Hostname() + ":" + obj.option.port
	}
	return clientReq, err
}

func (obj *Client) httpsHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	tlsClient := tls.Server(client, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{obj.cert},
	})
	defer tlsClient.Close()
	return obj.httpHandle(ctx, NewProxyCon(ctx, tlsClient, bufio.NewReader(tlsClient), *client.option))
}
func (obj *Client) httpHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	var err error
	clientReq, err := client.readRequest()
	if err != nil {
		return err
	}
	if strings.HasPrefix(clientReq.Host, "127.0.0.1") || strings.HasPrefix(clientReq.Host, "localhost") {
		if clientReq.URL.Port() == obj.port {
			return errors.New("loop addr error")
		}
	}
	if err = obj.verifyPwd(client, clientReq); err != nil {
		return err
	}
	proxyUrl, err := obj.dialer.GetProxy(ctx, nil)
	if err != nil {
		return err
	}
	var proxyServer net.Conn
	network := "tcp"
	host := clientReq.Host
	addr := net.JoinHostPort(clientReq.URL.Hostname(), clientReq.URL.Port())
	if proxyServer, err = obj.dialer.DialContextForProxy(ctx, network, client.option.schema, addr, host, proxyUrl); err != nil {
		return err
	}
	server := NewProxyCon(ctx, proxyServer, bufio.NewReader(proxyServer), *client.option)
	defer server.Close()
	if clientReq.Method == http.MethodConnect {
		if _, err = client.Write([]byte(fmt.Sprintf("%s 200 Connection established\r\n\r\n", clientReq.Proto))); err != nil {
			return err
		}
	} else {
		if err = clientReq.Write(server); err != nil {
			return err
		}
		if obj.RequestCallBack != nil {
			response, err := server.readResponse(clientReq)
			if err != nil {
				return err
			}
			obj.RequestCallBack(clientReq, response)
			if err = response.Write(client); err != nil {
				return err
			}
		}
	}
	return obj.copyMain(ctx, client, server)
}
func (obj *Client) sockes5Handle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	var err error
	if err = obj.verifySocket(client); err != nil {
		return err
	}
	//获取serverAddr
	addr, err := obj.getSocketAddr(client)
	if err != nil {
		return err
	}
	//获取host
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if strings.HasPrefix(addr, "127.0.0.1") || strings.HasPrefix(addr, "localhost") {
		if port == obj.port {
			return errors.New("loop addr error")
		}
	}
	//获取代理
	proxyUrl, err := obj.dialer.GetProxy(ctx, nil)
	if err != nil {
		return err
	}
	//获取schema
	httpsBytes, err := client.reader.Peek(1)
	if err != nil {
		return err
	}
	client.option.schema = "http"
	if httpsBytes[0] == 22 {
		client.option.schema = "https"
	}
	netword := "tcp"
	proxyServer, err := obj.dialer.DialContextForProxy(ctx, netword, client.option.schema, addr, host, proxyUrl)
	if err != nil {
		return err
	}
	server := NewProxyCon(ctx, proxyServer, bufio.NewReader(proxyServer), *client.option)
	server.option.port = port
	server.option.host = host
	defer server.Close()
	return obj.copyMain(ctx, client, server)
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
	go obj.http2Server.ServeConn(client, &http2.ServeConnOpts{
		Context: ctx,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = "https"
			r.URL.Host = net.JoinHostPort(client.option.host, client.option.port)
			r.Proto = "HTTP/1.1"
			r.ProtoMajor = 1
			r.ProtoMinor = 1
			if err = r.Write(server); err != nil {
				server.Close()
				client.Close()
			}
			resp, err := server.readResponse(r)
			if err != nil {
				server.Close()
				client.Close()
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
func (obj *Client) copyHttpMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer server.Close()
	defer client.Close()
	if client.option.http2 && !server.option.http2 { //http12 逻辑
		return obj.http21Copy(ctx, client, server)
	}
	if !client.option.http2 && server.option.http2 { //http12 逻辑
		return obj.http12Copy(ctx, client, server)
	}
	if obj.RequestCallBack == nil && obj.WsCallBack == nil { //没有回调直接返回
		go io.Copy(client, server)
		_, err = io.Copy(server, client)
		return err
	}
	if client.option.http2 && server.option.http2 { //http22 逻辑
		return obj.http22Copy(ctx, client, server)
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

func (obj *Client) tlsClient(ctx context.Context, conn net.Conn, ws bool, cert tls.Certificate) (tlsConn *tls.Conn, http2 bool, err error) {
	var nextProtos []string
	if ws {
		nextProtos = []string{"http/1.1"}
	} else {
		nextProtos = []string{"h2", "http/1.1"}
	}
	tlsConn = tls.Server(conn, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
		NextProtos:         nextProtos,
	})
	if err = tlsConn.HandshakeContext(ctx); err != nil {
		return nil, false, err
	}
	return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", err
}
func (obj *Client) tlsServer(ctx context.Context, conn net.Conn, addr string, ws bool) (net.Conn, bool, []*x509.Certificate, error) {
	if obj.ja3 {
		if tlsConn, err := ja3.NewClient(ctx, conn, obj.ja3Spec, ws, addr); err != nil {
			return tlsConn, false, nil, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", tlsConn.ConnectionState().PeerCertificates, err
		}
	} else {
		var nextProtos []string
		if ws {
			nextProtos = []string{"http/1.1"}
		} else {
			nextProtos = []string{"h2", "http/1.1"}
		}
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(addr), NextProtos: nextProtos})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return tlsConn, false, nil, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", tlsConn.ConnectionState().PeerCertificates, err
		}
	}
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
func (obj *Client) getSocketAddr(client *ProxyConn) (string, error) {
	buf := make([]byte, 4)
	addr := ""
	_, err := io.ReadFull(client.reader, buf) //读取版本号，CMD，RSV ，ATYP ，ADDR ，PORT
	if err != nil {
		return addr, fmt.Errorf("read header failed:%w", err)
	}
	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != 5 {
		return addr, fmt.Errorf("not supported ver:%v", ver)
	}
	if cmd != 1 {
		return addr, fmt.Errorf("not supported cmd:%v", ver)
	}
	switch atyp {
	case 1: //ipv4地址
		if _, err = io.ReadFull(client.reader, buf); err != nil {
			return addr, fmt.Errorf("read atyp failed:%w", err)
		}
		addr = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case 3: //域名
		hostSize, err := client.reader.ReadByte() //域名的长度
		if err != nil {
			return addr, fmt.Errorf("read hostSize failed:%w", err)
		}
		host := make([]byte, hostSize)
		if _, err = io.ReadFull(client.reader, host); err != nil {
			return addr, fmt.Errorf("read host failed:%w", err)
		}
		addr = tools.BytesToString(host)
	case 4: //IPv6地址
		host := make([]byte, 16)
		if _, err = io.ReadFull(client.reader, host); err != nil {
			return addr, fmt.Errorf("read atyp failed:%w", err)
		}
		addr = net.IP(host).String()
	default:
		return addr, errors.New("invalid atyp")
	}
	if _, err = io.ReadFull(client.reader, buf[:2]); err != nil { //读取端口号
		return addr, fmt.Errorf("read port failed:%w", err)
	}
	return net.JoinHostPort(addr, strconv.Itoa(int(binary.BigEndian.Uint16(buf[:2])))), nil
}
func (obj *Client) verifySocket(client *ProxyConn) error {
	ver, err := client.reader.ReadByte() //读取第一个字节判断是否是socks5协议
	if err != nil {
		return fmt.Errorf("read ver failed:%w", err)
	}
	if ver != 5 {
		return fmt.Errorf("not supported ver:%v", ver)
	}
	methodSize, err := client.reader.ReadByte() //读取第二个字节,method 的长度，支持认证的方法数量
	if err != nil {
		return fmt.Errorf("read methodSize failed:%w", err)
	}
	methods := make([]byte, methodSize)
	if _, err = io.ReadFull(client.reader, methods); err != nil { //读取method，支持认证的方法
		return fmt.Errorf("read method failed:%w", err)
	}
	if obj.basic != "" && !obj.whiteVerify(client) { //开始验证用户名密码
		if bytes.IndexByte(methods, 2) == -1 {
			return errors.New("不支持用户名密码验证")
		}
		_, err = client.Write([]byte{5, 2}) //告诉客户端要进行用户名密码验证
		if err != nil {
			return err
		}
		okVar, err := client.reader.ReadByte() //获取版本，通常为0x01
		if err != nil {
			return err
		}
		Len, err := client.reader.ReadByte() //获取用户名的长度
		if err != nil {
			return err
		}
		user := make([]byte, Len)
		if _, err = io.ReadFull(client.reader, user); err != nil {
			return err
		}
		if Len, err = client.reader.ReadByte(); err != nil { //获取密码的长度
			return err
		}
		pass := make([]byte, Len)
		if _, err = io.ReadFull(client.reader, pass); err != nil {
			return err
		}
		if tools.BytesToString(user) != obj.usr || tools.BytesToString(pass) != obj.pwd {
			client.Write([]byte{okVar, 0xff}) //用户名密码错误
			return errors.New("用户名密码错误")
		}
		_, err = client.Write([]byte{okVar, 0}) //协商成功
		return err
	}
	if _, err = client.Write([]byte{5, 0}); err != nil { //协商成功
		return err
	}
	if _, err = client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}); err != nil { //响应客户端连接成功
		return err
	}
	return err
}
