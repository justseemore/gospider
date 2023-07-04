package ja3

import (
	"context"
	"errors"
	"net"
	"time"
)

type Conn struct {
	reader  <-chan []byte
	writer  chan<- []byte
	readerI <-chan int
	writerI chan<- int

	ctx context.Context
	cnl context.CancelFunc

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
		return 0, errors.New("ja3Pip读取超时")
	case <-obj.ctx.Done():
		return 0, errors.New("ja3Pip closed")
	case obj.writerI <- len(b):
		select {
		case <-obj.readTimer.C:
			return 0, errors.New("ja3Pip读取超时")
		case <-obj.ctx.Done():
			return 0, errors.New("ja3Pip closed")
		case con := <-obj.reader:
			return copy(b, con), nil
		}
	}
}
func (obj *Conn) Write(b []byte) (n int, err error) {
	defer func() {
		if err != nil {
			obj.Close()
		}
	}()
	for {
		select {
		case <-obj.writerTimer.C:
			return 0, errors.New("ja3Pip写入超时")
		case <-obj.ctx.Done():
			return 0, errors.New("ja3Pip closed")
		case i := <-obj.readerI:
			if len(b) <= i {
				select {
				case <-obj.writerTimer.C:
					return n, errors.New("ja3Pip写入超时")
				case <-obj.ctx.Done():
					return 0, errors.New("ja3Pip closed")
				case obj.writer <- b:
					n += len(b)
					return
				}
			} else {
				wb := b[:i]
				b = b[i:]
				select {
				case <-obj.writerTimer.C:
					return n, errors.New("ja3Pip写入超时")
				case <-obj.ctx.Done():
					return 0, errors.New("ja3Pip closed")
				case obj.writer <- wb:
					n += len(wb)
				}
			}
		}
	}
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
