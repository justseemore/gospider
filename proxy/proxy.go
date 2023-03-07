package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	_ "unsafe"

	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/kinds"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"
	"gitee.com/baixudong/gospider/websocket"
)

//go:embed gospider.crt
var CrtFile []byte

//go:embed gospider.key
var KeyFile []byte

type ClientOption struct {
	Ja3         bool              //是否开启ja3
	Ja3Id       ja3.ClientHelloId //指定ja3id
	ProxyJa3    bool              //proxy是否开启ja3
	ProxyJa3Id  ja3.ClientHelloId //proxy指定ja3id
	DisDnsCache bool              //是否关闭dns 缓存

	Usr     string   //用户名
	Pwd     string   //密码
	IpWhite []net.IP //白名单 192.168.1.1,192.168.1.2
	Port    int      //代理端口
	Host    string   //代理host
	CrtFile []byte   //公钥,根证书
	KeyFile []byte   //私钥

	TLSHandshakeTimeout int64
	DnsCacheTime        int64
	GetProxy            func(ctx context.Context, url *url.URL) (string, error) //代理ip http://116.62.55.139:8888
	Proxy               string                                                  //代理ip http://192.168.1.50:8888
	KeepAlive           int64
	LocalAddr           string //本地网卡出口
	Vpn                 bool   //是否是vpn
	Dns                 string //dns
}

//go:linkname readRequest net/http.readRequest
func readRequest(b *bufio.Reader) (*http.Request, error)

type Client struct {
	Debug            bool  //是否打印debug
	Err              error //错误
	DisVerify        bool  //关闭验证
	RequestCallBack  func(*http.Request)
	ResponseCallBack func(*http.Request, *http.Response)
	WsSendCallBack   func(*MsgData)
	WsRecvCallBack   func(*MsgData)

	cert     tls.Certificate
	dialer   *requests.DialClient //连接的Dialer
	listener net.Listener         //Listener 服务
	basic    string
	usr      string
	pwd      string
	vpn      bool
	ipWhite  *kinds.Set[string]
	ja3      bool
	ja3Id    ja3.ClientHelloId
	ctx      context.Context
	cnl      context.CancelFunc
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
		if option.Ja3Id.IsSet() {
			server.ja3Id = ja3.HelloChrome_Auto
		} else {
			server.ja3Id = option.Ja3Id
		}
	} else if !option.Ja3Id.IsSet() {
		server.ja3 = true
		server.ja3Id = option.Ja3Id
	}
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
		Ja3Id:               option.ProxyJa3Id,
		DisDnsCache:         option.DisDnsCache,
		Dns:                 option.Dns,
	}); err != nil {
		return nil, err
	}
	//证书
	if option.CrtFile != nil && KeyFile != nil {
		if server.cert, err = tls.X509KeyPair(option.CrtFile, option.KeyFile); err != nil {
			return nil, err
		}
	} else {
		if server.cert, err = tls.X509KeyPair(CrtFile, KeyFile); err != nil {
			return nil, err
		}
	}
	//构造listen
	if server.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", option.Host, option.Port)); err != nil {
		return nil, err
	}
	return &server, nil
}

// 代理监听的端口
func (obj *Client) Addr() string {
	return obj.listener.Addr().String()
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
			return obj.httpsHandle(ctx, NewProxyCon(client, clientReader, nil))
		}
		return errors.New("vpn error")
	}
	switch firstCons[0] {
	case 5: //socks5 代理
		return obj.sockes5Handle(ctx, NewProxyCon(client, clientReader, nil))
	case 22: //https 代理
		return obj.httpsHandle(ctx, NewProxyCon(client, clientReader, nil))
	default: //http 代理
		return obj.httpHandle(ctx, NewProxyCon(client, clientReader, nil))
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
}
type ProxyConn struct {
	conn   net.Conn
	reader *bufio.Reader
	option *ProxyOption
}

func NewProxyCon(conn net.Conn, reader *bufio.Reader, option *ProxyOption) *ProxyConn {
	if option == nil {
		option = new(ProxyOption)
	}
	return &ProxyConn{conn: conn, reader: reader, option: option}
}
func (obj *ProxyConn) Read(b []byte) (int, error) {
	return obj.reader.Read(b)
}
func (obj *ProxyConn) Write(b []byte) (int, error) {
	return obj.conn.Write(b)
}
func (obj *ProxyConn) Close() error {
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
	if strings.HasPrefix(clientReq.Host, "127.0.0.1") || strings.HasPrefix(clientReq.Host, "localhost") {
		return clientReq, errors.New("loop addr error")
	}
	return clientReq, err
}

func (obj *ProxyConn) WriteResponse(response *http.Response) error {
	return response.Write(obj.conn)
}
func (obj *ProxyConn) WriteRequest(clientReq *http.Request) error {
	return clientReq.Write(obj.conn)
}

func (obj *Client) httpsHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	tlsClient := tls.Server(client, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{obj.cert},
		NextProtos:         []string{"h2", "http/1.1"},
	})
	defer tlsClient.Close()
	return obj.httpHandle(ctx, NewProxyCon(tlsClient, bufio.NewReader(tlsClient), client.option))
}
func (obj *Client) httpHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	var err error
	clientReq, err := obj.ReadRequest(client)
	if err != nil {
		return err
	}
	if err = obj.verifyPwd(client, clientReq); err != nil {
		return err
	}
	proxyUrl, err := obj.dialer.GetProxy(ctx, nil)
	if err != nil {
		return err
	}
	var server net.Conn
	network := "tcp"
	host := clientReq.Host
	addr := net.JoinHostPort(clientReq.URL.Hostname(), clientReq.URL.Port())
	if server, err = obj.dialer.DialContextForProxy(ctx, network, client.option.schema, addr, host, proxyUrl); err != nil {
		return err
	}
	defer server.Close()
	proxyServer := NewProxyCon(server, bufio.NewReader(server), client.option)
	if clientReq.Method == http.MethodConnect {
		if _, err = client.Write([]byte(fmt.Sprintf("%s 200 Connection established\r\n\r\n", clientReq.Proto))); err != nil {
			return err
		}
	} else {
		if err = proxyServer.WriteRequest(clientReq); err != nil {
			return err
		}
		if obj.ResponseCallBack != nil {
			response, err := obj.ReadResponse(proxyServer, clientReq)
			if err != nil {
				return err
			}
			if err = client.WriteResponse(response); err != nil {
				return err
			}
		}
	}
	return obj.copyMain(ctx, client, proxyServer)
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
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
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
	server, err := obj.dialer.DialContextForProxy(ctx, netword, client.option.schema, addr, host, proxyUrl)
	if err != nil {
		return err
	}
	defer server.Close()
	return obj.copyMain(ctx, client, NewProxyCon(server, bufio.NewReader(server), client.option))
}

func (obj *Client) copyMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	if client.option.schema == "http" {
		return obj.copyHttpMain(ctx, client, server)
	} else if client.option.schema == "https" {
		return obj.copyHttpsMain(ctx, client, server)
	} else {
		return errors.New("schema error")
	}
}

type MsgData struct {
	MsgType websocket.MessageType
	Data    []byte
}

func (obj *Client) ReadRequest(conn *ProxyConn) (*http.Request, error) {
	rs, err := conn.readRequest()
	if err != nil || obj.RequestCallBack == nil {
		return rs, err
	}
	obj.RequestCallBack(rs)
	return rs, err
}
func (obj *Client) ReadResponse(conn *ProxyConn, req *http.Request) (*http.Response, error) {
	rs, err := conn.readResponse(req)
	if err != nil || obj.ResponseCallBack == nil {
		return rs, err
	}
	obj.ResponseCallBack(req, rs)
	return rs, err
}
func (obj *Client) WsSend(ctx context.Context, conn *websocket.Conn, msgData *MsgData) error {
	if obj.WsSendCallBack != nil {
		obj.WsSendCallBack(msgData)
	}
	return conn.Send(ctx, msgData.MsgType, msgData.Data)
}
func (obj *Client) WsRecv(ctx context.Context, conn *websocket.Conn) (*MsgData, error) {
	msgType, msgData, err := conn.Recv(ctx)
	rs := &MsgData{
		MsgType: msgType,
		Data:    msgData,
	}
	if obj.WsRecvCallBack != nil {
		obj.WsRecvCallBack(rs)
	}
	return rs, err
}

func (obj *Client) httpSend(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	var req *http.Request
	for !server.option.isWs {
		if req, err = obj.ReadRequest(client); err != nil {
			return err
		}
		if err = server.WriteRequest(req); err != nil {
			return err
		}
	}
	return nil
}
func (obj *Client) httpRecv(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	var req *http.Request
	var rsp *http.Response
	for !server.option.isWs {
		if req, err = obj.ReadRequest(client); err != nil {
			return err
		}
		if err = server.WriteRequest(req); err != nil {
			return err
		}
		if rsp, err = obj.ReadResponse(server, req); err != nil {
			return err
		}
		if err = client.WriteResponse(rsp); err != nil {
			return err
		}
	}
	return nil
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
		if err = obj.WsSend(ctx, wsServer, &MsgData{MsgType: msgType, Data: msgData}); err != nil {
			return
		}
	}
}
func (obj *Client) wsRecv(ctx context.Context, wsClient *websocket.Conn, wsServer *websocket.Conn) (err error) {
	defer wsServer.Close("close")
	defer wsClient.Close("close")
	var msgData *MsgData
	for {
		obj.WsRecv(ctx, wsServer)
		if msgData, err = obj.WsRecv(ctx, wsServer); err != nil {
			return
		}
		if err = wsClient.Send(ctx, msgData.MsgType, msgData.Data); err != nil {
			return
		}
	}
}
func (obj *Client) httpCopy(ctx context.Context, writer *ProxyConn, reader *ProxyConn) (err error) {
	defer reader.Close()
	defer writer.Close()
	_, err = io.Copy(writer, reader)
	return err
}

func (obj *Client) httpCopyAll(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer client.Close()
	defer server.Close()
	go obj.httpCopy(ctx, client, server)
	return obj.httpCopy(ctx, server, client)
}
func (obj *Client) copyHttpMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer server.Close()
	defer client.Close()
	if client.option.http2 || server.option.http2 {
		return obj.httpCopyAll(ctx, client, server)
	}
	if obj.ResponseCallBack == nil && obj.WsRecvCallBack == nil { //排除 全部回调
		go obj.httpCopy(ctx, client, server)
		if obj.RequestCallBack == nil && obj.WsSendCallBack == nil { //排除发送回调
			return obj.httpCopy(ctx, server, client)
		} else { //有发送回调
			if err = obj.httpSend(ctx, client, server); err != nil {
				return err
			}
			if obj.WsSendCallBack == nil { //没有ws 回调直接返回
				return obj.httpCopy(ctx, server, client)
			} else { //有ws 发送回调
				wsClient := websocket.NewConn(client, false, client.option.wsOption)
				wsServer := websocket.NewConn(server, true, server.option.wsOption)
				return obj.wsSend(ctx, wsClient, wsServer)
			}
		}
	} else { //有接受回调，则发送也要处理
		if err = obj.httpRecv(ctx, client, server); err != nil {
			return err
		}
		if obj.WsRecvCallBack == nil && obj.WsSendCallBack == nil { //没有ws 回调直接返回
			return obj.httpCopyAll(ctx, client, server)
		}
		wsClient := websocket.NewConn(client, false, client.option.wsOption)
		wsServer := websocket.NewConn(server, true, server.option.wsOption)
		defer wsServer.Close("close")
		defer wsClient.Close("close")
		go func() {
			if obj.WsRecvCallBack == nil {
				err = obj.httpCopy(ctx, client, server)
				return
			}
			obj.wsRecv(ctx, wsClient, wsServer)
			return
		}()
		if obj.WsSendCallBack == nil {
			return obj.httpCopy(ctx, server, client)
		}
		return obj.wsSend(ctx, wsClient, wsServer)
	}
}

func (obj *Client) copyHttpsMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	if obj.RequestCallBack == nil && obj.ResponseCallBack == nil && obj.WsSendCallBack == nil && obj.WsRecvCallBack == nil && !obj.ja3 {
		return obj.copyHttpMain(ctx, client, server)
	}
	defer server.Close()
	defer client.Close()
	return obj.copyTlsHttpsMain(ctx, client, server)
}
func (obj *Client) tlsClient(ctx context.Context, conn net.Conn, http2 bool) (tlsConn *tls.Conn, err error) {
	protos := "http/1.1"
	if http2 {
		protos = "h2"
	}
	tlsConn = tls.Server(conn, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{obj.cert},
		NextProtos:         []string{protos},
	})
	return tlsConn, tlsConn.HandshakeContext(ctx)
}
func (obj *Client) tlsServer(ctx context.Context, conn net.Conn, addr string, ws bool) (net.Conn, bool, error) {
	if obj.ja3 {
		if tlsConn, err := ja3.Client(ctx, conn, obj.ja3Id, ws, addr); err != nil {
			return tlsConn, false, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", err
		}
	} else {
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(addr), NextProtos: []string{"h2", "http/1.1"}})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return tlsConn, false, err
		} else {
			return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol == "h2", err
		}
	}
}
func (obj *Client) copyTlsHttpsMain(ctx context.Context, client *ProxyConn, server *ProxyConn) (err error) {
	defer server.Close()
	defer client.Close()
	tlsServer, http2, err := obj.tlsServer(ctx, server, client.option.host, client.option.isWs || server.option.isWs)
	if err != nil {
		return err
	}
	tlsClient, err := obj.tlsClient(ctx, client, http2)
	if err != nil {
		return err
	}
	client.option.http2 = http2
	server.option.http2 = http2
	return obj.copyHttpMain(ctx, NewProxyCon(tlsClient, bufio.NewReader(tlsClient), client.option), NewProxyCon(tlsServer, bufio.NewReader(tlsServer), client.option))
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
	return fmt.Sprintf("%s:%d", addr, binary.BigEndian.Uint16(buf[:2])), nil
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
