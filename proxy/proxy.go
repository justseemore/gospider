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

	"gitee.com/baixudong/gospider/kinds"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"
)

//go:embed gospider.crt
var CrtFile []byte

//go:embed gospider.key
var KeyFile []byte

type ClientOption struct {
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

}

type Client struct {
	Debug     bool  //是否打印debug
	Err       error //错误
	DisVerify bool  //关闭验证
	cert      tls.Certificate
	dialer    *requests.DialClient //连接的Dialer
	listener  net.Listener         //Listener 服务
	basic     string
	usr       string
	pwd       string
	verify    bool
	ipWhite   *kinds.Set[string]
	ctx       context.Context
	cnl       context.CancelFunc
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
	if option.Usr != "" && option.Pwd != "" {
		server.basic = "Basic " + tools.Base64Encode(option.Usr+":"+option.Pwd)
		server.usr = option.Usr
		server.pwd = option.Pwd
		server.verify = true
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
	if obj.verify && clientReq.Header.Get("Proxy-Authorization") != obj.basic && !obj.whiteVerify(client) { //验证密码是否正确
		client.Write([]byte(fmt.Sprintf("%s 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic\r\n\r\n", clientReq.Proto)))
		return errors.New("auth verify fail")
	}
	return nil
}
func (obj *Client) getHttpProxyConn(ctx context.Context, ipUrl *url.URL) (net.Conn, error) {
	return obj.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipUrl.Hostname(), ipUrl.Port()))
}

func (obj *Client) mainHandle(ctx context.Context, client net.Conn) error {
	if client == nil {
		return errors.New("client is nil")
	}
	defer client.Close()
	if !obj.verify && !obj.whiteVerify(client) {
		return errors.New("auth verify false")
	}
	var err error
	clientReader := bufio.NewReader(client)
	firstCons, err := clientReader.Peek(1)
	if err != nil {
		return err
	}
	switch firstCons[0] {
	case 5: //socks5 代理
		return obj.sockes5Handle(ctx, &ProxyConn{conn: client, reader: clientReader})
	case 22: //https 代理
		return obj.httpsHandle(ctx, &ProxyConn{conn: client, reader: clientReader})
	default: //http 代理
		return obj.httpHandle(ctx, &ProxyConn{conn: client, reader: clientReader})
	}
}

type ProxyConn struct {
	conn   net.Conn
	reader *bufio.Reader
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
func (obj *ProxyConn) ReadRequest() (*http.Request, bool, error) {
	clientReq, err := http.ReadRequest(obj.reader)
	if err != nil {
		return clientReq, false, err
	}
	if hostName := clientReq.URL.Hostname(); hostName == "" {
		clientReq.URL.Host = clientReq.Host
	} else if clientReq.Host == "" {
		clientReq.Host = hostName
	}
	if clientReq.URL.Port() == "" {
		if clientReq.Method == http.MethodConnect {
			clientReq.URL.Host = clientReq.URL.Hostname() + ":" + "443"
		} else {
			clientReq.URL.Host = clientReq.URL.Hostname() + ":" + "80"
		}
	}
	if clientReq.URL.Scheme == "" {
		if clientReq.Method == http.MethodConnect {
			clientReq.URL.Scheme = "https"
		} else {
			clientReq.URL.Scheme = "http"
		}
	}
	if strings.HasPrefix(clientReq.Host, "127.0.0.1") || strings.HasPrefix(clientReq.Host, "localhost") {
		return clientReq, false, errors.New("loop addr error")
	}
	return clientReq, clientReq.Header.Get("Upgrade") == "websocket", err
}
func (obj *ProxyConn) WriteRequest(clientReq *http.Request, w io.Writer, ipUrl *url.URL) error {
	for key := range clientReq.Header {
		if strings.HasPrefix(key, "Proxy-") {
			clientReq.Header.Del(key)
		}
	}
	if ipUrl != nil && ipUrl.User != nil { //添加代理密码
		if _, ok := ipUrl.User.Password(); ok {
			clientReq.Header.Set("Proxy-Authorization", "Basic "+tools.Base64Encode(ipUrl.User.String()))
		}
	}
	return clientReq.Write(w)
}

func (obj *Client) httpsHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	tlsClient := tls.Server(client, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{obj.cert},
	})
	defer tlsClient.Close()
	return obj.httpHandle(ctx, &ProxyConn{
		conn:   tlsClient,
		reader: bufio.NewReader(tlsClient),
	})
}
func (obj *Client) httpHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	var err error
	clientReq, _, err := client.ReadRequest()
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
	addr := net.JoinHostPort(clientReq.URL.Hostname(), clientReq.URL.Port())
	if server, err = obj.dialer.DialContextForProxy(ctx, network, clientReq.URL.Scheme, addr, clientReq.Host, proxyUrl); err != nil {
		return err
	}
	defer server.Close()
	if clientReq.Method == http.MethodConnect {
		if _, err = client.Write([]byte(fmt.Sprintf("%s 200 Connection established\r\n\r\n", clientReq.Proto))); err != nil {
			return err
		}
	} else {
		if err = client.WriteRequest(clientReq, server, nil); err != nil {
			return err
		}
	}

	go func() { //服务端到客户端
		defer server.Close()
		defer client.Close()
		io.Copy(client, server)
	}()
	_, err = io.Copy(server, client) //客户端发送服务端
	return err
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
	if obj.verify && !obj.whiteVerify(client) { //开始验证用户名密码
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
		log.Print(err)
		return err
	}
	//获取schema
	httpsBytes, err := client.reader.Peek(1)
	if err != nil {
		return err
	}
	schema := "http"
	if httpsBytes[0] == 22 {
		schema = "https"
	}
	netword := "tcp"
	server, err := obj.dialer.DialContextForProxy(ctx, netword, schema, addr, host, proxyUrl)
	if err != nil {
		return err
	}

	defer server.Close()
	go func() { //服务端到客户端
		defer server.Close()
		defer client.Close()
		io.Copy(client, server)
	}()
	_, err = io.Copy(server, client) //客户端发送服务端
	return err
}
