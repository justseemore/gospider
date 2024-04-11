package ja3

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/justseemore/gospider/tools"
	utls "github.com/refraction-networking/utls"
)

type Conn struct {
	reader  <-chan []byte
	writer  chan<- []byte
	readerI <-chan int
	writerI chan<- int
	lock    sync.Mutex
	ctx     context.Context
	cnl     context.CancelFunc

	readTimer   time.Timer
	writerTimer time.Timer
}
type Addr struct{}

func (obj Addr) Network() string {
	return "ja3Pip"
}
func (obj Addr) String() string {
	return "ja3Pip"
}
func (obj *Conn) Read(b []byte) (n int, err error) {
	defer func() {
		if err != nil {
			obj.Close()
		}
	}()
	select {
	case <-obj.readTimer.C:
		return n, os.ErrDeadlineExceeded
	case <-obj.ctx.Done():
		return n, io.EOF
	case con := <-obj.reader:
		n = copy(b, con)
		select {
		case <-obj.readTimer.C:
			return n, os.ErrDeadlineExceeded
		case <-obj.ctx.Done():
			return n, io.EOF
		case obj.writerI <- n:
			return
		}
	}
}
func (obj *Conn) Write(b []byte) (n int, err error) {
	defer func() {
		if err != nil {
			obj.Close()
		}
	}()
	obj.lock.Lock()
	defer obj.lock.Unlock()
	for once := true; once || len(b) > 0; once = false {
		select {
		case <-obj.writerTimer.C:
			return n, os.ErrDeadlineExceeded
		case <-obj.ctx.Done():
			return n, io.EOF
		case obj.writer <- b:
			select {
			case <-obj.writerTimer.C:
				return n, os.ErrDeadlineExceeded
			case <-obj.ctx.Done():
				return n, io.EOF
			case i := <-obj.readerI:
				b = b[i:]
				n += i
			}
		}
	}
	return
}
func (obj *Conn) Close() error {
	obj.cnl()
	obj.readTimer.Stop()
	obj.writerTimer.Stop()
	return nil
}
func (obj *Conn) LocalAddr() net.Addr {
	return Addr{}
}
func (obj *Conn) RemoteAddr() net.Addr {
	return Addr{}
}
func (obj *Conn) SetDeadline(t time.Time) error {
	obj.SetReadDeadline(t)
	obj.SetWriteDeadline(t)
	return nil
}
func (obj *Conn) SetReadDeadline(t time.Time) error {
	obj.readTimer.Reset(time.Now().Sub(t))
	return nil
}
func (obj *Conn) SetWriteDeadline(t time.Time) error {
	obj.writerTimer.Reset(time.Now().Sub(t))
	return nil
}

func Pipe(preCtx context.Context) (net.Conn, net.Conn) {
	ctx, cnl := context.WithCancel(preCtx)
	readerCha := make(chan []byte)
	writerCha := make(chan []byte)

	readerI := make(chan int)
	writerI := make(chan int)
	localConn := &Conn{
		reader:      readerCha,
		readerI:     readerI,
		writer:      writerCha,
		writerI:     writerI,
		ctx:         ctx,
		cnl:         cnl,
		readTimer:   *time.NewTimer(time.Hour * 24 * 365 * 100),
		writerTimer: *time.NewTimer(time.Hour * 24 * 365 * 100),
	}
	remoteConn := &Conn{
		reader:      writerCha,
		readerI:     writerI,
		writer:      readerCha,
		writerI:     readerI,
		ctx:         ctx,
		cnl:         cnl,
		readTimer:   *time.NewTimer(time.Hour * 24 * 365 * 100),
		writerTimer: *time.NewTimer(time.Hour * 24 * 365 * 100),
	}
	return localConn, remoteConn
}

func Utls2Tls(preCtx, ctx context.Context, utlsConn *utls.UConn, host string) (*tls.Conn, error) {
	//获取cert
	var err error
	certs := utlsConn.ConnectionState().PeerCertificates
	var cert tls.Certificate
	if len(certs) > 0 {
		cert, err = tools.CreateProxyCertWithCert(nil, nil, certs[0])
	} else {
		cert, err = tools.CreateProxyCertWithName(tools.GetServerName(host))
	}
	if err != nil {
		return nil, err
	}
	localConn, remoteConn := Pipe(preCtx)
	//正常路径发送方
	tlsConn := tls.Client(localConn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         tools.GetServerName(host),
		NextProtos:         []string{"h2", "http/1.1"},
	})
	proto := utlsConn.ConnectionState().NegotiatedProtocol
	if proto == "" {
		proto = "http/1.1"
	}
	//代理接收方
	tlsClientConn := tls.Server(remoteConn, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
		NextProtos:         []string{proto},
	})
	go func() {
		defer utlsConn.Close()
		defer tlsConn.Close()
		defer tlsClientConn.Close()
		if err = tlsClientConn.Handshake(); err != nil {
			return
		}
		go func() {
			defer utlsConn.Close()
			defer tlsConn.Close()
			defer tlsClientConn.Close()
			_, err = io.Copy(utlsConn, tlsClientConn)
		}()
		_, err = io.Copy(tlsClientConn, utlsConn)
	}()

	if err = tlsConn.HandshakeContext(ctx); err != nil {
		tlsClientConn.Close()
		utlsConn.Close()
		tlsConn.Close()
	}
	return tlsConn, err
}
