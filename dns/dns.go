package dns

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/tools"
	"golang.org/x/net/dns/dnsmessage"
)

type Client struct {
	nameServer string
	poolNum    int
	cacheTime  int64
	dnsConn    net.PacketConn
	saveData   map[[16]byte]msgData
	lock       sync.Mutex
	dialer     net.Dialer
}
type ClientOption struct {
	ListenAddr string //监听的地址；
	NameServer string //dns服务
	PoolNum    int    //处理最大并发数
	CacheTime  int64  //dns 缓存时间
	LocalAddr  string //本地网卡
}
type msgData struct {
	time    int64
	answers []dnsmessage.Resource
}

func NewClient(options ...ClientOption) (*Client, error) {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.ListenAddr == "" {
		option.ListenAddr = "0.0.0.0:53"
	} else if !strings.Contains(option.ListenAddr, ":") {
		option.ListenAddr += ":53"
	}
	if option.NameServer == "" {
		option.NameServer = "223.5.5.5:53"
	} else if !strings.Contains(option.NameServer, ":") {
		option.NameServer += ":53"
	}
	if option.PoolNum == 0 {
		option.PoolNum = 655350
	}
	if option.CacheTime == 0 {
		option.CacheTime = 60 * 30
	}

	var dialer net.Dialer
	if option.LocalAddr == "" {
		dialer = net.Dialer{Timeout: time.Duration(8) * time.Second}
	} else {
		localaddr, err := net.ResolveTCPAddr("tcp", option.LocalAddr)
		if err != nil {
			return nil, err
		}
		dialer = net.Dialer{Timeout: time.Duration(8) * time.Second, LocalAddr: localaddr}
	}
	dnsConn, err := net.ListenPacket("udp", option.ListenAddr)
	if err != nil {
		return nil, err
	}
	return &Client{
		dnsConn:    dnsConn,
		nameServer: option.NameServer,
		poolNum:    option.PoolNum,
		cacheTime:  option.CacheTime,
		dialer:     dialer,
		saveData:   map[[16]byte]msgData{},
	}, nil
}
func (obj *Client) Run(ctx context.Context) error {
	defer obj.dnsConn.Close()
	for {
		buf := make([]byte, 1248)
		_, addr, err := obj.dnsConn.ReadFrom(buf)
		if err != nil {
			return err
		}
		go obj.handle(ctx, addr, buf)
	}
}
func (obj *Client) handle(ctx context.Context, addr net.Addr, buf []byte) error {
	defer recover()
	var msg dnsmessage.Message
	if err := msg.Unpack(buf); err != nil {
		return err
	}
	if len(msg.Questions) == 0 {
		return errors.New("questions == 0")
	}
	key := bytes.NewBuffer(nil)
	for _, val := range msg.Questions {
		key.WriteString(val.GoString())
	}
	keyMd5 := tools.Md5(key.Bytes())
	msgdata, ok := obj.saveData[keyMd5]
	if ok {
		if time.Now().Unix()-msgdata.time < obj.cacheTime {
			msg.Response = true
			msg.Answers = msgdata.answers
			raw, err := msg.Pack()
			if err != nil {
				return err
			}
			_, err = obj.dnsConn.WriteTo(raw, addr)
			return err
		}
	}
	raw, err := msg.Pack()
	if err != nil {
		return err
	}
	conn, err := obj.dialer.DialContext(ctx, "udp", obj.nameServer)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.Write(raw); err != nil {
		return err
	}
	bn, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if err = msg.Unpack(buf[:bn]); err != nil {
		return err
	}
	obj.lock.Lock()
	obj.saveData[keyMd5] = msgData{answers: msg.Answers, time: time.Now().Unix()}
	obj.lock.Unlock()
	if raw, err = msg.Pack(); err != nil {
		return err
	}
	_, err = obj.dnsConn.WriteTo(raw, addr)
	return err
}
